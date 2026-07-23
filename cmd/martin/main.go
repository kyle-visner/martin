package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kyle-visner/martin/internal/martin"
)

const version = "0.1.0-dev"

type app struct {
	store *martin.Store
	ctx   martin.Context
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		writeError(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	global := flag.NewFlagSet("martin", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	storeDir := global.String("store", ".martin", "local Jaybase store directory")
	jaybaseURL := global.String("jaybase-url", os.Getenv("JAYBASE_URL"), "hosted Jaybase HTTPS origin; defaults to JAYBASE_URL")
	cacheDir := global.String("cache-dir", os.Getenv("MARTIN_CACHE_DIR"), "private directory for an encrypted hosted-state checkpoint; defaults to MARTIN_CACHE_DIR")
	actor := global.String("actor", "owner", "authenticated actor id")
	role := global.String("role", "", "role to assume; defaults to the actor's Magpie role")
	if err := global.Parse(args); err != nil {
		return err
	}
	rest := global.Args()
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		return usage(out)
	}
	if rest[0] == "version" {
		return writeJSON(out, map[string]string{"version": version})
	}
	storeExplicit := false
	global.Visit(func(f *flag.Flag) {
		if f.Name == "store" {
			storeExplicit = true
		}
	})
	if strings.TrimSpace(*jaybaseURL) != "" && storeExplicit {
		return fmt.Errorf("--store and --jaybase-url/JAYBASE_URL are mutually exclusive")
	}
	var store *martin.Store
	var err error
	if strings.TrimSpace(*jaybaseURL) != "" {
		store, err = martin.OpenRemoteStoreWithOptions(*jaybaseURL, os.Getenv("JAYBASE_TOKEN"), martin.RemoteStoreOptions{
			CacheDir: *cacheDir,
		})
	} else {
		if strings.TrimSpace(*cacheDir) != "" {
			return fmt.Errorf("--cache-dir/MARTIN_CACHE_DIR is only valid with hosted Jaybase")
		}
		store, err = martin.OpenStore(*storeDir)
	}
	if err != nil {
		return err
	}
	defer store.Close()
	a := app{store: store, ctx: martin.Context{Actor: *actor, Role: *role}}

	switch rest[0] {
	case "init":
		fs := newFlagSet("init")
		currency := fs.String("currency", "USD", "immutable workspace ISO currency")
		if err := fs.Parse(rest[1:]); err != nil {
			return err
		}
		workspace, root, err := store.Initialize(a.ctx, *currency)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"workspace": workspace, "root": root, "store": store.Dir()})
	case "state", "export":
		state, err := store.Export(a.ctx)
		if err != nil {
			return err
		}
		return writeJSON(out, state)
	case "doctor":
		state, err := store.Export(a.ctx)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{
			"ok": true, "root": state.Root, "store": store.Dir(), "workspace": state.Workspace,
			"counts": map[string]int{
				"organizations": len(state.Organizations), "people": len(state.People), "deals": len(state.Deals),
				"activities": len(state.Activities), "tasks": len(state.Tasks), "customer_links": len(state.CustomerLinks),
			},
		})
	case "audit":
		nodes, err := store.Audit(a.ctx)
		if err != nil {
			return err
		}
		return writeJSON(out, nodes)
	case "organization":
		return a.organization(rest[1:], out)
	case "person":
		return a.person(rest[1:], out)
	case "deal":
		return a.deal(rest[1:], out)
	case "activity":
		return a.activity(rest[1:], out)
	case "task":
		return a.task(rest[1:], out)
	case "customer":
		return a.customer(rest[1:], out)
	case "pipeline":
		return a.pipeline(rest[1:], out)
	case "today":
		return a.today(rest[1:], out)
	case "search":
		return a.search(rest[1:], out)
	case "import-json":
		return a.importJSON(rest[1:], out)
	case "snapshot":
		return a.snapshot(rest[1:], out)
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func (a app) organization(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("organization command required")
	}
	switch args[0] {
	case "create":
		fs := newFlagSet("organization create")
		id := fs.String("id", "", "optional stable id")
		name := fs.String("name", "", "organization name")
		domain := fs.String("domain", "", "primary domain")
		email := fs.String("email", "", "primary email")
		phone := fs.String("phone", "", "primary phone")
		owner := fs.String("owner", "", "owner actor id")
		tags := fs.String("tags", "", "comma-separated tags")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entity, root, err := a.store.CreateOrganization(a.ctx, martin.Organization{
			ID: *id, Name: *name, Domain: *domain, Email: *email, Phone: *phone, OwnerID: *owner, Tags: splitCSV(*tags),
		})
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"organization": entity, "root": root})
	case "update":
		fs := newFlagSet("organization update")
		id := fs.String("id", "", "organization id")
		name := fs.String("name", "", "replacement name")
		domain := fs.String("domain", "", "replacement domain")
		email := fs.String("email", "", "replacement email")
		phone := fs.String("phone", "", "replacement phone")
		owner := fs.String("owner", "", "replacement owner")
		tags := fs.String("tags", "", "replacement comma-separated tags")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entity, err := a.store.GetOrganization(a.ctx, *id)
		if err != nil {
			return err
		}
		visited := visitedFlags(fs)
		if visited["name"] {
			entity.Name = *name
		}
		if visited["domain"] {
			entity.Domain = *domain
		}
		if visited["email"] {
			entity.Email = *email
		}
		if visited["phone"] {
			entity.Phone = *phone
		}
		if visited["owner"] {
			entity.OwnerID = *owner
		}
		if visited["tags"] {
			entity.Tags = splitCSV(*tags)
		}
		entity, root, err := a.store.UpdateOrganization(a.ctx, entity)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"organization": entity, "root": root})
	case "get":
		fs := newFlagSet("organization get")
		id := fs.String("id", "", "organization id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entity, err := a.store.GetOrganization(a.ctx, *id)
		if err != nil {
			return err
		}
		return writeJSON(out, entity)
	case "list":
		fs := newFlagSet("organization list")
		includeArchived := fs.Bool("include-archived", false, "include archived organizations")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entities, err := a.store.ListOrganizations(a.ctx, *includeArchived)
		if err != nil {
			return err
		}
		return writeJSON(out, entities)
	case "archive":
		fs := newFlagSet("organization archive")
		id := fs.String("id", "", "organization id")
		reason := fs.String("reason", "", "required audit reason")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entity, root, err := a.store.ArchiveOrganization(a.ctx, *id, *reason)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"organization": entity, "root": root})
	case "merge":
		fs := newFlagSet("organization merge")
		from := fs.String("from", "", "duplicate organization id")
		into := fs.String("into", "", "surviving organization id")
		reason := fs.String("reason", "", "required audit reason")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		root, err := a.store.MergeOrganizations(a.ctx, *from, *into, *reason)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"from": *from, "into": *into, "root": root})
	default:
		return fmt.Errorf("unknown organization command %q", args[0])
	}
}

