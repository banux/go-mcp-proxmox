// Command go-mcp-proxmox is an MCP server exposing the Proxmox VE API as tools.
//
// It communicates over stdio and authenticates against Proxmox with an API
// token configured through environment variables (see README.md). All logging
// goes to stderr: stdout is reserved for MCP protocol framing.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/banux/go-mcp-proxmox/internal/proxmox"
	"github.com/banux/go-mcp-proxmox/internal/tools"
)

const version = "0.1.0"

func main() {
	// Fail fast: an invalid configuration aborts before the MCP transport starts.
	cfg, err := proxmox.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "go-mcp-proxmox: %v\n", err)
		os.Exit(1)
	}

	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "go-mcp-proxmox: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg proxmox.Config) error {
	// stdout carries the MCP stdio framing; everything else goes to stderr.
	log.SetOutput(os.Stderr)

	client := proxmox.NewClient(cfg)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "go-mcp-proxmox",
		Version: version,
	}, nil)
	tools.Register(server, client)

	log.Printf("serving MCP over stdio, Proxmox at %s", cfg.URL)
	return server.Run(ctx, &mcp.StdioTransport{})
}
