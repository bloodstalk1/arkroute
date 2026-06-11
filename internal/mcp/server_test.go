package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

// testServer runs the MCP server with os.Pipe() as stdin/stdout and returns
// a function that sends a JSON-RPC request and reads the response.
func testServer(t *testing.T, configPath string) func(method string, params json.RawMessage) Response {
	t.Helper()

	rIn, wIn, _ := os.Pipe()
	rOut, wOut, _ := os.Pipe()

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	os.Stdin = rIn
	os.Stdout = wOut
	t.Cleanup(func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		rIn.Close()
		wIn.Close()
		rOut.Close()
		wOut.Close()
	})

	srv := New(ServerInfo{Name: "arkroute", Version: "test"}, Tools(), Handler(configPath))
	go func() {
		_ = srv.Run()
	}()

	decoder := json.NewDecoder(rOut)

	return func(method string, params json.RawMessage) Response {
		req := Request{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
		data, _ := json.Marshal(req)
		_, _ = wIn.Write(append(data, '\n'))

		var resp Response
		if err := decoder.Decode(&resp); err != nil {
			t.Fatalf("decode response for %s: %v", method, err)
		}
		return resp
	}
}

func writeConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMCPInitialize(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	call := testServer(t, writeConfig(t, cfg))

	resp := call("initialize", nil)
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result is not a map")
	}
	info, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("serverInfo missing")
	}
	if info["name"] != "arkroute" {
		t.Errorf("name = %q, want arkroute", info["name"])
	}
}

func TestMCPToolsList(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	call := testServer(t, writeConfig(t, cfg))

	resp := call("tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result is not a map")
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatal("tools is not a list")
	}
	if len(tools) != 5 {
		t.Errorf("got %d tools, want 5", len(tools))
	}
}

func TestMCPStatusTool(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	call := testServer(t, writeConfig(t, cfg))

	params, _ := json.Marshal(map[string]any{"name": "arkroute_status", "arguments": map[string]any{}})
	resp := call("tools/call", params)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %+v", resp.Error)
	}
	result, _ := resp.Result.(map[string]any)
	content, _ := result["content"].([]any)
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "Arkroute Gateway Status") {
		t.Errorf("status missing header: %s", text)
	}
}

func TestMCPTestToolRequiresModel(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	call := testServer(t, writeConfig(t, cfg))

	params, _ := json.Marshal(map[string]any{"name": "arkroute_test", "arguments": map[string]any{}})
	resp := call("tools/call", params)
	result, _ := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error for missing model param")
	}
}

func TestMCPTestToolFindsModel(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	call := testServer(t, writeConfig(t, cfg))

	params, _ := json.Marshal(map[string]any{
		"name": "arkroute_test",
		"arguments": map[string]any{
			"model":  "sonnet-or",
			"prompt": "hello",
		},
	})
	resp := call("tools/call", params)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %+v", resp.Error)
	}
	result, _ := resp.Result.(map[string]any)
	content, _ := result["content"].([]any)
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "Test Request") {
		t.Errorf("test tool missing header: %s", text)
	}
}

func TestMCPSimpleSchema(t *testing.T) {
	schema := SimpleSchema(map[string]any{
		"model":  map[string]any{"type": "string"},
		"prompt": map[string]any{"type": "string"},
	})
	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["type"] != "object" {
		t.Errorf("type = %v, want object", parsed["type"])
	}
	required := parsed["required"].([]any)
	if len(required) != 2 {
		t.Errorf("required = %v, want 2 fields", required)
	}
}

// verify io import
var _ io.Reader = &bytes.Buffer{}
