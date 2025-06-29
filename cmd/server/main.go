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
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/StacklokLabs/sqlite-mcp/internal/database"
	"github.com/StacklokLabs/sqlite-mcp/internal/resources"
	"github.com/StacklokLabs/sqlite-mcp/internal/tools"
)

const (
	defaultDB = "./database.db"
)

func main() {
	// Parse command line flags
	dbPath := flag.String("db", defaultDB, "Path to SQLite database file")
	addr := flag.String("addr", getDefaultAddress(), "Address to listen on")
	readWrite := flag.Bool("read-write", false,
		"Whether to allow write operations on the database. When false, the server operates in read-only mode")
	help := flag.Bool("help", false, "Show help message")

	flag.Parse()

	if *help {
		fmt.Printf("SQLite MCP Server - A Model Context Protocol server for SQLite databases\n\n")
		fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
		fmt.Printf("Options:\n")
		flag.PrintDefaults()
		fmt.Printf("\nEnvironment Variables:\n")
		fmt.Printf("  MCP_PORT    Port to listen on (overrides -addr flag port)\n")
		fmt.Printf("\nExample:\n")
		fmt.Printf("  %s -db ./mydata.db -addr :8080\n", os.Args[0])
		fmt.Printf("  MCP_PORT=9000 %s -db ./mydata.db\n", os.Args[0])
		return
	}

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received shutdown signal")
		cancel()
	}()

	// Validate database file exists (skip check for in-memory databases)
	if *dbPath != ":memory:" {
		if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
			log.Fatalf("Database file does not exist: %s", *dbPath)
		}
	}

	// Initialize database connection with read-only mode detection
	db, err := database.New(*dbPath, !*readWrite)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	// Create MCP server with capabilities
	mcpServer := server.NewMCPServer(
		"sqlite-mcp",
		"1.0.0",
		server.WithToolCapabilities(false), // No tool list change notifications
		server.WithResourceCapabilities(false, false), // No resource subscriptions or change notifications
		server.WithLogging(),                          // Enable logging
		server.WithRecovery(),                         // Enable panic recovery
	)

	// Initialize tools and resources
	queryTools := tools.New(db)
	schemaResources := resources.New(db)

	// Register tools based on read-write mode
	for _, tool := range queryTools.GetTools() {
		// In read-only mode, skip write operations
		if !*readWrite && (tool.Name == "execute_statement") {
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

	// Create SSE server with defaults (following OSV pattern)
	sseServer := server.NewSSEServer(mcpServer)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		mode := "read-write"
		if !*readWrite {
			mode = "read-only"
		}
		log.Printf("Starting SQLite MCP Server on %s (%s mode)", *addr, mode)
		log.Printf("Database: %s", *dbPath)

		if *readWrite {
			log.Printf("Available tools: execute_query, execute_statement, list_tables, describe_table")
		} else {
			log.Printf("Available tools: execute_query, list_tables, describe_table")
		}
		log.Printf("Available resources: schema://tables, schema://table/{name}")

		errChan <- sseServer.Start(*addr)
	}()

	// Wait for signal or error
	select {
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
	case <-ctx.Done():
		log.Printf("Shutting down server...")
	}

	log.Println("Server shutdown complete")
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
