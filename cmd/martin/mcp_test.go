package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyle-visner/martin/internal/martin"
)

func TestMCPInitializeListsToolsAndWritesThroughCRM(t *testing.T) {
	store, err := martin.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err := store.Initialize(martin.Context{Actor: "owner"}, "USD"); err != nil {
		t.Fatal(err)
	}
	server := newMCPServer(martin.NewCRM(store, martin.Context{Actor: "owner"}))

	initResp := mustHandle(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`)
	if initResp.Error != nil {
		t.Fatalf("initialize failed: %+v", initResp.Error)
	}
	result := asMap(t, initResp.Result)
	if result["protocolVersion"] != "2025-03-26" {
		t.Fatalf("unexpected protocol: %#v", result)
	}

	listResp := mustHandle(t, server, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := asMap(t, listResp.Result)["tools"].([]martin.Tool)
	if len(tools) < 20 {
		t.Fatalf("expected CRM tool catalog, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if names["init"] {
		t.Fatal("MCP must not expose init; use the CLI")
	}
	for _, required := range []string{"doctor", "organization_create", "deal_create", "deal_touch", "pipeline"} {
		if !names[required] {
			t.Fatalf("missing tool %s", required)
		}
	}

	org := callStructured(t, server, 3, "organization_create", `{"name":"MCP Co","domain":"mcp.test"}`)
	orgID := asMap(t, org["organization"])["id"].(string)
	deal := callStructured(t, server, 4, "deal_create", `{
		"name":"MCP Deal","organization_id":"`+orgID+`","value_cents":2500,
		"expected_close":"2026-08-01","next_action":"Call","next_due":"2026-07-24"
	}`)
	if asMap(t, deal["deal"])["stage"] != "new" {
		t.Fatalf("expected new deal, got %#v", deal)
	}

	dir := store.Dir()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cliOut := &bytes.Buffer{}
	if err := run([]string{"--store", dir, "--actor", "owner", "organization", "list"}, cliOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cliOut.String(), "MCP Co") {
		t.Fatalf("CLI should see the organization the MCP path wrote:\n%s", cliOut.String())
	}
}

func TestMCPHTTPRequiresBearerAndServesHealth(t *testing.T) {
	store, err := martin.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err := store.Initialize(martin.Context{Actor: "owner"}, "USD"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(mcpHTTPHandler("secret-token", newMCPServer(martin.NewCRM(store, martin.Context{Actor: "owner"}))))
	defer ts.Close()

	health, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health: %d", health.StatusCode)
	}

	unauth, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", unauth.StatusCode)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("authorized initialize: %d %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Mcp-Session-Id") == "" {
		t.Fatal("expected session id on initialize")
	}
}

func TestMCPHTTPRoundTripListsTools(t *testing.T) {
	store, err := martin.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ts := httptest.NewServer(mcpHTTPHandler("secret-token", newMCPServer(martin.NewCRM(store, martin.Context{Actor: "owner"}))))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw.Error != nil {
		t.Fatalf("tools/list: %+v", raw.Error)
	}
	if _, ok := asMap(t, raw.Result)["tools"]; !ok {
		t.Fatalf("expected tools, got %#v", raw.Result)
	}
}

func TestMCPOpenerReleasesStoreSoCLICanInterleave(t *testing.T) {
	dir := t.TempDir()
	store, err := martin.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Initialize(martin.Context{Actor: "owner"}, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	openCount := 0
	server := newMCPServerFromOpener(func() (*martin.CRM, func(), error) {
		openCount++
		opened, err := martin.OpenStore(dir)
		if err != nil {
			return nil, nil, err
		}
		return martin.NewCRM(opened, martin.Context{Actor: "owner"}), func() { _ = opened.Close() }, nil
	})
	if resp := mustHandle(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"doctor","arguments":{}}}`); resp.Error != nil {
		t.Fatal(resp.Error)
	}
	var out bytes.Buffer
	if err := run([]string{"--store", dir, "--actor", "owner", "doctor"}, &out); err != nil {
		t.Fatalf("CLI should run after MCP released the store: %v", err)
	}
	if openCount != 1 {
		t.Fatalf("expected one open/close cycle, got %d", openCount)
	}
}

