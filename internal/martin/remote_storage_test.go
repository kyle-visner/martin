package martin

import (
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

func TestHostedBackendWorkflow(t *testing.T) {
	token := strings.Repeat("w", 64)
	sum := sha256.Sum256([]byte(token))
	authRaw, _ := json.Marshal(map[string]any{"tokens": []map[string]string{{
		"id": "owner", "role": "writer", "sha256": hex.EncodeToString(sum[:]),
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
	defer db.Close()
	api, err := server.New(server.Options{
		Store: db, Auth: auth, BackupDir: t.TempDir(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(api.Handler())
	defer httpServer.Close()

	cacheDir := t.TempDir()
	store, err := OpenRemoteStoreWithOptions(httpServer.URL, token, RemoteStoreOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{Actor: "owner"}
	if _, _, err := store.Initialize(ctx, "USD"); err != nil {
		t.Fatal(err)
	}
	org, root, err := store.CreateOrganization(ctx, Organization{Name: "Hosted Co"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Root != root || state.Organizations[org.ID].Name != org.Name {
		t.Fatalf("hosted state mismatch: %#v", state)
	}
	second, _, err := store.CreateOrganization(ctx, Organization{Name: "Incremental Co"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	cacheFiles, err := os.ReadDir(filepath.Join(cacheDir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cacheFiles) != 1 {
		t.Fatalf("expected one hosted checkpoint, got %d", len(cacheFiles))
	}
	cachePath := filepath.Join(cacheDir, "state", cacheFiles[0].Name())
	cacheInfo, err := os.Lstat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !cacheInfo.Mode().IsRegular() || cacheInfo.Mode().Perm() != 0o600 {
		t.Fatalf("checkpoint permissions = %v, want private regular 0600 file", cacheInfo.Mode())
	}
	cacheRaw, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cacheRaw), org.Name) || strings.Contains(string(cacheRaw), second.Name) {
		t.Fatal("hosted checkpoint exposed plaintext CRM data")
	}

	store, err = OpenRemoteStoreWithOptions(httpServer.URL, token, RemoteStoreOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	state, err = store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Organizations[org.ID].Name != org.Name || state.Organizations[second.ID].Name != second.Name {
		t.Fatalf("incremental checkpoint replay mismatch: %#v", state.Organizations)
	}
	if _, err := store.CreateSnapshot(ctx, "hosted-check"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(cachePath, []byte(`{"version":1,"nonce":"AA==","ciphertext":"AA=="}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err = OpenRemoteStoreWithOptions(httpServer.URL, token, RemoteStoreOptions{CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state, err = store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Organizations[org.ID].Name != org.Name || state.Organizations[second.ID].Name != second.Name {
		t.Fatalf("cold replay after corrupt checkpoint mismatch: %#v", state.Organizations)
	}
}

func TestRemoteURLPolicy(t *testing.T) {
	for _, raw := range []string{
		"http://example.com", "https://user@example.com", "https://example.com/path",
		"https://example.com?token=secret", "https://example.com/#fragment",
	} {
		if _, err := OpenRemoteStore(raw, "token"); err == nil {
			t.Fatalf("accepted unsafe URL %q", raw)
		}
	}
	if _, err := OpenRemoteStore("http://127.0.0.1:8080", "token"); err != nil {
		t.Fatalf("rejected loopback development URL: %v", err)
	}
}
