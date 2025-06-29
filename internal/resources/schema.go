// Package resources provides MCP resources for SQLite database schema information
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/StacklokLabs/sqlite-mcp/internal/database"
)

// SchemaResources provides MCP resources for SQLite database schema information
type SchemaResources struct {
	db *database.DB
}

// New creates a new SchemaResources instance
func New(db *database.DB) *SchemaResources {
	return &SchemaResources{db: db}
}

// GetResources returns all available MCP resources
func (*SchemaResources) GetResources() []mcp.Resource {
	return []mcp.Resource{
		mcp.NewResource(
			"schema://tables",
			"Database Tables",
			mcp.WithResourceDescription("List of all tables in the SQLite database"),
			mcp.WithMIMEType("application/json"),
		),
	}
}

// GetResourceTemplates returns all available MCP resource templates
func (*SchemaResources) GetResourceTemplates() []mcp.ResourceTemplate {
	return []mcp.ResourceTemplate{
		mcp.NewResourceTemplate(
			"schema://table/{name}",
			"Table Schema",
			mcp.WithTemplateDescription("Schema information for a specific table"),
			mcp.WithTemplateMIMEType("application/json"),
		),
	}
}

// HandleResource handles MCP resource requests
func (sr *SchemaResources) HandleResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	uri := request.Params.URI

	switch {
	case uri == "schema://tables":
		return sr.handleTablesList(ctx)
	case strings.HasPrefix(uri, "schema://table/"):
		tableName := strings.TrimPrefix(uri, "schema://table/")
		return sr.handleTableSchema(ctx, tableName)
	default:
		return nil, fmt.Errorf("unknown resource URI: %s", uri)
	}
}

// handleTablesList returns a list of all tables
func (sr *SchemaResources) handleTablesList(_ context.Context) ([]mcp.ResourceContents, error) {
	tables, err := sr.db.GetTables()
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	jsonData, err := json.MarshalIndent(tables, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tables: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "schema://tables",
			MIMEType: "application/json",
			Text:     string(jsonData),
		},
	}, nil
}

// handleTableSchema returns schema information for a specific table
func (sr *SchemaResources) handleTableSchema(_ context.Context, tableName string) ([]mcp.ResourceContents, error) {
	if tableName == "" {
		return nil, fmt.Errorf("table name is required")
	}

	schema, err := sr.db.GetTableSchema(tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get table schema for '%s': %w", tableName, err)
	}

	if len(schema) == 0 {
		return nil, fmt.Errorf("table '%s' not found", tableName)
	}

	// Create a more structured response
	tableInfo := map[string]interface{}{
		"table_name": tableName,
		"columns":    schema,
	}

	jsonData, err := json.MarshalIndent(tableInfo, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal table schema: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      fmt.Sprintf("schema://table/%s", tableName),
			MIMEType: "application/json",
			Text:     string(jsonData),
		},
	}, nil
}
