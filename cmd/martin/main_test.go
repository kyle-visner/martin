package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCLIHappyPath(t *testing.T) {
	store := filepath.Join(t.TempDir(), "store")
	runJSON := func(args ...string) map[string]any {
		t.Helper()
		var out bytes.Buffer
		full := append([]string{"--store", store}, args...)
		if err := run(full, &out); err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("decode %v output %q: %v", args, out.String(), err)
		}
		return result
	}
	runJSON("init", "--currency", "USD")
	orgResult := runJSON("organization", "create", "--name", "CLI Co", "--domain", "cli.test")
	org := orgResult["organization"].(map[string]any)
	orgID := org["id"].(string)
	dealResult := runJSON(
		"deal", "create", "--name", "CLI Deal", "--organization-id", orgID,
		"--value-cents", "2500", "--expected-close", "2026-08-01",
		"--next-action", "Call", "--next-due", "2026-07-24",
	)
	deal := dealResult["deal"].(map[string]any)
	if deal["stage"] != "new" {
		t.Fatalf("deal output = %#v", deal)
	}
	pipeline := runJSON("pipeline")
	if pipeline["currency"] != "USD" {
		t.Fatalf("pipeline output = %#v", pipeline)
	}
}

func TestCLICacheRequiresHostedJaybase(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{
		"--store", filepath.Join(t.TempDir(), "store"),
		"--cache-dir", filepath.Join(t.TempDir(), "cache"),
		"init",
	}, &out)
	if err == nil {
		t.Fatal("expected local cache configuration to be rejected")
	}
}