func (a app) person(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("person command required")
	}
	switch args[0] {
	case "create":
		fs := newFlagSet("person create")
		id := fs.String("id", "", "optional stable id")
		name := fs.String("display-name", "", "person display name")
		organizationID := fs.String("organization-id", "", "organization id")
		title := fs.String("title", "", "job title")
		email := fs.String("email", "", "email")
		phone := fs.String("phone", "", "phone")
		owner := fs.String("owner", "", "owner actor id")
		tags := fs.String("tags", "", "comma-separated tags")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entity, root, err := a.store.CreatePerson(a.ctx, martin.Person{
			ID: *id, DisplayName: *name, OrganizationID: *organizationID, Title: *title,
			Email: *email, Phone: *phone, OwnerID: *owner, Tags: splitCSV(*tags),
		})
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"person": entity, "root": root})
	case "update":
		fs := newFlagSet("person update")
		id := fs.String("id", "", "person id")
		name := fs.String("display-name", "", "replacement display name")
		organizationID := fs.String("organization-id", "", "replacement organization; empty clears")
		title := fs.String("title", "", "replacement title")
		email := fs.String("email", "", "replacement email")
		phone := fs.String("phone", "", "replacement phone")
		owner := fs.String("owner", "", "replacement owner")
		tags := fs.String("tags", "", "replacement comma-separated tags")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entity, err := a.store.GetPerson(a.ctx, *id)
		if err != nil {
			return err
		}
		visited := visitedFlags(fs)
		if visited["display-name"] {
			entity.DisplayName = *name
		}
		if visited["organization-id"] {
			entity.OrganizationID = *organizationID
		}
		if visited["title"] {
			entity.Title = *title
		}
		if visited["email"] {
			entity.Email = *email
		}
		if visited["phone"] {
			entity.Phone = *phone
		}
		if visited["owner"] {
			entity.OwnerID = *owner
		}
		if visited["tags"] {
			entity.Tags = splitCSV(*tags)
		}
		entity, root, err := a.store.UpdatePerson(a.ctx, entity)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"person": entity, "root": root})
	case "get":
		fs := newFlagSet("person get")
		id := fs.String("id", "", "person id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entity, err := a.store.GetPerson(a.ctx, *id)
		if err != nil {
			return err
		}
		return writeJSON(out, entity)
	case "list":
		fs := newFlagSet("person list")
		includeArchived := fs.Bool("include-archived", false, "include archived people")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entities, err := a.store.ListPeople(a.ctx, *includeArchived)
		if err != nil {
			return err
		}
		return writeJSON(out, entities)
	case "archive":
		fs := newFlagSet("person archive")
		id := fs.String("id", "", "person id")
		reason := fs.String("reason", "", "required audit reason")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		entity, root, err := a.store.ArchivePerson(a.ctx, *id, *reason)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"person": entity, "root": root})
	case "merge":
		fs := newFlagSet("person merge")
		from := fs.String("from", "", "duplicate person id")
		into := fs.String("into", "", "surviving person id")
		reason := fs.String("reason", "", "required audit reason")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		root, err := a.store.MergePeople(a.ctx, *from, *into, *reason)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"from": *from, "into": *into, "root": root})
	default:
		return fmt.Errorf("unknown person command %q", args[0])
	}
}

