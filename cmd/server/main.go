// Package main provides the entry point for the sqlite-mcp server application
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/StacklokLabs/sqlite-mcp/internal/database"
	"github.com/StacklokLabs/sqlite-mcp/internal/resources"
	"github.com/StacklokLabs/sqlite-mcp/internal/tools"
)

const (
	defaultDB = "./database.db"

	// Transport types
	transportSSE            = "sse"
	transportStreamableHTTP = "streamable-http"
)

func main() {
	config := parseFlags()
	if config.help {
		showHelp()
		return
	}

	ctx := setupContext()
	db := initializeDatabase(config.dbPath, config.readWrite)
	defer closeDatabase(db)

	mcpServer := createMCPServer()
	registerToolsAndResources(mcpServer, db, config.readWrite)

	runServer(ctx, mcpServer, config.addr, config.dbPath, config.readWrite, config.transport)
}

// Config holds the parsed command line configuration
type Config struct {
	dbPath    string
	addr      string
	readWrite bool
	transport string
	help      bool
}

// parseFlags parses command line flags and returns configuration
func parseFlags() Config {
	dbPath := flag.String("db", defaultDB, "Path to SQLite database file")
	addr := flag.String("addr", getDefaultAddress(), "Address to listen on")
	readWrite := flag.Bool("read-write", false,
		"Whether to allow write operations on the database. When false, the server operates in read-only mode")
	transport := flag.String("transport", getDefaultTransport(),
		"Transport protocol: 'sse' or 'streamable-http'. Also via MCP_TRANSPORT env var")
	help := flag.Bool("help", false, "Show help message")

	flag.Parse()

	return Config{
		dbPath:    *dbPath,
		addr:      *addr,
		readWrite: *readWrite,
		transport: *transport,
		help:      *help,
	}
}

// showHelp displays the help message
func showHelp() {
	fmt.Printf("SQLite MCP Server - A Model Context Protocol server for SQLite databases\n\n")
	fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
	fmt.Printf("Options:\n")
	flag.PrintDefaults()
	fmt.Printf("\nEnvironment Variables:\n")
	fmt.Printf("  MCP_PORT       Port to listen on (overrides -addr flag port)\n")
	fmt.Printf("  MCP_TRANSPORT  Transport protocol: 'sse' or 'streamable-http' (default: streamable-http)\n")
	fmt.Printf("\nExample:\n")
	fmt.Printf("  %s -db ./mydata.db -addr :8080\n", os.Args[0])
	fmt.Printf("  MCP_PORT=9000 %s -db ./mydata.db\n", os.Args[0])
	fmt.Printf("  MCP_TRANSPORT=sse %s -db ./mydata.db\n", os.Args[0])
}

// setupContext creates a cancellable context with signal handling
func setupContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received shutdown signal")
		cancel()
	}()

	return ctx
}

// initializeDatabase validates and initializes the database connection
func initializeDatabase(dbPath string, readWrite bool) *database.DB {
	// Validate database file exists (skip check for in-memory databases)
	if dbPath != ":memory:" {
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			log.Fatalf("Database file does not exist: %s", dbPath)
		}
	}

	// Initialize database connection with read-only mode detection
	db, err := database.New(dbPath, !readWrite)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	return db
}

// closeDatabase safely closes the database connection
func closeDatabase(db *database.DB) {
	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
}

// createMCPServer creates and configures the MCP server
func createMCPServer() *server.MCPServer {
	return server.NewMCPServer(
		"sqlite-mcp",
		"1.0.0",
		server.WithToolCapabilities(false), // No tool list change notifications
		server.WithResourceCapabilities(false, false), // No resource subscriptions or change notifications
		server.WithLogging(),                          // Enable logging
		server.WithRecovery(),                         // Enable panic recovery
	)
}

