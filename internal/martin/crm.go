package martin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// CRM is the first-class programmatic CRM API. The CLI and the MCP server
// are adapters over this type. They do not wrap each other.
type CRM struct {
	Store *Store
	Ctx   Context
}

func NewCRM(store *Store, ctx Context) *CRM {
	if strings.TrimSpace(ctx.Actor) == "" {
		ctx.Actor = "owner"
	}
	return &CRM{Store: store, Ctx: ctx}
}

// Tool describes one Martin operation as an MCP tool. Names are stable and
// map 1:1 onto existing Store operations, not onto argv strings.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Invoke dispatches a named CRM operation. params is a JSON object. This is
// the MCP tools/call path. Init remains the CLI command and is not a tool.
func (c *CRM) Invoke(name string, params json.RawMessage) (any, error) {
	switch strings.TrimSpace(name) {
	case "doctor":
		return c.doctor()
	case "today":
		var in struct {
			OwnerID string `json:"owner_id"`
			AsOf    string `json:"as_of"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		return c.Store.Today(c.Ctx, in.OwnerID, in.AsOf)
	case "pipeline":
		var in struct {
			OwnerID string `json:"owner_id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		return c.Store.Pipeline(c.Ctx, in.OwnerID)
	case "search":
		var in struct {
			Query string `json:"query"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		results, err := c.Store.Search(c.Ctx, in.Query)
		if err != nil {
			return nil, err
		}
		return catalogObject("results", results), nil
	case "state", "export":
		return c.Store.Export(c.Ctx)
	case "audit":
		nodes, err := c.Store.Audit(c.Ctx)
		if err != nil {
			return nil, err
		}
		return catalogObject("events", nodes), nil
	case "import_json":
		var bundle ImportBundle
		if err := decodeParams(params, &bundle); err != nil {
			return nil, err
		}
		result, root, err := c.Store.Import(c.Ctx, bundle)
		if err != nil {
			return nil, err
		}
		return map[string]any{"import": result, "root": root}, nil
	case "snapshot_create":
		var in struct {
			Name string `json:"name"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		root, err := c.Store.CreateSnapshot(c.Ctx, in.Name)
		if err != nil {
			return nil, err
		}
		return map[string]string{"name": "martin-" + in.Name, "root": root}, nil
	case "organization_create":
		var organization Organization
		if err := decodeParams(params, &organization); err != nil {
			return nil, err
		}
		entity, root, err := c.Store.CreateOrganization(c.Ctx, organization)
		if err != nil {
			return nil, err
		}
		return map[string]any{"organization": entity, "root": root}, nil
	case "organization_update":
		var in organizationUpdateArgs
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		entity, err := c.Store.GetOrganization(c.Ctx, in.ID)
		if err != nil {
			return nil, err
		}
		in.apply(&entity)
		entity, root, err := c.Store.UpdateOrganization(c.Ctx, entity)
		if err != nil {
			return nil, err
		}
		return map[string]any{"organization": entity, "root": root}, nil
	case "organization_get":
		var in struct {
			ID string `json:"id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		return c.Store.GetOrganization(c.Ctx, in.ID)
	case "organization_list":
		var in struct {
			IncludeArchived bool `json:"include_archived"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		entities, err := c.Store.ListOrganizations(c.Ctx, in.IncludeArchived)
		if err != nil {
			return nil, err
		}
		return catalogObject("organizations", entities), nil
	case "organization_archive":
		var in struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		entity, root, err := c.Store.ArchiveOrganization(c.Ctx, in.ID, in.Reason)
		if err != nil {
			return nil, err
		}
		return map[string]any{"organization": entity, "root": root}, nil
	case "organization_merge":
		var in struct {
			From   string `json:"from"`
			Into   string `json:"into"`
			Reason string `json:"reason"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		root, err := c.Store.MergeOrganizations(c.Ctx, in.From, in.Into, in.Reason)
		if err != nil {
			return nil, err
		}
		return map[string]any{"from": in.From, "into": in.Into, "root": root}, nil
	case "person_create":
		var person Person
		if err := decodeParams(params, &person); err != nil {
			return nil, err
		}
		entity, root, err := c.Store.CreatePerson(c.Ctx, person)
		if err != nil {
			return nil, err
		}
		return map[string]any{"person": entity, "root": root}, nil
	case "person_update":
		var in personUpdateArgs
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		entity, err := c.Store.GetPerson(c.Ctx, in.ID)
		if err != nil {
			return nil, err
		}
		in.apply(&entity)
		entity, root, err := c.Store.UpdatePerson(c.Ctx, entity)
		if err != nil {
			return nil, err
		}
		return map[string]any{"person": entity, "root": root}, nil
	case "person_get":
		var in struct {
			ID string `json:"id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		return c.Store.GetPerson(c.Ctx, in.ID)
	case "person_list":
		var in struct {
			IncludeArchived bool `json:"include_archived"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		entities, err := c.Store.ListPeople(c.Ctx, in.IncludeArchived)
		if err != nil {
			return nil, err
		}
		return catalogObject("people", entities), nil
	case "person_archive":
		var in struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		entity, root, err := c.Store.ArchivePerson(c.Ctx, in.ID, in.Reason)
		if err != nil {
			return nil, err
		}
		return map[string]any{"person": entity, "root": root}, nil
	case "person_merge":
		var in struct {
			From   string `json:"from"`
			Into   string `json:"into"`
			Reason string `json:"reason"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		root, err := c.Store.MergePeople(c.Ctx, in.From, in.Into, in.Reason)
		if err != nil {
			return nil, err
		}
		return map[string]any{"from": in.From, "into": in.Into, "root": root}, nil
	case "deal_create":
		var in struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			OrganizationID string `json:"organization_id"`
			PersonID       string `json:"person_id"`
			OwnerID        string `json:"owner_id"`
			ValueCents     int64  `json:"value_cents"`
			ExpectedClose  string `json:"expected_close"`
			NextAction     string `json:"next_action"`
			NextDue        string `json:"next_due"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		deal, task, root, err := c.Store.CreateDeal(c.Ctx, Deal{
			ID: in.ID, Name: in.Name, OrganizationID: in.OrganizationID, PersonID: in.PersonID,
			OwnerID: in.OwnerID, ValueCents: in.ValueCents, ExpectedClose: in.ExpectedClose,
		}, in.NextAction, in.NextDue)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deal": deal, "next_action": task, "root": root}, nil
	case "deal_advance":
		var in struct {
			ID         string `json:"id"`
			NextAction string `json:"next_action"`
			NextDue    string `json:"next_due"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		deal, task, root, err := c.Store.AdvanceDeal(c.Ctx, in.ID, in.NextAction, in.NextDue)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deal": deal, "next_action": task, "root": root}, nil
	case "deal_touch":
		var in struct {
			ID         string       `json:"id"`
			Kind       ActivityKind `json:"kind"`
			Summary    string       `json:"summary"`
			OccurredAt string       `json:"occurred_at"`
			NextAction string       `json:"next_action"`
			NextDue    string       `json:"next_due"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		at, err := parseOccurredAt(in.OccurredAt)
		if err != nil {
			return nil, err
		}
		deal, activity, task, root, err := c.Store.TouchDeal(c.Ctx, in.ID, in.Kind, in.Summary, at, in.NextAction, in.NextDue)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deal": deal, "activity": activity, "next_action": task, "root": root}, nil
	case "deal_win":
		var in struct {
			ID       string `json:"id"`
			ClosedOn string `json:"closed_on"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		deal, root, err := c.Store.WinDeal(c.Ctx, in.ID, in.ClosedOn)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deal": deal, "root": root}, nil
	case "deal_lose":
		var in struct {
			ID       string `json:"id"`
			ClosedOn string `json:"closed_on"`
			Reason   string `json:"reason"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		deal, root, err := c.Store.LoseDeal(c.Ctx, in.ID, in.ClosedOn, in.Reason)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deal": deal, "root": root}, nil
	case "deal_reopen":
		var in struct {
			ID         string `json:"id"`
			Reason     string `json:"reason"`
			NextAction string `json:"next_action"`
			NextDue    string `json:"next_due"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		deal, task, root, err := c.Store.ReopenDeal(c.Ctx, in.ID, in.Reason, in.NextAction, in.NextDue)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deal": deal, "next_action": task, "root": root}, nil
	case "deal_get":
		var in struct {
			ID string `json:"id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		return c.Store.GetDeal(c.Ctx, in.ID)
	case "deal_list":
		var in struct {
			IncludeClosed bool `json:"include_closed"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		deals, err := c.Store.ListDeals(c.Ctx, in.IncludeClosed)
		if err != nil {
			return nil, err
		}
		return catalogObject("deals", deals), nil
	case "activity_log":
		var in struct {
			Kind           ActivityKind `json:"kind"`
			Summary        string       `json:"summary"`
			OccurredAt     string       `json:"occurred_at"`
			OrganizationID string       `json:"organization_id"`
			PersonID       string       `json:"person_id"`
			DealID         string       `json:"deal_id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		at, err := parseOccurredAt(in.OccurredAt)
		if err != nil {
			return nil, err
		}
		activity, root, err := c.Store.LogActivity(c.Ctx, Activity{
			Kind: in.Kind, Summary: in.Summary, OccurredAt: at,
			OrganizationID: in.OrganizationID, PersonID: in.PersonID, DealID: in.DealID,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"activity": activity, "root": root}, nil
	case "activity_list":
		var in struct {
			OrganizationID string `json:"organization_id"`
			PersonID       string `json:"person_id"`
			DealID         string `json:"deal_id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		activities, err := c.Store.ListActivities(c.Ctx, in.OrganizationID, in.PersonID, in.DealID)
		if err != nil {
			return nil, err
		}
		return catalogObject("activities", activities), nil
	case "task_create":
		var in struct {
			Title          string `json:"title"`
			DueDate        string `json:"due"`
			OwnerID        string `json:"owner_id"`
			OrganizationID string `json:"organization_id"`
			PersonID       string `json:"person_id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		task, root, err := c.Store.CreateTask(c.Ctx, Task{
			Title: in.Title, DueDate: in.DueDate, OwnerID: in.OwnerID,
			OrganizationID: in.OrganizationID, PersonID: in.PersonID,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"task": task, "root": root}, nil
	case "task_complete":
		var in struct {
			ID string `json:"id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		task, root, err := c.Store.CompleteTask(c.Ctx, in.ID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"task": task, "root": root}, nil
	case "task_cancel":
		var in struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		task, root, err := c.Store.CancelTask(c.Ctx, in.ID, in.Reason)
		if err != nil {
			return nil, err
		}
		return map[string]any{"task": task, "root": root}, nil
	case "task_list":
		var in struct {
			OwnerID string     `json:"owner_id"`
			Status  TaskStatus `json:"status"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		tasks, err := c.Store.ListTasks(c.Ctx, in.OwnerID, in.Status)
		if err != nil {
			return nil, err
		}
		return catalogObject("tasks", tasks), nil
	case "customer_link":
		var in struct {
			OrganizationID   string `json:"organization_id"`
			PersonID         string `json:"person_id"`
			MagpieCustomerID string `json:"magpie_customer_id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		link, root, err := c.Store.LinkCustomer(c.Ctx, in.OrganizationID, in.PersonID, in.MagpieCustomerID)
		if err != nil {
			return nil, err
		}
		return map[string]any{"customer_link": link, "root": root}, nil
	case "customer_unlink":
		var in struct {
			MagpieCustomerID string `json:"magpie_customer_id"`
			Reason           string `json:"reason"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		link, root, err := c.Store.UnlinkCustomer(c.Ctx, in.MagpieCustomerID, in.Reason)
		if err != nil {
			return nil, err
		}
		return map[string]any{"customer_link": link, "root": root}, nil
	case "customer_get":
		var in struct {
			MagpieCustomerID string `json:"magpie_customer_id"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		return c.Store.Customer(c.Ctx, in.MagpieCustomerID)
	case "customer_list":
		var in struct {
			IncludeRemoved bool `json:"include_removed"`
		}
		if err := decodeParams(params, &in); err != nil {
			return nil, err
		}
		links, err := c.Store.ListCustomerLinks(c.Ctx, in.IncludeRemoved)
		if err != nil {
			return nil, err
		}
		return catalogObject("customer_links", links), nil
	default:
		return nil, appErr(ErrValidation, "unknown Martin operation %q", name)
	}
}

func (c *CRM) doctor() (map[string]any, error) {
	state, err := c.Store.Export(c.Ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok": true, "root": state.Root, "store": c.Store.Dir(), "workspace": state.Workspace,
		"counts": map[string]int{
			"organizations": len(state.Organizations), "people": len(state.People), "deals": len(state.Deals),
			"activities": len(state.Activities), "tasks": len(state.Tasks), "customer_links": len(state.CustomerLinks),
		},
	}, nil
}

type organizationUpdateArgs struct {
	ID      string    `json:"id"`
	Name    *string   `json:"name"`
	Domain  *string   `json:"domain"`
	Email   *string   `json:"email"`
	Phone   *string   `json:"phone"`
	OwnerID *string   `json:"owner_id"`
	Tags    *[]string `json:"tags"`
}

func (in organizationUpdateArgs) apply(entity *Organization) {
	if in.Name != nil {
		entity.Name = *in.Name
	}
	if in.Domain != nil {
		entity.Domain = *in.Domain
	}
	if in.Email != nil {
		entity.Email = *in.Email
	}
	if in.Phone != nil {
		entity.Phone = *in.Phone
	}
	if in.OwnerID != nil {
		entity.OwnerID = *in.OwnerID
	}
	if in.Tags != nil {
		entity.Tags = *in.Tags
	}
}

type personUpdateArgs struct {
	ID             string    `json:"id"`
	DisplayName    *string   `json:"display_name"`
	OrganizationID *string   `json:"organization_id"`
	Title          *string   `json:"title"`
	Email          *string   `json:"email"`
	Phone          *string   `json:"phone"`
	OwnerID        *string   `json:"owner_id"`
	Tags           *[]string `json:"tags"`
}

func (in personUpdateArgs) apply(entity *Person) {
	if in.DisplayName != nil {
		entity.DisplayName = *in.DisplayName
	}
	if in.OrganizationID != nil {
		entity.OrganizationID = *in.OrganizationID
	}
	if in.Title != nil {
		entity.Title = *in.Title
	}
	if in.Email != nil {
		entity.Email = *in.Email
	}
	if in.Phone != nil {
		entity.Phone = *in.Phone
	}
	if in.OwnerID != nil {
		entity.OwnerID = *in.OwnerID
	}
	if in.Tags != nil {
		entity.Tags = *in.Tags
	}
}

func decodeParams[T any](raw json.RawMessage, dest *T) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return appErr(ErrValidation, "invalid arguments: %s", err)
	}
	return nil
}

func ToolCatalog() []Tool {
	ids := "Use opaque Martin IDs returned by prior calls. Do not invent them."
	cents := "Integer minor units. Never send floating-point dollars."
	date := "Business date YYYY-MM-DD."
	stages := "Pipeline stages are new, qualified, proposal, won, lost. Every open deal has exactly one next action."
	return []Tool{
		tool("doctor", "Workspace health: root, currency, and entity counts. Requires an initialized store.", objectSchema(nil, nil)),
		tool("today", "Due and overdue tasks as of a date. "+date, objectSchema(nil, map[string]any{
			"owner_id": map[string]any{"type": "string"},
			"as_of":    map[string]any{"type": "string", "description": date},
		})),
		tool("pipeline", "Open-deal pipeline totals by stage. "+stages, objectSchema(nil, map[string]any{
			"owner_id": map[string]any{"type": "string"},
		})),
		tool("search", "Case-insensitive search of organizations, people, and deals.", objectSchema([]string{"query"}, map[string]any{
			"query": map[string]any{"type": "string"},
		})),
		tool("state", "Reconstructed CRM state. Broad; prefer list tools for ordinary work.", objectSchema(nil, nil)),
		tool("export", "Alias of state. Reconstructed CRM state.", objectSchema(nil, nil)),
		tool("audit", "Martin event history as an object with an events array. Metadata only.", objectSchema(nil, nil)),
		tool("import_json", "Idempotent normalized import keyed by source and source_key. Requires manage access.", objectSchema([]string{"source", "source_key"}, map[string]any{
			"source":         map[string]any{"type": "string"},
			"source_key":     map[string]any{"type": "string"},
			"organizations":  map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"people":         map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"deals":          map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"activities":     map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"tasks":          map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"customer_links": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		})),
		tool("snapshot_create", "Create a named local root checkpoint. This is not an off-host backup.", objectSchema([]string{"name"}, map[string]any{
			"name": map[string]any{"type": "string"},
		})),
		tool("organization_create", "Create an organization. Domains are unique across active organizations.", objectSchema([]string{"name"}, map[string]any{
			"id":       map[string]any{"type": "string"},
			"name":     map[string]any{"type": "string"},
			"domain":   map[string]any{"type": "string"},
			"email":    map[string]any{"type": "string"},
			"phone":    map[string]any{"type": "string"},
			"owner_id": map[string]any{"type": "string"},
			"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		})),
		tool("organization_update", "Update an organization by id. Only supplied fields change.", objectSchema([]string{"id"}, map[string]any{
			"id":       map[string]any{"type": "string", "description": ids},
			"name":     map[string]any{"type": "string"},
			"domain":   map[string]any{"type": "string"},
			"email":    map[string]any{"type": "string"},
			"phone":    map[string]any{"type": "string"},
			"owner_id": map[string]any{"type": "string"},
			"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		})),
		tool("organization_get", "Get one organization.", objectSchema([]string{"id"}, map[string]any{"id": map[string]any{"type": "string"}})),
		tool("organization_list", "List organizations as an object with an organizations array.", objectSchema(nil, map[string]any{
			"include_archived": map[string]any{"type": "boolean"},
		})),
		tool("organization_archive", "Archive an organization. There is no delete.", objectSchema([]string{"id", "reason"}, map[string]any{
			"id":     map[string]any{"type": "string"},
			"reason": map[string]any{"type": "string"},
		})),
		tool("organization_merge", "Merge a duplicate organization into a survivor. Requires manage access.", objectSchema([]string{"from", "into", "reason"}, map[string]any{
			"from":   map[string]any{"type": "string"},
			"into":   map[string]any{"type": "string"},
			"reason": map[string]any{"type": "string"},
		})),
		tool("person_create", "Create a person. Emails are unique across active people, case-insensitively.", objectSchema([]string{"display_name"}, map[string]any{
			"id":              map[string]any{"type": "string"},
			"display_name":    map[string]any{"type": "string"},
			"organization_id": map[string]any{"type": "string"},
			"title":           map[string]any{"type": "string"},
			"email":           map[string]any{"type": "string"},
			"phone":           map[string]any{"type": "string"},
			"owner_id":        map[string]any{"type": "string"},
			"tags":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		})),
		tool("person_update", "Update a person by id. Only supplied fields change. Empty organization_id clears the link.", objectSchema([]string{"id"}, map[string]any{
			"id":              map[string]any{"type": "string", "description": ids},
			"display_name":    map[string]any{"type": "string"},
			"organization_id": map[string]any{"type": "string"},
			"title":           map[string]any{"type": "string"},
			"email":           map[string]any{"type": "string"},
			"phone":           map[string]any{"type": "string"},
			"owner_id":        map[string]any{"type": "string"},
			"tags":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		})),
		tool("person_get", "Get one person.", objectSchema([]string{"id"}, map[string]any{"id": map[string]any{"type": "string"}})),
		tool("person_list", "List people as an object with a people array.", objectSchema(nil, map[string]any{
			"include_archived": map[string]any{"type": "boolean"},
		})),
		tool("person_archive", "Archive a person. There is no delete.", objectSchema([]string{"id", "reason"}, map[string]any{
			"id":     map[string]any{"type": "string"},
			"reason": map[string]any{"type": "string"},
		})),
		tool("person_merge", "Merge a duplicate person into a survivor. Requires manage access.", objectSchema([]string{"from", "into", "reason"}, map[string]any{
			"from":   map[string]any{"type": "string"},
			"into":   map[string]any{"type": "string"},
			"reason": map[string]any{"type": "string"},
		})),
		tool("deal_create", "Create an open deal. Requires next_action and next_due. "+cents+" "+date+" "+stages, objectSchema([]string{"name", "value_cents", "expected_close", "next_action", "next_due"}, map[string]any{
			"id":              map[string]any{"type": "string"},
			"name":            map[string]any{"type": "string"},
			"organization_id": map[string]any{"type": "string"},
			"person_id":       map[string]any{"type": "string"},
			"owner_id":        map[string]any{"type": "string"},
			"value_cents":     map[string]any{"type": "integer", "description": cents},
			"expected_close":  map[string]any{"type": "string", "description": date},
			"next_action":     map[string]any{"type": "string"},
			"next_due":        map[string]any{"type": "string", "description": date},
		})),
		tool("deal_advance", "Advance one open stage and replace the next action. Proposal-stage deals must be won or lost.", objectSchema([]string{"id", "next_action", "next_due"}, map[string]any{
			"id":          map[string]any{"type": "string", "description": ids},
			"next_action": map[string]any{"type": "string"},
			"next_due":    map[string]any{"type": "string", "description": date},
		})),
		tool("deal_touch", "Log an interaction and replace the next action in one write.", objectSchema([]string{"id", "kind", "summary", "next_action", "next_due"}, map[string]any{
			"id":          map[string]any{"type": "string", "description": ids},
			"kind":        map[string]any{"type": "string", "enum": []string{"call", "email", "meeting", "note"}},
			"summary":     map[string]any{"type": "string"},
			"occurred_at": map[string]any{"type": "string", "description": "RFC3339 timestamp; defaults to now."},
			"next_action": map[string]any{"type": "string"},
			"next_due":    map[string]any{"type": "string", "description": date},
		})),
		tool("deal_win", "Win a proposal-stage deal and close its next action.", objectSchema([]string{"id", "closed_on"}, map[string]any{
			"id":        map[string]any{"type": "string"},
			"closed_on": map[string]any{"type": "string", "description": date},
		})),
		tool("deal_lose", "Lose a deal with a reason and close its next action.", objectSchema([]string{"id", "closed_on", "reason"}, map[string]any{
			"id":        map[string]any{"type": "string"},
			"closed_on": map[string]any{"type": "string", "description": date},
			"reason":    map[string]any{"type": "string"},
		})),
		tool("deal_reopen", "Reopen a closed deal with a new next action. Requires manage access.", objectSchema([]string{"id", "reason", "next_action", "next_due"}, map[string]any{
			"id":          map[string]any{"type": "string"},
			"reason":      map[string]any{"type": "string"},
			"next_action": map[string]any{"type": "string"},
			"next_due":    map[string]any{"type": "string", "description": date},
		})),
		tool("deal_get", "Get one deal.", objectSchema([]string{"id"}, map[string]any{"id": map[string]any{"type": "string"}})),
		tool("deal_list", "List deals as an object with a deals array.", objectSchema(nil, map[string]any{
			"include_closed": map[string]any{"type": "boolean"},
		})),
		tool("activity_log", "Log a standalone relationship activity. Prefer deal_touch when the interaction belongs to a deal.", objectSchema([]string{"kind", "summary"}, map[string]any{
			"kind":            map[string]any{"type": "string", "enum": []string{"call", "email", "meeting", "note"}},
			"summary":         map[string]any{"type": "string"},
			"occurred_at":     map[string]any{"type": "string", "description": "RFC3339 timestamp; defaults to now."},
			"organization_id": map[string]any{"type": "string"},
			"person_id":       map[string]any{"type": "string"},
			"deal_id":         map[string]any{"type": "string"},
		})),
		tool("activity_list", "List activities as an object with an activities array.", objectSchema(nil, map[string]any{
			"organization_id": map[string]any{"type": "string"},
			"person_id":       map[string]any{"type": "string"},
			"deal_id":         map[string]any{"type": "string"},
		})),
		tool("task_create", "Create a relationship follow-up task. Do not attach generic tasks to deals.", objectSchema([]string{"title", "due"}, map[string]any{
			"title":           map[string]any{"type": "string"},
			"due":             map[string]any{"type": "string", "description": date},
			"owner_id":        map[string]any{"type": "string"},
			"organization_id": map[string]any{"type": "string"},
			"person_id":       map[string]any{"type": "string"},
		})),
		tool("task_complete", "Complete a pending task.", objectSchema([]string{"id"}, map[string]any{"id": map[string]any{"type": "string"}})),
		tool("task_cancel", "Cancel a pending task.", objectSchema([]string{"id", "reason"}, map[string]any{
			"id":     map[string]any{"type": "string"},
			"reason": map[string]any{"type": "string"},
		})),
		tool("task_list", "List tasks as an object with a tasks array.", objectSchema(nil, map[string]any{
			"owner_id": map[string]any{"type": "string"},
			"status":   map[string]any{"type": "string", "enum": []string{"pending", "completed", "canceled"}},
		})),
		tool("customer_link", "Link exactly one organization or person to a Magpie customer. Requires manage access. Never fuzzy-match by name.", objectSchema([]string{"magpie_customer_id"}, map[string]any{
			"organization_id":    map[string]any{"type": "string"},
			"person_id":          map[string]any{"type": "string"},
			"magpie_customer_id": map[string]any{"type": "string"},
		})),
		tool("customer_unlink", "Remove a Magpie customer link. Requires manage access.", objectSchema([]string{"magpie_customer_id", "reason"}, map[string]any{
			"magpie_customer_id": map[string]any{"type": "string"},
			"reason":             map[string]any{"type": "string"},
		})),
		tool("customer_get", "Combined Martin and Magpie customer view for an explicit link.", objectSchema([]string{"magpie_customer_id"}, map[string]any{
			"magpie_customer_id": map[string]any{"type": "string"},
		})),
		tool("customer_list", "List customer links as an object with a customer_links array.", objectSchema(nil, map[string]any{
			"include_removed": map[string]any{"type": "boolean"},
		})),
	}
}

func tool(name, description string, schema map[string]any) Tool {
	return Tool{Name: name, Description: description, InputSchema: schema}
}

// catalogObject wraps a list so MCP structuredContent is a JSON object.
func catalogObject(field string, items any) map[string]any {
	if items == nil {
		return map[string]any{field: []any{}}
	}
	return map[string]any{field: items}
}

func objectSchema(required []string, props map[string]any) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func KnownTool(name string) bool {
	for _, item := range ToolCatalog() {
		if item.Name == name {
			return true
		}
	}
	return false
}

func EncodeJSON(v any) ([]byte, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return buf, nil
}