func (a app) deal(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("deal command required")
	}
	switch args[0] {
	case "create":
		fs := newFlagSet("deal create")
		id := fs.String("id", "", "optional stable id")
		name := fs.String("name", "", "deal name")
		organizationID := fs.String("organization-id", "", "organization id")
		personID := fs.String("person-id", "", "primary person id")
		owner := fs.String("owner", "", "owner actor id")
		valueCents := fs.Int64("value-cents", 0, "deal value in minor units")
		expectedClose := fs.String("expected-close", "", "expected close date YYYY-MM-DD")
		nextAction := fs.String("next-action", "", "required next action")
		nextDue := fs.String("next-due", "", "next action due date YYYY-MM-DD")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		deal, task, root, err := a.store.CreateDeal(a.ctx, martin.Deal{
			ID: *id, Name: *name, OrganizationID: *organizationID, PersonID: *personID,
			OwnerID: *owner, ValueCents: *valueCents, ExpectedClose: *expectedClose,
		}, *nextAction, *nextDue)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"deal": deal, "next_action": task, "root": root})
	case "advance":
		fs := newFlagSet("deal advance")
		id := fs.String("id", "", "deal id")
		nextAction := fs.String("next-action", "", "replacement next action")
		nextDue := fs.String("next-due", "", "replacement next action due date")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		deal, task, root, err := a.store.AdvanceDeal(a.ctx, *id, *nextAction, *nextDue)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"deal": deal, "next_action": task, "root": root})
	case "touch":
		fs := newFlagSet("deal touch")
		id := fs.String("id", "", "deal id")
		kind := fs.String("kind", "", "call, email, meeting, or note")
		summary := fs.String("summary", "", "interaction summary")
		occurredAt := fs.String("occurred-at", "", "RFC3339 timestamp; defaults to now")
		nextAction := fs.String("next-action", "", "replacement next action")
		nextDue := fs.String("next-due", "", "replacement next action due date")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		at, err := parseRFC3339(*occurredAt)
		if err != nil {
			return err
		}
		deal, activity, task, root, err := a.store.TouchDeal(a.ctx, *id, martin.ActivityKind(*kind), *summary, at, *nextAction, *nextDue)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"deal": deal, "activity": activity, "next_action": task, "root": root})
	case "win":
		fs := newFlagSet("deal win")
		id := fs.String("id", "", "deal id")
		closedOn := fs.String("closed-on", "", "close date YYYY-MM-DD")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		deal, root, err := a.store.WinDeal(a.ctx, *id, *closedOn)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"deal": deal, "root": root})
	case "lose":
		fs := newFlagSet("deal lose")
		id := fs.String("id", "", "deal id")
		closedOn := fs.String("closed-on", "", "close date YYYY-MM-DD")
		reason := fs.String("reason", "", "required loss reason")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		deal, root, err := a.store.LoseDeal(a.ctx, *id, *closedOn, *reason)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"deal": deal, "root": root})
	case "reopen":
		fs := newFlagSet("deal reopen")
		id := fs.String("id", "", "deal id")
		reason := fs.String("reason", "", "required audit reason")
		nextAction := fs.String("next-action", "", "new next action")
		nextDue := fs.String("next-due", "", "new next action due date")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		deal, task, root, err := a.store.ReopenDeal(a.ctx, *id, *reason, *nextAction, *nextDue)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"deal": deal, "next_action": task, "root": root})
	case "get":
		fs := newFlagSet("deal get")
		id := fs.String("id", "", "deal id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		deal, err := a.store.GetDeal(a.ctx, *id)
		if err != nil {
			return err
		}
		return writeJSON(out, deal)
	case "list":
		fs := newFlagSet("deal list")
		includeClosed := fs.Bool("include-closed", false, "include won and lost deals")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		deals, err := a.store.ListDeals(a.ctx, *includeClosed)
		if err != nil {
			return err
		}
		return writeJSON(out, deals)
	default:
		return fmt.Errorf("unknown deal command %q", args[0])
	}
}

