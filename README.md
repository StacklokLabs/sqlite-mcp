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

### Examples

```bash
# Start server with custom database and port
./sqlite-mcp -db ./mydata.db -addr :9000

# Start server in read-write mode
./sqlite-mcp -db ./mydata.db -read-write

# Use environment variable for port
MCP_PORT=8080 ./sqlite-mcp -db ./mydata.db

# Read-only mode (default)
./sqlite-mcp -db ./mydata.db
```

## Development

### Prerequisites

- Go 1.21 or later
- SQLite database file

### Building

```bash
go build -o sqlite-mcp .
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run tests for specific package
go test ./internal/database -v
```

### Linting

```bash
golangci-lint run
```

## Project Structure

```
sqlite-mcp/
├── main.go                    # Entry point and server setup
├── internal/
│   ├── database/             # Database connection and operations
│   │   ├── db.go
│   │   └── db_test.go
│   ├── tools/                # MCP tools implementation
│   │   ├── query.go
│   │   └── query_test.go
│   ├── resources/            # MCP resources implementation
│   │   ├── schema.go
│   │   └── schema_test.go
│   └── testutil/             # Common test utilities
│       └── testdb.go
├── testdata/                 # Test data files
├── go.mod
├── go.sum
└── README.md
```

## API Endpoints

When running, the server exposes the following endpoints:

- `GET /mcp/sse`: Server-Sent Events endpoint for MCP communication
- `POST /mcp/message`: Message endpoint for sending MCP requests

## Security Considerations

- **Read-Only Mode**: By default, the server runs in read-only mode to prevent accidental data modification
- **Database Validation**: The server validates that the database file exists before starting
- **Input Validation**: All SQL queries and parameters are validated before execution
- **Error Handling**: Comprehensive error handling prevents information leakage

## Dependencies

- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go): MCP protocol implementation
- [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3): SQLite driver for Go
- [stretchr/testify](https://github.com/stretchr/testify): Testing toolkit

## License

This project is part of the Stacklok ecosystem.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run tests and linting
6. Submit a pull request

## Troubleshooting

### Common Issues

1. **Database file not found**: Ensure the database file exists and the path is correct
2. **Permission denied**: Check file permissions for the database file
3. **Port already in use**: Use a different port with `-addr` flag or `MCP_PORT` environment variable

### Debugging

Enable verbose logging by checking the server output. The server logs all important events including:

- Server startup and configuration
- Database connection status
- Available tools and resources
- Error messages and stack traces