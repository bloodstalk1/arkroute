// Package mcp implements a minimal Model Context Protocol (MCP) server
// over STDIO transport (JSON-RPC 2.0). It exposes arkroute operations
// as MCP tools so that any MCP-compatible client (Claude Desktop,
// Claude Code, Cursor) can inspect and control the local gateway.
//
// Specification: https://spec.modelcontextprotocol.io/specification/2024-11-05/
package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// --- JSON-RPC 2.0 types ---

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCP protocol version.
const protocolVersion = "2024-11-05"

// ServerInfo is returned during initialize.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool definition (tools/list response shape).
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolCallResult is the content of a tools/call response.
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// --- Server ---

// Server handles MCP requests over STDIO. Call Run() to start the event loop.
type Server struct {
	mu     sync.Mutex
	writer io.Writer
	info   ServerInfo
	tools  []Tool
	call   func(toolName string, args map[string]any) (string, error)
}

// New creates a Server with the given identity and tool set.
func New(info ServerInfo, tools []Tool, call func(toolName string, args map[string]any) (string, error)) *Server {
	return &Server{info: info, tools: tools, call: call}
}

// Run starts the STDIO event loop. It reads JSON-RPC messages from os.Stdin
// and writes responses to os.Stdout. It blocks until stdin is closed or an
// unrecoverable error occurs.
func (s *Server) Run() error {
	s.writer = os.Stdout
	decoder := json.NewDecoder(os.Stdin)
	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("mcp: decode: %w", err)
		}
		if req.JSONRPC != "2.0" {
			continue
		}
		s.handle(req)
	}
}

func (s *Server) handle(req Request) {
	switch req.Method {
	case "initialize":
		s.respondInitialize(req.ID)
	case "notifications/initialized":
		// No response needed per spec.
	case "tools/list":
		s.respondToolsList(req.ID)
	case "tools/call":
		s.respondToolsCall(req.ID, req.Params)
	default:
		s.write(Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Data    any    `json:"data,omitempty"`
			}{Code: -32601, Message: "method not found: " + req.Method},
		})
	}
}

func (s *Server) write(resp Response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.Marshal(resp)
	fmt.Fprintf(s.writer, "%s\n", data)
}

func (s *Server) respondInitialize(id any) {
	s.write(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"protocolVersion": protocolVersion,
			"serverInfo":      s.info,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		},
	})
}

func (s *Server) respondToolsList(id any) {
	s.write(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"tools": s.tools,
		},
	})
}

func (s *Server) respondToolsCall(id any, params json.RawMessage) {
	var input struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		s.write(Response{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Data    any    `json:"data,omitempty"`
			}{Code: -32602, Message: "invalid params: " + err.Error()},
		})
		return
	}
	if s.call == nil {
		s.writeToolResult(id, ToolCallResult{Content: []ToolContent{{Type: "text", Text: "no tool handler configured"}}, IsError: true})
		return
	}
	text, err := s.call(input.Name, input.Arguments)
	if err != nil {
		s.writeToolResult(id, ToolCallResult{Content: []ToolContent{{Type: "text", Text: err.Error()}}, IsError: true})
		return
	}
	s.writeToolResult(id, ToolCallResult{Content: []ToolContent{{Type: "text", Text: text}}})
}

func (s *Server) writeToolResult(id any, result ToolCallResult) {
	s.write(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

// --- Helpers for building tools ---

// SimpleSchema builds a basic JSON Schema object with properties.
func SimpleSchema(props map[string]any) json.RawMessage {
	schema := map[string]any{"type": "object", "properties": props}
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	data, _ := json.Marshal(schema)
	return data
}

// NoArgsSchema returns an empty object schema.
func NoArgsSchema() json.RawMessage {
	data, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{}})
	return data
}