func (a app) activity(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("activity command required")
	}
	switch args[0] {
	case "log":
		fs := newFlagSet("activity log")
		kind := fs.String("kind", "", "call, email, meeting, or note")
		summary := fs.String("summary", "", "activity summary")
		occurredAt := fs.String("occurred-at", "", "RFC3339 timestamp; defaults to now")
		organizationID := fs.String("organization-id", "", "related organization")
		personID := fs.String("person-id", "", "related person")
		dealID := fs.String("deal-id", "", "related deal")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		at, err := parseRFC3339(*occurredAt)
		if err != nil {
			return err
		}
		activity, root, err := a.store.LogActivity(a.ctx, martin.Activity{
			Kind: martin.ActivityKind(*kind), Summary: *summary, OccurredAt: at,
			OrganizationID: *organizationID, PersonID: *personID, DealID: *dealID,
		})
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"activity": activity, "root": root})
	case "list":
		fs := newFlagSet("activity list")
		organizationID := fs.String("organization-id", "", "filter by organization")
		personID := fs.String("person-id", "", "filter by person")
		dealID := fs.String("deal-id", "", "filter by deal")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		activities, err := a.store.ListActivities(a.ctx, *organizationID, *personID, *dealID)
		if err != nil {
			return err
		}
		return writeJSON(out, activities)
	default:
		return fmt.Errorf("unknown activity command %q", args[0])
	}
}

