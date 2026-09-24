package martin

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kyle-visner/jaybase"
)

// Patterns copied from Jaybase server/allow.go at a4529fa (jaybase#20).
// Martin stays on its current Jaybase pin and does not import the server package.
var (
	jaybaseExactTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}\.[a-z0-9][a-z0-9._-]{0,63}$`)
	jaybaseCommandPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)
)

func TestMartinFactsMatchJaybaseCatalogGrammar(t *testing.T) {
	files := parseMartinSources(t)
	constants := stringConstants(files)
	commands := emittedCommands(t, files, constants)
	if len(commands) == 0 {
		t.Fatal("found no Martin commands")
	}
	seen := map[string]struct{}{}
	for _, command := range commands {
		if !jaybaseCommandPattern.MatchString(command) {
			t.Errorf("command %q is not an exact Jaybase command", command)
		}
		seen[command] = struct{}{}
	}
	if _, ok := seen[commandDealReopen]; !ok {
		t.Fatalf("emitted commands %v do not include %q", commands, commandDealReopen)
	}
	for command := range seen {
		if strings.HasPrefix(command, "deal reopen:") {
			t.Errorf("Martin still emits legacy command %q", command)
		}
	}
	if jaybaseCommandPattern.MatchString("deal reopen: customer returned") {
		t.Fatal("legacy reopen command unexpectedly matches the Jaybase command charset")
	}

	types := martinEventTypes(t, files)
	if len(types) == 0 {
		t.Fatal("found no Martin event types")
	}
	namespaces := map[string]struct{}{}
	for _, eventType := range types {
		if !jaybaseExactTypePattern.MatchString(eventType) {
			t.Errorf("type %q is not namespace.name", eventType)
		}
		namespace, name, ok := strings.Cut(eventType, ".")
		if !ok || namespace == "" || name == "" {
			t.Errorf("type %q has no namespace", eventType)
			continue
		}
		namespaces[namespace] = struct{}{}
		if !jaybaseTypeAllowed("martin.*", eventType) {
			t.Errorf("type %q is outside martin.*", eventType)
		}
	}
	if len(namespaces) != 1 {
		t.Fatalf("Martin types use namespaces %v; allow.types needs a single namespace", namespaces)
	}
	if _, ok := namespaces["martin"]; !ok {
		t.Fatalf("namespace = %v", namespaces)
	}
}

func TestSnapshotRefsMatchMartinAllowPattern(t *testing.T) {
	names := []string{"q", "quarterly", "Q3.final", "backup_2026", "a-b"}
	for _, name := range names {
		if !snapshotNamePattern.MatchString(name) {
			t.Fatalf("fixture %q is not a snapshot name", name)
		}
		ref := "martin-" + name
		if !jaybaseRefAllowed("martin-*", ref) {
			t.Fatalf("ref %q does not match martin-*", ref)
		}
	}
	if jaybaseRefAllowed("martin-*", "magpie-quarterly") {
		t.Fatal("martin-* matched a non-Martin ref")
	}
}

func TestReopenDealUsesExactCommandAndPayloadReason(t *testing.T) {
	store := openReopenFixture(t)
	defer store.Close()
	ctx := Context{Actor: "owner"}
	deal := mustLostDeal(t, store, ctx)

	reopened, task, root, err := store.ReopenDeal(ctx, deal.ID, "  customer returned  ", "Restart discovery", "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	if !isOpenStage(reopened.Stage) || reopened.NextTaskID != task.ID || task.Title != "Restart discovery" {
		t.Fatalf("reopened deal = %#v task = %#v", reopened, task)
	}

	nodes, err := store.NodesFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	last := nodes[len(nodes)-1]
	if last.Type != TypeDealReopened || last.Command != commandDealReopen {
		t.Fatalf("reopen fact type=%s command=%q", last.Type, last.Command)
	}
	raw, err := store.db.NodePayload(last)
	if err != nil {
		t.Fatal(err)
	}
	var payload dealWorkflowPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Reason != "customer returned" {
		t.Fatalf("payload reason = %q", payload.Reason)
	}
	if strings.Contains(last.Command, "customer returned") || strings.Contains(last.Command, ":") {
		t.Fatalf("reason leaked into command %q", last.Command)
	}
}

func TestHistoricalDealReopenCommandStillProjects(t *testing.T) {
	dir := t.TempDir()
	store := openReopenFixtureAt(t, dir)
	ctx := Context{Actor: "owner"}
	deal := mustLostDeal(t, store, ctx)
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	closed := state.Deals[deal.ID]
	reopened := closed
	reopened.PreviousStage = closed.Stage
	reopened.Stage = DealQualified
	reopened.ClosedOn = ""
	reopened.LostReason = ""
	reopened.NextTaskID = "task:historical"
	task := Task{
		ID: "task:historical", Title: "Restart", DueDate: "2026-09-02", OwnerID: closed.OwnerID,
		Status: TaskPending, OrganizationID: closed.OrganizationID, DealID: closed.ID,
	}
	root := state.Root
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	rawStore, err := jaybase.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	appendRaw(t, rawStore, root, TypeDealReopened, closed.ID, "deal reopen: customer returned", dealWorkflowPayload{
		Deal: reopened, NextTask: &task,
	})
	if err := rawStore.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state, err = store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	got := state.Deals[closed.ID]
	if got.Stage != DealQualified || got.NextTaskID != task.ID || got.ClosedOn != "" {
		t.Fatalf("historical reopen did not project: %#v", got)
	}
	if state.Tasks[task.ID].Status != TaskPending || state.Tasks[task.ID].Title != "Restart" {
		t.Fatalf("historical next action = %#v", state.Tasks[task.ID])
	}

	nodes, err := store.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, node := range nodes {
		if node.Type != TypeDealReopened {
			continue
		}
		found = true
		if node.Command != "deal reopen: customer returned" {
			t.Fatalf("historical command = %q", node.Command)
		}
	}
	if !found {
		t.Fatal("historical reopen fact missing from audit")
	}
}

func openReopenFixture(t *testing.T) *Store {
	t.Helper()
	return openReopenFixtureAt(t, t.TempDir())
}

func openReopenFixtureAt(t *testing.T, dir string) *Store {
	t.Helper()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Initialize(Context{Actor: "owner"}, "USD"); err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store
}

func mustLostDeal(t *testing.T, store *Store, ctx Context) Deal {
	t.Helper()
	organization, _, err := store.CreateOrganization(ctx, Organization{Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	deal, _, _, err := store.CreateDeal(ctx, Deal{
		Name: "Renewal", OrganizationID: organization.ID, ValueCents: 100, ExpectedClose: "2026-09-01",
	}, "Send quote", "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	deal, _, err = store.LoseDeal(ctx, deal.ID, "2026-08-15", "timing")
	if err != nil {
		t.Fatal(err)
	}
	return deal
}

func parseMartinSources(t *testing.T) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatal("no Martin sources found")
	}
	return files
}

func stringConstants(files []*ast.File) map[string]string {
	constants := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for i, name := range valueSpec.Names {
					if i >= len(valueSpec.Values) {
						continue
					}
					lit, ok := valueSpec.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					value, err := strconv.Unquote(lit.Value)
					if err != nil {
						continue
					}
					constants[name.Name] = value
				}
			}
		}
	}
	return constants
}

func martinEventTypes(t *testing.T, files []*ast.File) []string {
	t.Helper()
	constants := stringConstants(files)
	var types []string
	for name, value := range constants {
		if strings.HasPrefix(name, "Type") {
			types = append(types, value)
		}
	}
	return types
}

func emittedCommands(t *testing.T, files []*ast.File, constants map[string]string) []string {
	t.Helper()
	var commands []string
	dynamic := false
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "appendAt" || len(call.Args) < 4 {
				return true
			}
			if command, ok := exactCommand(call.Args[3], constants); ok {
				commands = append(commands, command)
				return true
			}
			ident, ok := call.Args[3].(*ast.Ident)
			if !ok || ident.Name != "command" {
				t.Fatalf("appendAt command must be an exact string, got %T", call.Args[3])
			}
			dynamic = true
			return true
		})
	}
	if dynamic {
		commands = append(commands, commandAssignments(t, files)...)
	}
	return commands
}

func commandAssignments(t *testing.T, files []*ast.File) []string {
	t.Helper()
	var commands []string
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != len(assignment.Rhs) {
				return true
			}
			for i, lhs := range assignment.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name != "command" {
					continue
				}
				command, ok := exactCommand(assignment.Rhs[i], nil)
				if !ok {
					t.Fatalf("command assignment is not an exact string (%T)", assignment.Rhs[i])
				}
				commands = append(commands, command)
			}
			return true
		})
	}
	if len(commands) == 0 {
		t.Fatal("appendAt uses command but no exact assignments were found")
	}
	return commands
}

func exactCommand(expr ast.Expr, constants map[string]string) (string, bool) {
	switch arg := expr.(type) {
	case *ast.BasicLit:
		if arg.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(arg.Value)
		if err != nil {
			return "", false
		}
		return value, true
	case *ast.Ident:
		if constants == nil {
			return "", false
		}
		value, ok := constants[arg.Name]
		return value, ok
	default:
		return "", false
	}
}

func jaybaseTypeAllowed(pattern, eventType string) bool {
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*") + "."
		return strings.HasPrefix(eventType, prefix) && len(eventType) > len(prefix)
	}
	return pattern == eventType
}

func jaybaseRefAllowed(pattern, name string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix) && len(name) > len(prefix)
	}
	return pattern == name
}
