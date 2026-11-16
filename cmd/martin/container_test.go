package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyle-visner/jaybase"
	"github.com/kyle-visner/jaybase/server"
)

// Container deployments run `martin mcp --http :8879` inside a private
// network and hand secrets over as files. These tests pin both halves.

func TestValidateHTTPAddrAcceptsAnyInterface(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8879", "localhost:8879", ":8879", "0.0.0.0:8879", "[::]:8879", "[::1]:8879", "10.0.0.5:8879"} {
		if err := validateHTTPAddr(addr); err != nil {
			t.Fatalf("validateHTTPAddr(%q) = %v, want nil", addr, err)
		}
	}
	for _, addr := range []string{"", "8879", "127.0.0.1", "[::1]", "127.0.0.1:", "a:b:c"} {
		if err := validateHTTPAddr(addr); err == nil {
			t.Fatalf("validateHTTPAddr(%q) = nil, want error", addr)
		}
	}
}

func TestCLIReadsJaybaseTokenFile(t *testing.T) {
	token := strings.Repeat("f", 64)
	url := startJaybase(t, token)
	t.Setenv("JAYBASE_TOKEN", "")

	tokenFile := filepath.Join(t.TempDir(), "store_token")
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAYBASE_TOKEN_FILE", tokenFile)
	var out bytes.Buffer
	if err := run([]string{"--jaybase-url", url, "--actor", "owner", "init", "--currency", "USD"}, &out); err != nil {
		t.Fatalf("init with JAYBASE_TOKEN_FILE: %v", err)
	}
	out.Reset()
	if err := run([]string{"--jaybase-url", url, "--actor", "owner", "organization", "create", "--name", "File Token Co"}, &out); err != nil {
		t.Fatalf("write with JAYBASE_TOKEN_FILE: %v", err)
	}
	if !strings.Contains(out.String(), "File Token Co") {
		t.Fatalf("unexpected output: %s", out.String())
	}

	wrong := filepath.Join(t.TempDir(), "wrong_token")
	if err := os.WriteFile(wrong, []byte(strings.Repeat("x", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAYBASE_TOKEN_FILE", wrong)
	if err := run([]string{"--jaybase-url", url, "--actor", "owner", "organization", "list"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a wrong token from JAYBASE_TOKEN_FILE to be refused")
	}

	t.Setenv("JAYBASE_TOKEN_FILE", filepath.Join(t.TempDir(), "missing"))
	err := run([]string{"--jaybase-url", url, "--actor", "owner", "organization", "list"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "JAYBASE_TOKEN_FILE") {
		t.Fatalf("expected a missing JAYBASE_TOKEN_FILE to fail by name, got %v", err)
	}

	// JAYBASE_TOKEN wins over the file, as with every other _FILE secret.
	t.Setenv("JAYBASE_TOKEN", token)
	if err := run([]string{"--jaybase-url", url, "--actor", "owner", "organization", "list"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("JAYBASE_TOKEN should take precedence over JAYBASE_TOKEN_FILE: %v", err)
	}
}

func startJaybase(t *testing.T, token string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(token))
	authRaw, _ := json.Marshal(map[string]any{"tokens": []map[string]string{{
		"id": "martin", "role": "writer", "sha256": hex.EncodeToString(sum[:]),
	}}})
	authPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authPath, authRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	auth, err := server.LoadAuthenticator(authPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := jaybase.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	api, err := server.New(server.Options{
		Store: db, Auth: auth, BackupDir: t.TempDir(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(api.Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}