func (a app) task(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("task command required")
	}
	switch args[0] {
	case "create":
		fs := newFlagSet("task create")
		title := fs.String("title", "", "task title")
		due := fs.String("due", "", "due date YYYY-MM-DD")
		owner := fs.String("owner", "", "owner actor id")
		organizationID := fs.String("organization-id", "", "related organization")
		personID := fs.String("person-id", "", "related person")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		task, root, err := a.store.CreateTask(a.ctx, martin.Task{
			Title: *title, DueDate: *due, OwnerID: *owner,
			OrganizationID: *organizationID, PersonID: *personID,
		})
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"task": task, "root": root})
	case "complete":
		fs := newFlagSet("task complete")
		id := fs.String("id", "", "task id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		task, root, err := a.store.CompleteTask(a.ctx, *id)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"task": task, "root": root})
	case "cancel":
		fs := newFlagSet("task cancel")
		id := fs.String("id", "", "task id")
		reason := fs.String("reason", "", "required audit reason")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		task, root, err := a.store.CancelTask(a.ctx, *id, *reason)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"task": task, "root": root})
	case "list":
		fs := newFlagSet("task list")
		owner := fs.String("owner", "", "filter by owner")
		status := fs.String("status", "", "pending, completed, or canceled")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		tasks, err := a.store.ListTasks(a.ctx, *owner, martin.TaskStatus(*status))
		if err != nil {
			return err
		}
		return writeJSON(out, tasks)
	default:
		return fmt.Errorf("unknown task command %q", args[0])
	}
}

func (a app) customer(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("customer command required")
	}
	switch args[0] {
	case "link":
		fs := newFlagSet("customer link")
		organizationID := fs.String("organization-id", "", "Martin organization id")
		personID := fs.String("person-id", "", "Martin person id")
		customerID := fs.String("magpie-customer-id", "", "Magpie customer id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		link, root, err := a.store.LinkCustomer(a.ctx, *organizationID, *personID, *customerID)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"customer_link": link, "root": root})
	case "unlink":
		fs := newFlagSet("customer unlink")
		customerID := fs.String("magpie-customer-id", "", "Magpie customer id")
		reason := fs.String("reason", "", "required audit reason")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		link, root, err := a.store.UnlinkCustomer(a.ctx, *customerID, *reason)
		if err != nil {
			return err
		}
		return writeJSON(out, map[string]any{"customer_link": link, "root": root})
	case "get":
		fs := newFlagSet("customer get")
		customerID := fs.String("magpie-customer-id", "", "Magpie customer id")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		view, err := a.store.Customer(a.ctx, *customerID)
		if err != nil {
			return err
		}
		return writeJSON(out, view)
	case "list":
		fs := newFlagSet("customer list")
		includeRemoved := fs.Bool("include-removed", false, "include removed links")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		links, err := a.store.ListCustomerLinks(a.ctx, *includeRemoved)
		if err != nil {
			return err
		}
		return writeJSON(out, links)
	default:
		return fmt.Errorf("unknown customer command %q", args[0])
	}
}

func (a app) pipeline(args []string, out io.Writer) error {
	fs := newFlagSet("pipeline")
	owner := fs.String("owner", "", "filter by owner")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := a.store.Pipeline(a.ctx, *owner)
	if err != nil {
		return err
	}
	return writeJSON(out, report)
}

func (a app) today(args []string, out io.Writer) error {
	fs := newFlagSet("today")
	owner := fs.String("owner", "", "filter by owner")
	asOf := fs.String("as-of", "", "date YYYY-MM-DD; defaults to today")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := a.store.Today(a.ctx, *owner, *asOf)
	if err != nil {
		return err
	}
	return writeJSON(out, report)
}

func (a app) search(args []string, out io.Writer) error {
	fs := newFlagSet("search")
	query := fs.String("query", "", "case-insensitive search text")
	if err := fs.Parse(args); err != nil {
		return err
	}
	results, err := a.store.Search(a.ctx, *query)
	if err != nil {
		return err
	}
	return writeJSON(out, results)
}

