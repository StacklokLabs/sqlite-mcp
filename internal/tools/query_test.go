package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StacklokLabs/sqlite-mcp/internal/testutil"
)

// Tool and argument names used across the tool-call test cases.
const (
	toolExecuteQuery     = "execute_query"
	toolExecuteStatement = "execute_statement"
	toolDescribeTable    = "describe_table"

	argQuery      = "query"
	argStatement  = "statement"
	argParameters = "parameters"
)

func TestNew(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	qt := New(db)
	assert.NotNil(t, qt)
}

func TestGetTools(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	qt := New(db)
	tools := qt.GetTools()

	assert.Len(t, tools, 4)

	toolNames := make([]string, len(tools))
	for i, tool := range tools {
		toolNames[i] = tool.Name
	}

	assert.Contains(t, toolNames, toolExecuteQuery)
	assert.Contains(t, toolNames, toolExecuteStatement)
	assert.Contains(t, toolNames, "list_tables")
	assert.Contains(t, toolNames, toolDescribeTable)
}

func TestHandleExecuteQuery(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	qt := New(db)
	ctx := context.Background()

	t.Run("successful select query", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolExecuteQuery,
				Arguments: map[string]interface{}{
					argQuery: "SELECT * FROM users ORDER BY id",
				},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "Alice")
		assert.Contains(t, text, "Bob")
	})

	t.Run("query with parameters", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolExecuteQuery,
				Arguments: map[string]interface{}{
					argQuery:      "SELECT * FROM users WHERE age > ?",
					argParameters: []interface{}{"27"},
				},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "Alice")
		assert.NotContains(t, text, "Bob")
	})

	t.Run("non-select query rejected", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolExecuteQuery,
				Arguments: map[string]interface{}{
					argQuery: "INSERT INTO users (name, email, age) VALUES ('Charlie', 'charlie@example.com', 35)",
				},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "only SELECT queries are allowed")
	})

	t.Run("missing query parameter", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      toolExecuteQuery,
				Arguments: map[string]interface{}{},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "query parameter is required")
	})
}

func TestHandleExecuteStatement(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	qt := New(db)
	ctx := context.Background()

	t.Run("successful insert", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolExecuteStatement,
				Arguments: map[string]interface{}{
					argStatement:  "INSERT INTO users (name, email, age) VALUES (?, ?, ?)",
					argParameters: []interface{}{"Charlie", "charlie@example.com", "35"},
				},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "Rows affected: 1")
	})

	t.Run("successful update", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolExecuteStatement,
				Arguments: map[string]interface{}{
					argStatement:  "UPDATE users SET age = ? WHERE name = ?",
					argParameters: []interface{}{"31", "Alice"},
				},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "Rows affected: 1")
	})

	t.Run("select query rejected", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolExecuteStatement,
				Arguments: map[string]interface{}{
					argStatement: "SELECT * FROM users",
				},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "SELECT queries should use execute_query tool")
	})
}

func TestHandleListTables(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	qt := New(db)
	ctx := context.Background()

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "list_tables",
			Arguments: map[string]interface{}{},
		},
	}

	result, err := qt.HandleTool(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, result)

	text := testutil.GetTextContent(t, result.Content[0])
	assert.Contains(t, text, "users")
	assert.Contains(t, text, "products")
}

func TestHandleDescribeTable(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	qt := New(db)
	ctx := context.Background()

	t.Run("existing table", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolDescribeTable,
				Arguments: map[string]interface{}{
					"table_name": "users",
				},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)
		assert.NotNil(t, result)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "id")
		assert.Contains(t, text, "name")
		assert.Contains(t, text, "email")
		assert.Contains(t, text, "age")
	})

	t.Run("non-existent table", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: toolDescribeTable,
				Arguments: map[string]interface{}{
					"table_name": "non_existent",
				},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "Table 'non_existent' not found")
	})

	t.Run("missing table name", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      toolDescribeTable,
				Arguments: map[string]interface{}{},
			},
		}

		result, err := qt.HandleTool(ctx, request)
		require.NoError(t, err)

		text := testutil.GetTextContent(t, result.Content[0])
		assert.Contains(t, text, "table_name parameter is required")
	})
}

func TestHandleUnknownTool(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	qt := New(db)
	ctx := context.Background()

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "unknown_tool",
			Arguments: map[string]interface{}{},
		},
	}

	result, err := qt.HandleTool(ctx, request)
	require.NoError(t, err)

	text := testutil.GetTextContent(t, result.Content[0])
	assert.Contains(t, text, "Unknown tool: unknown_tool")
}
