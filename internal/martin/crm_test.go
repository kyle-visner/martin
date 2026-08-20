package martin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolCatalogOmitsInitAndCoversCRM(t *testing.T) {
	names := map[string]bool{}
	for _, item := range ToolCatalog() {
		names[item.Name] = true
	}
	if names["init"] || KnownTool("init") {
		t.Fatal("init must remain the CLI command; MCP must not re-implement it")
	}
	for _, required := range []string{
		"doctor", "today", "pipeline", "search", "state", "audit",
		"organization_create", "person_create", "deal_create", "deal_touch",
		"activity_log", "task_create", "customer_link",
	} {
		if !names[required] {
			t.Fatalf("missing tool %s", required)
		}
	}
}

func TestInvokeUsesExistingStoreAndWrapsLists(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := Context{Actor: "owner"}
	if _, _, err := store.Initialize(ctx, "USD"); err != nil {
		t.Fatal(err)
	}
	crm := NewCRM(store, ctx)

	created, err := crm.Invoke("organization_create", json.RawMessage(`{"name":"Acme","domain":"acme.test"}`))
	if err != nil {
		t.Fatal(err)
	}
	orgID := asMap(t, created)["organization"].(Organization).ID
	if orgID == "" {
		t.Fatalf("expected organization id, got %#v", created)
	}

	listed, err := crm.Invoke("organization_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	list := asMap(t, listed)
	if _, ok := list["organizations"]; !ok {
		t.Fatalf("list result must be an object with organizations, got %#v", listed)
	}

	deal, err := crm.Invoke("deal_create", json.RawMessage(`{
		"name":"Website","organization_id":"`+orgID+`","value_cents":2500,
		"expected_close":"2026-08-31","next_action":"Call","next_due":"2026-07-24"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	body := asMap(t, deal)
	if body["deal"].(Deal).Stage != DealNew {
		t.Fatalf("deal = %#v", deal)
	}
	if body["next_action"].(Task).Title != "Call" {
		t.Fatalf("next action = %#v", deal)
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
	// Keep typed fields for callers that type-assert the original map.
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return out
}

func TestInvokeRejectsUnknownFieldsIncludingActor(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err := store.Initialize(Context{Actor: "owner"}, "USD"); err != nil {
		t.Fatal(err)
	}
	crm := NewCRM(store, Context{Actor: "owner"})
	_, err = crm.Invoke("organization_create", json.RawMessage(`{"name":"Acme","actor":"other"}`))
	if err == nil {
		t.Fatal("clients must not be able to supply actor")
	}
	if !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("got %v", err)
	}
}
