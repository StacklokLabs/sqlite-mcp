// Package tools provides MCP tools for SQLite database operations
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StacklokLabs/sqlite-mcp/internal/database"
)

// hasMultipleStatements reports whether sql contains more than one SQL
// statement. The underlying driver executes every ;-separated statement in a
// single call, so a lone prefix check (e.g. "starts with SELECT") lets an
// attacker chain further statements such as ATTACH after a harmless SELECT.
// Semicolons inside string literals, quoted identifiers and comments are not
// statement separators, so they are skipped. A single trailing semicolon, and
// trailing whitespace or comments, are allowed.
func hasMultipleStatements(sql string) bool {
	runes := []rune(sql)
	n := len(runes)
	sawSeparator := false
	for i := 0; i < n; i++ {
		switch c := runes[i]; {
		case c == '-' && i+1 < n && runes[i+1] == '-':
			for i < n && runes[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && runes[i+1] == '*':
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i++ // skip the closing '/'
		case c == '\'' || c == '"' || c == '`':
			i++
			for i < n {
				if runes[i] == c {
					if i+1 < n && runes[i+1] == c { // doubled-quote escape
						i += 2
						continue
					}
					break
				}
				i++
			}
		case c == '[': // SQLite bracket-quoted identifier
			for i < n && runes[i] != ']' {
				i++
			}
		case c == ';':
			sawSeparator = true
		default:
			// Any real content after a statement separator means there is a
			// second statement.
			if sawSeparator && !unicode.IsSpace(c) {
				return true
			}
		}
	}
	return false
}

// QueryTools provides MCP tools for SQLite database operations
type QueryTools struct {
	db *database.DB
}

// New creates a new QueryTools instance
func New(db *database.DB) *QueryTools {
	return &QueryTools{db: db}
}

// GetTools returns all available MCP tools
func (qt *QueryTools) GetTools() []mcp.Tool {
	return []mcp.Tool{
		qt.executeQueryTool(),
		qt.executeStatementTool(),
		qt.listTablesTool(),
		qt.describeTableTool(),
	}
}

// executeQueryTool creates the execute_query tool for SELECT operations
func (*QueryTools) executeQueryTool() mcp.Tool {
	return mcp.NewTool(
		"execute_query",
		mcp.WithDescription("Execute a SELECT query against the SQLite database"),
		mcp.WithString("query", mcp.Required(), mcp.Description("The SQL SELECT query to execute")),
		mcp.WithArray("parameters", mcp.Description("Optional parameters for the query"), mcp.Items(map[string]any{"type": "string"})),
	)
}

// executeStatementTool creates the execute_statement tool for INSERT/UPDATE/DELETE operations
func (*QueryTools) executeStatementTool() mcp.Tool {
	return mcp.NewTool(
		"execute_statement",
		mcp.WithDescription("Execute an INSERT, UPDATE, or DELETE statement against the SQLite database"),
		mcp.WithString("statement", mcp.Required(), mcp.Description("The SQL statement to execute")),
		mcp.WithArray("parameters",
			mcp.Description("Optional parameters for the statement"),
			mcp.Items(map[string]any{"type": "string"})),
	)
}

// listTablesTool creates the list_tables tool
func (*QueryTools) listTablesTool() mcp.Tool {
	return mcp.NewTool(
		"list_tables",
		mcp.WithDescription("List all tables in the SQLite database"),
	)
}

// describeTableTool creates the describe_table tool
func (*QueryTools) describeTableTool() mcp.Tool {
	return mcp.NewTool(
		"describe_table",
		mcp.WithDescription("Get the schema information for a specific table"),
		mcp.WithString("table_name", mcp.Required(), mcp.Description("The name of the table to describe")),
	)
}

// HandleTool handles MCP tool calls
func (qt *QueryTools) HandleTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	switch request.Params.Name {
	case "execute_query":
		return qt.handleExecuteQuery(ctx, request)
	case "execute_statement":
		return qt.handleExecuteStatement(ctx, request)
	case "list_tables":
		return qt.handleListTables(ctx, request)
	case "describe_table":
		return qt.handleDescribeTable(ctx, request)
	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown tool: %s", request.Params.Name)), nil
	}
}

// handleExecuteQuery handles SELECT queries
func (qt *QueryTools) handleExecuteQuery(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := mcp.ParseString(request, "query", "")
	if query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	// Validate that it's a SELECT query
	trimmedQuery := strings.TrimSpace(strings.ToUpper(query))
	if !strings.HasPrefix(trimmedQuery, "SELECT") {
		return mcp.NewToolResultError("only SELECT queries are allowed with execute_query"), nil
	}

	// Reject statement chaining. Without this, a payload like
	// "SELECT 1; ATTACH ..." passes the SELECT prefix check and the driver
	// still runs the trailing statements, defeating read-only mode.
	if hasMultipleStatements(query) {
		return mcp.NewToolResultError("only a single SQL statement is allowed with execute_query"), nil
	}

	// Parse parameters
	var params []interface{}
	if paramArray := mcp.ParseArgument(request, "parameters", nil); paramArray != nil {
		if paramSlice, ok := paramArray.([]interface{}); ok {
			params = paramSlice
		}
	}

	results, err := qt.db.Query(query, params...)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Query execution failed", err), nil
	}

	// Format results as JSON
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to format results", err), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Query executed successfully. Results:\n```json\n%s\n```", string(jsonData))), nil
}

// handleExecuteStatement handles INSERT/UPDATE/DELETE statements
func (qt *QueryTools) handleExecuteStatement(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	statement := mcp.ParseString(request, "statement", "")
	if statement == "" {
		return mcp.NewToolResultError("statement parameter is required"), nil
	}

	// Validate that it's not a SELECT query
	trimmedStatement := strings.TrimSpace(strings.ToUpper(statement))
	if strings.HasPrefix(trimmedStatement, "SELECT") {
		return mcp.NewToolResultError("SELECT queries should use execute_query tool"), nil
	}

	// Reject statement chaining here too, so a single write tool call cannot
	// smuggle extra statements (e.g. ATTACH to reach files outside the
	// configured database).
	if hasMultipleStatements(statement) {
		return mcp.NewToolResultError("only a single SQL statement is allowed with execute_statement"), nil
	}

	// Parse parameters
	var params []interface{}
	if paramArray := mcp.ParseArgument(request, "parameters", nil); paramArray != nil {
		if paramSlice, ok := paramArray.([]interface{}); ok {
			params = paramSlice
		}
	}

	rowsAffected, err := qt.db.Execute(statement, params...)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Statement execution failed", err), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Statement executed successfully. Rows affected: %d", rowsAffected)), nil
}

// handleListTables handles listing all tables
func (qt *QueryTools) handleListTables(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tables, err := qt.db.GetTables()
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to list tables", err), nil
	}

	if len(tables) == 0 {
		return mcp.NewToolResultText("No tables found in the database"), nil
	}

	jsonData, err := json.MarshalIndent(tables, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to format table list", err), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Tables in database:\n```json\n%s\n```", string(jsonData))), nil
}

// handleDescribeTable handles table schema description
func (qt *QueryTools) handleDescribeTable(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tableName := mcp.ParseString(request, "table_name", "")
	if tableName == "" {
		return mcp.NewToolResultError("table_name parameter is required"), nil
	}

	schema, err := qt.db.GetTableSchema(tableName)
	if err != nil {
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("Failed to describe table '%s'", tableName), err), nil
	}

	if len(schema) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("Table '%s' not found", tableName)), nil
	}

	jsonData, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to format schema", err), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Schema for table '%s':\n```json\n%s\n```", tableName, string(jsonData))), nil
}
