package martin_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kyle-visner/martin"
)

func TestHostCanInvokeCRMTools(t *testing.T) {
	store, err := martin.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := martin.Context{Actor: martin.ActorOwner}
	if _, _, err := store.Initialize(ctx, "USD"); err != nil {
		t.Fatal(err)
	}
	if !martin.KnownTool("organization_create") || martin.KnownTool("init") {
		t.Fatal("KnownTool does not match the catalog")
	}
	crm := martin.NewCRM(store, ctx)
	out, err := crm.Invoke("organization_create", json.RawMessage(`{"name":"Acme"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := martin.EncodeJSON(out); err != nil {
		t.Fatal(err)
	}
	_, err = crm.Invoke("organization_get", json.RawMessage(`{"id":"org_missing"}`))
	var appErr *martin.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("want an AppError, got %v", err)
	}
}