// registerToolsAndResources registers tools and resources with the MCP server
func registerToolsAndResources(mcpServer *server.MCPServer, db *database.DB, readWrite bool) {
	// Initialize tools and resources
	queryTools := tools.New(db)
	schemaResources := resources.New(db)

	// Register tools based on read-write mode
	for _, tool := range queryTools.GetTools() {
		// In read-only mode, skip write operations
		if !readWrite && (tool.Name == "execute_statement") {
			log.Printf("Skipping write tool '%s' in read-only mode", tool.Name)
			continue
		}
		mcpServer.AddTool(tool, queryTools.HandleTool)
	}

	// Register resources
	for _, resource := range schemaResources.GetResources() {
		mcpServer.AddResource(resource, schemaResources.HandleResource)
	}

	// Register resource templates
	for _, template := range schemaResources.GetResourceTemplates() {
		mcpServer.AddResourceTemplate(template, schemaResources.HandleResource)
	}
}

// runServer starts the server and handles shutdown
func runServer(ctx context.Context, mcpServer *server.MCPServer, addr, dbPath string, readWrite bool, transport string) {
	// Create the appropriate transport server
	var transportServer interface {
		Start(string) error
		Shutdown(context.Context) error
	}

	switch strings.ToLower(transport) {
	case transportStreamableHTTP:
		log.Println("Using streamable-http transport")
		transportServer = server.NewStreamableHTTPServer(mcpServer)
	case transportSSE:
		log.Println("Using SSE transport")
		transportServer = server.NewSSEServer(mcpServer)
	default:
		log.Fatalf("Invalid transport: %s. Must be 'sse' or 'streamable-http'", transport)
	}

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		logServerStart(addr, dbPath, readWrite, transport)
		errChan <- transportServer.Start(addr)
	}()

	// Wait for signal or error
	select {
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
	case <-ctx.Done():
		log.Printf("Shutting down server...")
		if err := transportServer.Shutdown(ctx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}

	log.Println("Server shutdown complete")
}

// logServerStart logs server startup information
func logServerStart(addr, dbPath string, readWrite bool, transport string) {
	mode := "read-only"
	if readWrite {
		mode = "read-write"
	}

	log.Printf("Starting SQLite MCP Server on %s (%s mode, %s transport)", addr, mode, transport)
	log.Printf("Database: %s", dbPath)

	if readWrite {
		log.Printf("Available tools: execute_query, execute_statement, list_tables, describe_table")
	} else {
		log.Printf("Available tools: execute_query, list_tables, describe_table")
	}
	log.Printf("Available resources: schema://tables, schema://table/{name}")
}

// getDefaultAddress returns the address to listen on based on MCP_PORT environment variable.
// If the environment variable is not set, returns ":8080".
// If set, validates that the port is valid and returns ":<port>".
func getDefaultAddress() string {
	port := "8080"
	if envPort := os.Getenv("MCP_PORT"); envPort != "" {
		if portNum, err := strconv.Atoi(envPort); err == nil {
			if portNum >= 0 && portNum <= 65535 {
				port = envPort
			} else {
				log.Printf("Invalid MCP_PORT value: %s (must be between 0 and 65535), using default port 8080", envPort)
			}
		} else {
			log.Printf("Invalid MCP_PORT value: %s (must be a valid number), using default port 8080", envPort)
		}
	}
	return ":" + port
}

// getDefaultTransport returns the transport to use based on MCP_TRANSPORT environment variable.
// If the environment variable is not set, returns "streamable-http".
// Valid values are "sse" and "streamable-http".
func getDefaultTransport() string {
	defaultTransport := transportStreamableHTTP

	transportEnv := os.Getenv("MCP_TRANSPORT")
	if transportEnv == "" {
		return defaultTransport
	}

	// Normalize the transport value
	transport := strings.ToLower(strings.TrimSpace(transportEnv))

	// Validate the transport value
	if transport != transportSSE && transport != transportStreamableHTTP {
		log.Printf("Invalid MCP_TRANSPORT: %s, using default: %s",
			transportEnv, defaultTransport)
		return defaultTransport
	}

	return transport
}
