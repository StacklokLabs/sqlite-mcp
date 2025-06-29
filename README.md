# SQLite MCP Server

A Model Context Protocol (MCP) server that provides tools and resources for querying SQLite databases. This server enables LLMs to interact with SQLite databases through a standardized protocol.

## Features

- **Database Query Tools**: Execute SELECT queries and data modification statements
- **Schema Resources**: Access database schema information and table structures
- **SSE Transport**: Server-Sent Events transport for real-time communication
- **Read-Only Mode**: Optional read-only mode for safe database access
- **Comprehensive Testing**: Full test coverage with testify
- **Linting**: Code quality ensured with golangci-lint

## Tools

The server provides the following MCP tools:

- `execute_query`: Execute SELECT queries against the SQLite database
- `execute_statement`: Execute INSERT, UPDATE, or DELETE statements (only in read-write mode)
- `list_tables`: List all tables in the database
- `describe_table`: Get schema information for a specific table

## Resources

The server provides the following MCP resources:

- `schema://tables`: List of all tables in the database
- `schema://table/{name}`: Schema information for a specific table

## Installation

```bash
go build -o sqlite-mcp .
```

## Usage

### Basic Usage

```bash
./sqlite-mcp -db ./path/to/database.db
```

### Command Line Options

```bash
./sqlite-mcp [options]

Options:
  -addr string
        Address to listen on (default ":8080")
  -db string
        Path to SQLite database file (default "./database.db")
  -help
        Show help message
  -read-write
        Whether to allow write operations on the database. When false, the server operates in read-only mode
```

### Environment Variables

- `MCP_PORT`: Port to listen on (overrides -addr flag port)

## Development

### Prerequisites

- Go 1.21 or later
- SQLite database file

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run tests and linting
6. Submit a pull request
