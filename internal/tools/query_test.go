package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/StacklokLabs/sqlite-mcp/internal/testutil"
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

	assert.Contains(t, toolNames, "execute_query")
	assert.Contains(t, toolNames, "execute_statement")
	assert.Contains(t, toolNames, "list_tables")
	assert.Contains(t, toolNames, "describe_table")
}

func TestHandleExecuteQuery(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	qt := New(db)
	ctx := context.Background()

	t.Run("successful select query", func(t *testing.T) {
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "execute_query",
				Arguments: map[string]interface{}{
					"query": "SELECT * FROM users ORDER BY id",
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
				Name: "execute_query",
				Arguments: map[string]interface{}{
					"query":      "SELECT * FROM users WHERE age > ?",
					"parameters": []interface{}{"27"},
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
				Name: "execute_query",
				Arguments: map[string]interface{}{
					"query": "INSERT INTO users (name, email, age) VALUES ('Charlie', 'charlie@example.com', 35)",
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
				Name:      "execute_query",
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
				Name: "execute_statement",
				Arguments: map[string]interface{}{
					"statement":  "INSERT INTO users (name, email, age) VALUES (?, ?, ?)",
					"parameters": []interface{}{"Charlie", "charlie@example.com", "35"},
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
				Name: "execute_statement",
				Arguments: map[string]interface{}{
					"statement":  "UPDATE users SET age = ? WHERE name = ?",
					"parameters": []interface{}{"31", "Alice"},
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
				Name: "execute_statement",
				Arguments: map[string]interface{}{
					"statement": "SELECT * FROM users",
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
				Name: "describe_table",
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
				Name: "describe_table",
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
				Name:      "describe_table",
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
