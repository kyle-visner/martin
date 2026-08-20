package main

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kyle-visner/martin/internal/martin"
)

const (
	mcpProtocolLatest = "2025-03-26"
	mcpServerName     = "martin"
	mcpServerVersion  = "0.1.0-mcp"
)

var mcpProtocolSupported = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
	"2025-06-18": true,
}

type mcpServer struct {
	invoke func(name string, args json.RawMessage) (any, error)
}

func newMCPServer(crm *martin.CRM) *mcpServer {
	return &mcpServer{invoke: crm.Invoke}
}

func newMCPServerFromOpener(open func() (*martin.CRM, func(), error)) *mcpServer {
	var mu sync.Mutex
	return &mcpServer{invoke: func(name string, args json.RawMessage) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		crm, done, err := open()
		if err != nil {
			return nil, err
		}
		defer done()
		return crm.Invoke(name, args)
	}}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (s *mcpServer) handle(req rpcRequest) *rpcResponse {
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return rpcErr(req.ID, -32600, "jsonrpc must be 2.0", nil)
	}
	switch req.Method {
	case "initialize":
		return rpcOK(req.ID, s.initialize(req.Params))
	case "notifications/initialized", "initialized":
		return nil
	case "ping":
		return rpcOK(req.ID, map[string]any{})
	case "tools/list":
		return rpcOK(req.ID, map[string]any{"tools": martin.ToolCatalog()})
	case "tools/call":
		result, err := s.callTool(req.Params)
		if err != nil {
			return rpcOK(req.ID, toolError(err))
		}
		return rpcOK(req.ID, result)
	case "resources/list":
		return rpcOK(req.ID, map[string]any{"resources": []any{}})
	case "prompts/list":
		return rpcOK(req.ID, map[string]any{"prompts": []any{}})
	default:
		return rpcErr(req.ID, -32601, "method not found: "+req.Method, nil)
	}
}

func (s *mcpServer) initialize(params json.RawMessage) map[string]any {
	version := mcpProtocolLatest
	var in struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(emptyObject(params), &in); err == nil && mcpProtocolSupported[in.ProtocolVersion] {
		version = in.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    mcpServerName,
			"version": mcpServerVersion,
		},
		"instructions": "Martin is the CRM. Initialize with the CLI (`martin --actor owner init --currency USD`); there is no MCP init tool. Use integer minor units, YYYY-MM-DD dates, and opaque Martin IDs. Pipeline stages are new, qualified, proposal, won, lost. Every open deal has exactly one next action. Prefer deal_touch when logging an interaction and setting the next step. Writes are martin.* facts only. The process actor is bound by the host and cannot be changed by a tool.",
	}
}

func (s *mcpServer) callTool(params json.RawMessage) (map[string]any, error) {
	var in struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(emptyObject(params), &in); err != nil {
		return nil, martinAppErr("validation_error", "invalid tools/call params")
	}
	if !martin.KnownTool(in.Name) {
		return nil, martinAppErr("validation_error", "unknown tool "+in.Name)
	}
	result, err := s.invoke(in.Name, in.Arguments)
	if err != nil {
		return nil, err
	}
	structured := mcpStructuredContent(result)
	text, err := martin.EncodeJSON(structured)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(text)},
		},
		"structuredContent": structured,
		"isError":           false,
	}, nil
}

// mcpStructuredContent keeps MCP structuredContent a JSON object. Clients that
// validate the field as a record reject a top-level array.
func mcpStructuredContent(result any) any {
	if result == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return result
	}
	if isJSONObject(raw) {
		return result
	}
	return map[string]any{"items": result}
}

func isJSONObject(raw []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(raw)), "{")
}

func toolError(err error) map[string]any {
	payload := map[string]string{"code": "error", "message": err.Error()}
	var appErr *martin.AppError
	if errors.As(err, &appErr) {
		payload["code"] = string(appErr.Code)
		payload["message"] = appErr.Message
	}
	text, _ := json.Marshal(payload)
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(text)},
		},
		"structuredContent": payload,
		"isError":           true,
	}
}

func martinAppErr(code, message string) error {
	return &martin.AppError{Code: martin.ErrorCode(code), Message: message}
}