func (a app) importJSON(args []string, out io.Writer) error {
	fs := newFlagSet("import-json")
	file := fs.String("file", "", "normalized import JSON; '-' reads stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := readFile(*file)
	if err != nil {
		return err
	}
	var bundle martin.ImportBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return fmt.Errorf("decode import bundle: %w", err)
	}
	result, root, err := a.store.Import(a.ctx, bundle)
	if err != nil {
		return err
	}
	return writeJSON(out, map[string]any{"import": result, "root": root})
}

func (a app) snapshot(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "create" {
		return fmt.Errorf("usage: snapshot create --name NAME")
	}
	fs := newFlagSet("snapshot create")
	name := fs.String("name", "", "snapshot name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	root, err := a.store.CreateSnapshot(a.ctx, *name)
	if err != nil {
		return err
	}
	return writeJSON(out, map[string]string{"name": "martin-" + *name, "root": root})
}

func usage(out io.Writer) error {
	_, err := fmt.Fprintln(out, `Martin CRM CLI

Core:
  init [--currency USD]
  doctor
  today [--owner ID] [--as-of YYYY-MM-DD]
  pipeline [--owner ID]
  search --query TEXT
  state
  export
  audit
  import-json --file FILE
  snapshot create --name NAME
  version

Organizations:
  organization create --name NAME [--domain DOMAIN] [--email EMAIL] [--phone PHONE] [--owner ID] [--tags a,b]
  organization update --id ID [fields]
  organization get --id ID
  organization list [--include-archived]
  organization archive --id ID --reason REASON
  organization merge --from ID --into ID --reason REASON

People:
  person create --display-name NAME [--organization-id ID] [--email EMAIL] [--phone PHONE] [--title TITLE]
  person update --id ID [fields]
  person get --id ID
  person list [--include-archived]
  person archive --id ID --reason REASON
  person merge --from ID --into ID --reason REASON

Deals:
  deal create --name NAME [--organization-id ID] [--person-id ID] --value-cents N --expected-close DATE --next-action TEXT --next-due DATE
  deal advance --id ID --next-action TEXT --next-due DATE
  deal touch --id ID --kind KIND --summary TEXT --next-action TEXT --next-due DATE
  deal win --id ID --closed-on DATE
  deal lose --id ID --closed-on DATE --reason REASON
  deal reopen --id ID --reason REASON --next-action TEXT --next-due DATE
  deal get --id ID
  deal list [--include-closed]

Relationship work:
  activity log --kind KIND --summary TEXT [--organization-id ID] [--person-id ID] [--deal-id ID]
  activity list [entity filters]
  task create --title TEXT --due DATE [--organization-id ID] [--person-id ID]
  task complete --id ID
  task cancel --id ID --reason REASON
  task list [--owner ID] [--status STATUS]

Bookkeeping bridge:
  customer link [--organization-id ID|--person-id ID] --magpie-customer-id ID
  customer unlink --magpie-customer-id ID --reason REASON
  customer get --magpie-customer-id ID
  customer list [--include-removed]

Global flags:
  --store DIR
  --jaybase-url HTTPS_ORIGIN (or JAYBASE_URL; token from JAYBASE_TOKEN)
  --cache-dir DIR (or MARTIN_CACHE_DIR; encrypted hosted checkpoint)
  --actor USER_ID
  --role ROLE`)
	return err
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	return visited
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func parseRFC3339(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("occurred-at must use RFC3339")
	}
	return value.UTC(), nil
}

func readFile(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("--file is required")
	}
	if path == "-" {
		return io.ReadAll(io.LimitReader(os.Stdin, 64<<20))
	}
	return os.ReadFile(path)
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func writeError(out io.Writer, err error) {
	var appError *martin.AppError
	if errors.As(err, &appError) {
		_ = writeJSON(out, appError)
		return
	}
	_ = writeJSON(out, &martin.AppError{Code: martin.ErrValidation, Message: err.Error()})
}
