package martin

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kyle-visner/jaybase"
)

func TestBDDAerieCommitsRemainForeignWhileAdvancingMartinRoot(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := Context{Actor: "owner"}
	if _, _, err := store.Initialize(ctx, "USD"); err != nil {
		t.Fatal(err)
	}
	before, err := store.CurrentRoot()
	if err != nil {
		t.Fatal(err)
	}
	aerieRoot, err := store.db.AppendAt(jaybase.Context{Actor: "aerie"}, jaybase.AppendOptions{
		Type: "aerie.commit.created.v1", EntityID: "repository:default", Command: "commit",
		Payload: map[string]any{"message": "Document onboarding"},
	}, before)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Root != aerieRoot || state.Workspace == nil {
		t.Fatalf("Martin did not advance across Aerie commit: %#v", state)
	}
	organization, root, err := store.CreateOrganization(ctx, Organization{Name: "After Aerie"})
	if err != nil {
		t.Fatal(err)
	}
	if organization.ID == "" || root == aerieRoot {
		t.Fatalf("Martin did not append after Aerie root: organization=%#v root=%s", organization, root)
	}
}

func TestOpinionatedDealWorkflow(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC)
	store.now = func() time.Time {
		current := now
		now = now.Add(time.Second)
		return current
	}
	ctx := Context{Actor: "owner"}
	if _, _, err := store.Initialize(ctx, "USD"); err != nil {
		t.Fatal(err)
	}
	organization, _, err := store.CreateOrganization(ctx, Organization{Name: "Acme", Domain: "https://www.Acme.test/"})
	if err != nil {
		t.Fatal(err)
	}
	if organization.Domain != "acme.test" {
		t.Fatalf("normalized domain = %q", organization.Domain)
	}
	person, _, err := store.CreatePerson(ctx, Person{
		DisplayName: "Ada Lovelace", Email: "ADA@EXAMPLE.COM", OrganizationID: organization.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	deal, firstTask, _, err := store.CreateDeal(ctx, Deal{
		Name: "Implementation", OrganizationID: organization.ID, PersonID: person.ID,
		ValueCents: 125000, ExpectedClose: "2026-08-31",
	}, "Run discovery", "2026-07-24")
	if err != nil {
		t.Fatal(err)
	}
	if deal.Stage != DealNew || deal.NextTaskID != firstTask.ID || firstTask.Status != TaskPending {
		t.Fatalf("unexpected new deal: %#v task=%#v", deal, firstTask)
	}

	deal, secondTask, _, err := store.AdvanceDeal(ctx, deal.ID, "Prepare proposal", "2026-07-25")
	if err != nil {
		t.Fatal(err)
	}
	if deal.Stage != DealQualified || deal.NextTaskID != secondTask.ID {
		t.Fatalf("unexpected advanced deal: %#v", deal)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Tasks[firstTask.ID].Status != TaskCompleted {
		t.Fatalf("first task was not completed: %#v", state.Tasks[firstTask.ID])
	}

	deal, activity, thirdTask, _, err := store.TouchDeal(
		ctx, deal.ID, ActivityMeeting, "Discovery completed", time.Time{}, "Send proposal", "2026-07-26",
	)
	if err != nil {
		t.Fatal(err)
	}
	if activity.DealID != deal.ID || deal.NextTaskID != thirdTask.ID {
		t.Fatalf("touch was not atomic: deal=%#v activity=%#v task=%#v", deal, activity, thirdTask)
	}
	deal, fourthTask, _, err := store.AdvanceDeal(ctx, deal.ID, "Get signature", "2026-07-30")
	if err != nil {
		t.Fatal(err)
	}
	if deal.Stage != DealProposal || fourthTask.DealID != deal.ID {
		t.Fatalf("unexpected proposal stage: %#v %#v", deal, fourthTask)
	}
	if _, _, _, err := store.AdvanceDeal(ctx, deal.ID, "Invalid", "2026-07-31"); err == nil {
		t.Fatal("proposal deal advanced instead of requiring win or loss")
	}
	deal, _, err = store.WinDeal(ctx, deal.ID, "2026-07-30")
	if err != nil {
		t.Fatal(err)
	}
	if deal.Stage != DealWon || deal.NextTaskID != "" {
		t.Fatalf("won deal retained an open next action: %#v", deal)
	}
	state, _ = store.LoadState()
	if state.Tasks[fourthTask.ID].Status != TaskCanceled {
		t.Fatalf("winning did not close the outstanding task: %#v", state.Tasks[fourthTask.ID])
	}
	if _, _, err := store.ArchiveOrganization(ctx, organization.ID, "engagement complete"); err != nil {
		t.Fatal(err)
	}
}

func TestTasksReportsAndSearch(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.now = func() time.Time { return time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC) }
	ctx := Context{Actor: "owner"}
	_, _, _ = store.Initialize(ctx, "USD")
	org, _, _ := store.CreateOrganization(ctx, Organization{Name: "Future Perfect", Domain: "futureperfect.test"})
	task, _, err := store.CreateTask(ctx, Task{Title: "Follow up", DueDate: "2026-07-22", OrganizationID: org.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.CreateTask(ctx, Task{Title: "Call today", DueDate: "2026-07-23", OrganizationID: org.ID})
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.Today(ctx, "", "2026-07-23")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Overdue) != 1 || len(report.Due) != 1 {
		t.Fatalf("today report = %#v", report)
	}
	if _, _, err := store.CompleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	results, err := store.Search(ctx, "future")
	if err != nil || len(results) != 1 || results[0].ID != org.ID {
		t.Fatalf("search results=%#v err=%v", results, err)
	}
}

func TestMagpieBridgeAndRoleReuse(t *testing.T) {
	dir := t.TempDir()
	raw, err := jaybase.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := appendRaw(t, raw, "", "store.init", "", "store init", magpieEvent("init", map[string]any{
		"roles": map[string]any{
			"Owner":      map[string]any{"name": "Owner", "permissions": []string{"ledger:read"}},
			"Sales Rep":  map[string]any{"name": "Sales Rep", "permissions": []string{"notes:read"}},
			"Accountant": map[string]any{"name": "Accountant", "permissions": []string{"ledger:read"}},
		},
		"users": map[string]any{
			"owner": map[string]string{"id": "owner", "role": "Owner"},
			"rep":   map[string]string{"id": "rep", "role": "Sales Rep"},
			"acct":  map[string]string{"id": "acct", "role": "Accountant"},
		},
		"settings": map[string]string{"accounting_basis": "cash"},
	}))
	root = appendRaw(t, raw, root, "customer", "cust:acme", "customer upsert", magpieEvent("customer.upsert", map[string]any{
		"customer": map[string]any{"id": "cust:acme", "name": "Acme Books"},
	}))
	_ = appendRaw(t, raw, root, "invoice", "inv:1", "invoice create", magpieEvent("invoice.create", map[string]any{
		"invoice": map[string]any{
			"id": "inv:1", "invoice_number": "1001", "customer_id": "cust:acme",
			"invoice_date": "2026-07-20", "status": "open", "total_cents": 50000,
		},
	}))
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner := Context{Actor: "owner"}
	if _, _, err := store.Initialize(owner, "USD"); err != nil {
		t.Fatal(err)
	}
	org, _, err := store.CreateOrganization(Context{Actor: "rep"}, Organization{Name: "Acme CRM"})
	if err != nil {
		t.Fatalf("Sales Rep could not write CRM facts: %v", err)
	}
	if _, _, err := store.CreateOrganization(Context{Actor: "acct"}, Organization{Name: "Forbidden"}); err == nil {
		t.Fatal("Accountant unexpectedly wrote CRM facts")
	}
	link, _, err := store.LinkCustomer(owner, org.ID, "", "cust:acme")
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.Customer(Context{Actor: "acct"}, "cust:acme")
	if err != nil {
		t.Fatal(err)
	}
	if view.Link.ID != link.ID || view.Customer.Name != "Acme Books" || len(view.Invoices) != 1 {
		t.Fatalf("combined customer view = %#v", view)
	}
}

func TestUnknownMartinEventFailsClosedButOtherAppsAreIgnored(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{Actor: "owner"}
	_, root, err := store.Initialize(ctx, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := jaybase.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	root = appendRaw(t, raw, root, "other.application.fact", "other:1", "other write", map[string]bool{"ok": true})
	_ = appendRaw(t, raw, root, "martin.unknown.v1", "unknown:1", "bad write", map[string]bool{"ok": false})
	_ = raw.Close()

	store, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.LoadState()
	var appError *AppError
	if !errors.As(err, &appError) || appError.Code != ErrIntegrity {
		t.Fatalf("unknown Martin event error = %T %v", err, err)
	}
}

func TestIdempotentImport(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := Context{Actor: "owner"}
	_, _, _ = store.Initialize(ctx, "USD")
	bundle := ImportBundle{
		Source: "legacy-crm", SourceKey: "batch-1",
		Organizations: []Organization{{ID: "org:legacy", Name: "Legacy Co"}},
	}
	first, firstRoot, err := store.Import(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	second, secondRoot, err := store.Import(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if firstRoot != secondRoot || first.Receipt.Digest != second.Receipt.Digest {
		t.Fatalf("import replay changed state: first=%#v second=%#v", first, second)
	}
	bundle.Organizations[0].Name = "Changed"
	if _, _, err := store.Import(ctx, bundle); err == nil {
		t.Fatal("changed import reused an existing source key")
	}
}

func appendRaw(t *testing.T, store *jaybase.Store, expectedRoot, typ, entityID, command string, payload any) string {
	t.Helper()
	root, err := store.AppendAt(jaybase.Context{Actor: "fixture", Role: "writer"}, jaybase.AppendOptions{
		Type: typ, EntityID: entityID, Command: command, Payload: payload, CreatedAt: time.Now().UTC(),
	}, expectedRoot)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func magpieEvent(kind string, data any) map[string]any {
	raw, _ := json.Marshal(data)
	return map[string]any{"kind": kind, "data": json.RawMessage(raw)}
}
