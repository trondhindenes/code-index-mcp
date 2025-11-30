package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
	"github.com/trondhindenes/code-index-mcp/handlers"
)

func main() {
	s := server.NewMCPServer(
		"code-index",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// Register all tools
	handlers.RegisterTools(s)

	// Start the server
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