func TestMCPStructuredContentIsObjectForEveryTool(t *testing.T) {
	store, err := martin.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err := store.Initialize(martin.Context{Actor: "owner"}, "USD"); err != nil {
		t.Fatal(err)
	}
	server := newMCPServer(martin.NewCRM(store, martin.Context{Actor: "owner"}))

	for i, tool := range martin.ToolCatalog() {
		raw := mustHandle(t, server, `{"jsonrpc":"2.0","id":`+itoa(i+2)+`,"method":"tools/call","params":{"name":"`+tool.Name+`","arguments":{}}}`)
		if raw.Error != nil {
			t.Fatalf("%s rpc error: %+v", tool.Name, raw.Error)
		}
		body := asMap(t, raw.Result)
		assertJSONObject(t, tool.Name+" structuredContent", body["structuredContent"])
	}

	orgs := callStructured(t, server, 100, "organization_list", "{}")
	if _, ok := orgs["organizations"]; !ok {
		t.Fatalf("organization_list structuredContent must include organizations, got %#v", orgs)
	}
	audit := callStructured(t, server, 101, "audit", "{}")
	if _, ok := audit["events"]; !ok {
		t.Fatalf("audit structuredContent must include events, got %#v", audit)
	}
}

func TestMCPWrapsArrayInvokeResultAsObject(t *testing.T) {
	server := &mcpServer{invoke: func(name string, args json.RawMessage) (any, error) {
		return []string{"org:1", "deal:1"}, nil
	}}
	raw := mustHandle(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"organization_list","arguments":{}}}`)
	if raw.Error != nil {
		t.Fatal(raw.Error)
	}
	body := asMap(t, raw.Result)
	content := asMap(t, body["structuredContent"])
	assertJSONObject(t, "wrapped array", content)
	items, ok := content["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected items array, got %#v", content)
	}
}

func TestMCPStdioRoundTrip(t *testing.T) {
	store, err := martin.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	var out bytes.Buffer
	if err := serveMCPStdio(in, &out, newMCPServer(martin.NewCRM(store, martin.Context{Actor: "owner"}))); err != nil {
		t.Fatal(err)
	}
	var resp rpcResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("stdio tools/list: %+v", resp.Error)
	}
	if _, ok := asMap(t, resp.Result)["tools"]; !ok {
		t.Fatalf("stdio tools/list missing tools: %#v", resp.Result)
	}
}

func TestMCPHTTPRequiresTokenAndLoopback(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	var out bytes.Buffer
	if err := run([]string{"--store", dir, "--actor", "owner", "init", "--currency", "USD"}, &out); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MARTIN_MCP_TOKEN", "")
	t.Setenv("MARTIN_MCP_TOKEN_FILE", "")
	if err := run([]string{"--store", dir, "--actor", "owner", "mcp", "--http", "127.0.0.1:8879"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected HTTP MCP without token to fail")
	}

	t.Setenv("MARTIN_MCP_TOKEN", "secret")
	if err := run([]string{"--store", dir, "--actor", "owner", "mcp", "--http", "0.0.0.0:8879"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected non-loopback HTTP bind to fail")
	}

	t.Setenv("MARTIN_MCP_TOKEN_FILE", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("MARTIN_MCP_TOKEN", "")
	err := run([]string{"--store", dir, "--actor", "owner", "mcp", "--http", "127.0.0.1:8879"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected missing MARTIN_MCP_TOKEN_FILE to fail")
	}
	if !strings.Contains(err.Error(), "MARTIN_MCP_TOKEN_FILE") && !strings.Contains(err.Error(), "read") {
		t.Fatalf("file read error should surface, got %v", err)
	}
}

func mustHandle(t *testing.T, server *mcpServer, raw string) *rpcResponse {
	t.Helper()
	var req rpcRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	resp := server.handle(req)
	if resp == nil {
		t.Fatal("expected response")
	}
	return resp
}

func callStructured(t *testing.T, server *mcpServer, id int, name, args string) map[string]any {
	t.Helper()
	if args == "" {
		args = "{}"
	}
	raw := mustHandle(t, server, `{"jsonrpc":"2.0","id":`+itoa(id)+`,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`)
	if raw.Error != nil {
		t.Fatalf("%s rpc error: %+v", name, raw.Error)
	}
	body := asMap(t, raw.Result)
	if body["isError"] == true {
		t.Fatalf("%s tool error: %#v", name, body)
	}
	return asMap(t, body["structuredContent"])
}

func assertJSONObject(t *testing.T, label string, value any) {
	t.Helper()
	buf, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(buf)), "{") {
		t.Fatalf("%s must be a JSON object, got %s", label, buf)
	}
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	if m, ok := value.(map[string]any); ok {
		return m
	}
	buf, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf, &out); err != nil {
		t.Fatalf("not an object: %s", buf)
	}
	return out
}

func itoa(n int) string {
	buf, _ := json.Marshal(n)
	return string(buf)
}
