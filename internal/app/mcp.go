package app

import (
	"fmt"
	"os"

	"github.com/bloodstalk1/arkroute/internal/buildinfo"
	"github.com/bloodstalk1/arkroute/internal/mcp"
)

// RunMCP starts the MCP server on STDIO. It reads JSON-RPC 2.0 requests
// from stdin and writes responses to stdout. This is intended for use
// with MCP-compatible clients like Claude Desktop or Claude Code:
//
//	claude mcp add-server arkroute -- arkroute mcp
func RunMCP(configPath string) error {
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("config not found at %s: %w", configPath, err)
	}
	srv := mcp.New(
		mcp.ServerInfo{Name: "arkroute", Version: buildinfo.Summary()},
		mcp.Tools(),
		mcp.Handler(configPath),
	)
	// Block until stdin closes.
	return srv.Run()
}
