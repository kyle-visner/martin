package martin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretFromEnvPrefersVariableOverFile(t *testing.T) {
	t.Setenv("DEMO_TOKEN", "from-env")
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEMO_TOKEN_FILE", path)
	got, err := SecretFromEnv("DEMO_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-env" {
		t.Fatalf("got %q", got)
	}
}

func TestSecretFromEnvReadsFileWhenVariableEmpty(t *testing.T) {
	t.Setenv("DEMO_TOKEN", "")
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("  from-file  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEMO_TOKEN_FILE", path)
	got, err := SecretFromEnv("DEMO_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-file" {
		t.Fatalf("got %q", got)
	}
}

func TestSecretFromEnvDoesNotSwallowFileReadError(t *testing.T) {
	t.Setenv("DEMO_TOKEN", "")
	t.Setenv("DEMO_TOKEN_FILE", filepath.Join(t.TempDir(), "missing-token"))
	got, err := SecretFromEnv("DEMO_TOKEN")
	if err == nil {
		t.Fatal("expected missing token file to fail")
	}
	if got != "" {
		t.Fatalf("expected empty secret on file error, got %q", got)
	}
	if !strings.Contains(err.Error(), "DEMO_TOKEN_FILE") && !strings.Contains(err.Error(), "read") {
		t.Fatalf("error should mention the file read, got %v", err)
	}
}