func rpcOK(id json.RawMessage, result any) *rpcResponse {
	if isNullID(id) {
		return nil
	}
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func rpcErr(id json.RawMessage, code int, message string, data any) *rpcResponse {
	if isNullID(id) {
		return nil
	}
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message, Data: data}}
}

func isNullID(id json.RawMessage) bool {
	return len(bytesTrim(id)) == 0 || string(id) == "null"
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func emptyObject(raw json.RawMessage) []byte {
	raw = bytesTrim(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("{}")
	}
	return raw
}

func (a app) mcp(args []string, _ io.Writer) error {
	fs := newFlagSet("mcp")
	httpAddr := fs.String("http", "", "Streamable HTTP listen address, for example 127.0.0.1:8879. Default is stdio.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := a.storeDir
	hosted := strings.TrimSpace(a.jaybaseURL) != ""
	cacheDir := a.cacheDir
	ctx := a.ctx
	if err := a.store.Close(); err != nil {
		return err
	}
	open := func() (*martin.CRM, func(), error) {
		var store *martin.Store
		var err error
		if hosted {
			store, err = martin.OpenRemoteStoreWithOptions(a.jaybaseURL, os.Getenv("JAYBASE_TOKEN"), martin.RemoteStoreOptions{
				CacheDir: cacheDir,
			})
		} else {
			store, err = martin.OpenStore(dir)
		}
		if err != nil {
			return nil, nil, err
		}
		return martin.NewCRM(store, ctx), func() { _ = store.Close() }, nil
	}
	server := newMCPServerFromOpener(open)
	if strings.TrimSpace(*httpAddr) != "" {
		if err := requireLoopbackHTTPAddr(*httpAddr); err != nil {
			return err
		}
		mcpToken, err := martin.SecretFromEnv("MARTIN_MCP_TOKEN")
		if err != nil {
			return err
		}
		if mcpToken == "" {
			return fmt.Errorf("HTTP MCP requires MARTIN_MCP_TOKEN or MARTIN_MCP_TOKEN_FILE")
		}
		return serveMCPHTTP(*httpAddr, mcpToken, server)
	}
	return serveMCPStdio(os.Stdin, os.Stdout, server)
}

func requireLoopbackHTTPAddr(addr string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("http address must be host:port, for example 127.0.0.1:8879")
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return fmt.Errorf("HTTP MCP must bind loopback (for example 127.0.0.1:8879)")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("HTTP MCP must bind loopback (for example 127.0.0.1:8879)")
	}
	return nil
}

func serveMCPStdio(in io.Reader, out io.Writer, server *mcpServer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if resp := rpcErr(nil, -32700, "parse error", nil); resp != nil {
				_ = enc.Encode(resp)
			}
			continue
		}
		if resp := server.handle(req); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func serveMCPHTTP(addr, token string, server *mcpServer) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           mcpHTTPHandler(token, server),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "martin mcp listening on %s\n", ln.Addr().String())
	return srv.Serve(ln)
}

func mcpHTTPHandler(token string, server *mcpServer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"server":"martin-mcp"}`))
	})
	handler := func(w http.ResponseWriter, r *http.Request) {
		setMCPHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !bearerOK(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="martin-mcp"`)
			http.Error(w, `{"code":"permission_denied","message":"invalid bearer token"}`, http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeRPC(w, rpcErr(nil, -32700, "parse error", nil))
			return
		}
		if req.Method == "initialize" && r.Header.Get("Mcp-Session-Id") == "" {
			w.Header().Set("Mcp-Session-Id", newSessionID())
		} else if sid := r.Header.Get("Mcp-Session-Id"); sid != "" {
			w.Header().Set("Mcp-Session-Id", sid)
		}
		resp := server.handle(req)
		if resp == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeRPC(w, resp)
	}
	mux.HandleFunc("/mcp", handler)
	mux.HandleFunc("/", handler)
	return mux
}

func setMCPHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Mcp-Session-Id, MCP-Protocol-Version")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
}

func writeRPC(w http.ResponseWriter, resp *rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func bearerOK(header, token string) bool {
	got := strings.TrimSpace(header)
	if strings.HasPrefix(strings.ToLower(got), "bearer ") {
		got = strings.TrimSpace(got[7:])
	}
	if token == "" || got == "" {
		return false
	}
	if len(got) != len(token) {
		subtle.ConstantTimeCompare([]byte(token), []byte(token))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func newSessionID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
