package resources

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

	sr := New(db)
	assert.NotNil(t, sr)
}

func TestGetResources(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	sr := New(db)
	resources := sr.GetResources()

	assert.Len(t, resources, 1)
	assert.Equal(t, "schema://tables", resources[0].URI)
	assert.Equal(t, "Database Tables", resources[0].Name)
}

func TestGetResourceTemplates(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	sr := New(db)
	templates := sr.GetResourceTemplates()

	assert.Len(t, templates, 1)
	assert.Equal(t, "Table Schema", templates[0].Name)
	// URITemplate is a complex type, so we'll just check it's not nil
	assert.NotNil(t, templates[0].URITemplate)
}

func TestHandleTablesList(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	sr := New(db)
	ctx := context.Background()

	request := mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "schema://tables",
		},
	}

	contents, err := sr.HandleResource(ctx, request)
	require.NoError(t, err)
	assert.Len(t, contents, 1)

	text := testutil.GetTextResourceContents(t, contents[0])
	assert.Contains(t, text, "users")
	assert.Contains(t, text, "products")
}

func TestHandleTableSchema(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	sr := New(db)
	ctx := context.Background()

	t.Run("existing table", func(t *testing.T) {
		request := mcp.ReadResourceRequest{
			Params: mcp.ReadResourceParams{
				URI: "schema://table/users",
			},
		}

		contents, err := sr.HandleResource(ctx, request)
		require.NoError(t, err)
		assert.Len(t, contents, 1)

		text := testutil.GetTextResourceContents(t, contents[0])
		assert.Contains(t, text, "table_name")
		assert.Contains(t, text, "users")
		assert.Contains(t, text, "columns")
		assert.Contains(t, text, "id")
		assert.Contains(t, text, "name")
		assert.Contains(t, text, "email")
		assert.Contains(t, text, "age")
	})

	t.Run("non-existent table", func(t *testing.T) {
		request := mcp.ReadResourceRequest{
			Params: mcp.ReadResourceParams{
				URI: "schema://table/non_existent",
			},
		}

		_, err := sr.HandleResource(ctx, request)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "table 'non_existent' not found")
	})

	t.Run("empty table name", func(t *testing.T) {
		request := mcp.ReadResourceRequest{
			Params: mcp.ReadResourceParams{
				URI: "schema://table/",
			},
		}

		_, err := sr.HandleResource(ctx, request)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "table name is required")
	})
}

func TestHandleUnknownResource(t *testing.T) {
	db := testutil.CreateTestDB(t)
	defer db.Close()

	sr := New(db)
	ctx := context.Background()

	request := mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "unknown://resource",
		},
	}

	_, err := sr.HandleResource(ctx, request)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown resource URI")
}
