package martin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kyle-visner/jaybase"
)

const (
	TypeWorkspaceInitialized = "martin.workspace.initialized.v1"
	TypeOrganizationCreated  = "martin.organization.created.v1"
	TypeOrganizationUpdated  = "martin.organization.updated.v1"
	TypeOrganizationArchived = "martin.organization.archived.v1"
	TypeOrganizationMerged   = "martin.organization.merged.v1"
	TypePersonCreated        = "martin.person.created.v1"
	TypePersonUpdated        = "martin.person.updated.v1"
	TypePersonArchived       = "martin.person.archived.v1"
	TypePersonMerged         = "martin.person.merged.v1"
	TypeDealCreated          = "martin.deal.created.v1"
	TypeDealAdvanced         = "martin.deal.advanced.v1"
	TypeDealTouched          = "martin.deal.touched.v1"
	TypeDealWon              = "martin.deal.won.v1"
	TypeDealLost             = "martin.deal.lost.v1"
	TypeDealReopened         = "martin.deal.reopened.v1"
	TypeActivityLogged       = "martin.activity.logged.v1"
	TypeTaskCreated          = "martin.task.created.v1"
	TypeTaskCompleted        = "martin.task.completed.v1"
	TypeTaskCanceled         = "martin.task.canceled.v1"
	TypeCustomerLinkCreated  = "martin.customer-link.created.v1"
	TypeCustomerLinkRemoved  = "martin.customer-link.removed.v1"
	TypeImportApplied        = "martin.import.applied.v1"
)

type storageBackend interface {
	Close() error
	Dir() string
	CurrentRoot() (string, error)
	AppendAt(jaybase.Context, jaybase.AppendOptions, string) (string, error)
	NodesFromRoot(string) ([]jaybase.Node, error)
	AuditLog() ([]jaybase.Node, error)
	NodePayload(jaybase.Node) ([]byte, error)
	NamedRef(string) (string, error)
	WriteNamedRefAt(string, string, string) error
}

type incrementalReplayBackend interface {
	EventsAfter(string) ([]jaybase.Node, string, error)
}

type stateCheckpointCache interface {
	Load() (State, bool, error)
	Save(State) error
	Invalidate() error
}

type localStorageBackend struct {
	store *jaybase.Store
}

type Store struct {
	db         storageBackend
	now        func() time.Time
	stateCache stateCheckpointCache
}

type entityPayload[T any] struct {
	Entity T `json:"entity"`
}

type dealWorkflowPayload struct {
	Deal          Deal      `json:"deal"`
	CompletedTask *Task     `json:"completed_task,omitempty"`
	NextTask      *Task     `json:"next_task,omitempty"`
	Activity      *Activity `json:"activity,omitempty"`
}

type mergePayload struct {
	FromID string `json:"from_id"`
	IntoID string `json:"into_id"`
	Reason string `json:"reason"`
}

type importPayload struct {
	Receipt       ImportReceipt  `json:"receipt"`
	Organizations []Organization `json:"organizations,omitempty"`
	People        []Person       `json:"people,omitempty"`
	Deals         []Deal         `json:"deals,omitempty"`
	Activities    []Activity     `json:"activities,omitempty"`
	Tasks         []Task         `json:"tasks,omitempty"`
	CustomerLinks []CustomerLink `json:"customer_links,omitempty"`
}

type magpieEnvelope struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

func OpenStore(dir string) (*Store, error) {
	db, err := jaybase.OpenStore(dir)
	if err != nil {
		return nil, storageError(err)
	}
	return newStore(&localStorageBackend{store: db}), nil
}

func newStore(db storageBackend) *Store {
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Store) Close() error {
	return storageError(s.db.Close())
}

func (s *Store) Dir() string {
	return s.db.Dir()
}

func (b *localStorageBackend) Close() error {
	return b.store.Close()
}

func (b *localStorageBackend) Dir() string {
	return b.store.Dir()
}

func (b *localStorageBackend) CurrentRoot() (string, error) {
	return b.store.CurrentRoot()
}

func (b *localStorageBackend) AppendAt(ctx jaybase.Context, options jaybase.AppendOptions, expectedRoot string) (string, error) {
	return b.store.AppendAt(ctx, options, expectedRoot)
}

func (b *localStorageBackend) NodesFromRoot(root string) ([]jaybase.Node, error) {
	return b.store.NodesFromRoot(root)
}

func (b *localStorageBackend) AuditLog() ([]jaybase.Node, error) {
	return b.store.AuditLog()
}

func (b *localStorageBackend) NodePayload(node jaybase.Node) ([]byte, error) {
	return b.store.NodePayload(node)
}

func (b *localStorageBackend) NamedRef(name string) (string, error) {
	return b.store.NamedRef(name)
}

func (b *localStorageBackend) WriteNamedRefAt(name, root, expectedRoot string) error {
	return b.store.WriteNamedRefAt(name, root, expectedRoot)
}

func (s *Store) CurrentRoot() (string, error) {
	root, err := s.db.CurrentRoot()
	return root, storageError(err)
}

func (s *Store) NodesFromRoot(root string) ([]jaybase.Node, error) {
	nodes, err := s.db.NodesFromRoot(root)
	return nodes, storageError(err)
}

func (s *Store) LoadState() (State, error) {
	if s.stateCache != nil {
		return s.loadStateFromCheckpoint()
	}
	root, err := s.CurrentRoot()
	if err != nil {
		return State{}, err
	}
	st := emptyState()
	if root == "" {
		return st, nil
	}
	nodes, err := s.NodesFromRoot(root)
	if err != nil {
		return State{}, err
	}
	for _, node := range nodes {
		if err := s.applyNode(&st, node); err != nil {
			return State{}, err
		}
		st.Root = node.Hash
	}
	return st, nil
}

func (s *Store) loadStateFromCheckpoint() (State, error) {
	replay, ok := s.db.(incrementalReplayBackend)
	if !ok {
		return State{}, appErr(ErrInternal, "state checkpoint configured for a backend without incremental replay")
	}
	st, found, err := s.stateCache.Load()
	if err != nil {
		return State{}, err
	}
	if !found {
		st = emptyState()
	}

replayAttempts:
	for attempt := 0; attempt < 2; attempt++ {
		checkpointRoot := st.Root
		nodes, targetRoot, err := replay.EventsAfter(checkpointRoot)
		if err != nil {
			var dbErr *jaybase.AppError
			if checkpointRoot != "" && errors.As(err, &dbErr) && dbErr.Code == jaybase.ErrNotFound {
				if invalidateErr := s.stateCache.Invalidate(); invalidateErr != nil {
					return State{}, invalidateErr
				}
				st = emptyState()
				found = false
				continue
			}
			return State{}, storageError(err)
		}
		for _, node := range nodes {
			if err := s.applyNode(&st, node); err != nil {
				if found {
					if invalidateErr := s.stateCache.Invalidate(); invalidateErr != nil {
						return State{}, invalidateErr
					}
					st = emptyState()
					found = false
					continue replayAttempts
				}
				return State{}, err
			}
			st.Root = node.Hash
		}
		if st.Root != targetRoot {
			if found {
				if invalidateErr := s.stateCache.Invalidate(); invalidateErr != nil {
					return State{}, invalidateErr
				}
				st = emptyState()
				found = false
				continue
			}
			return State{}, appErr(ErrIntegrity, "Jaybase incremental replay ended at %q instead of captured root %q", st.Root, targetRoot)
		}
		if !found || len(nodes) > 0 {
			_ = s.stateCache.Save(st)
		}
		return st, nil
	}
	return State{}, appErr(ErrNotFound, "cached Jaybase root was not found after a cold replay")
}

func (s *Store) Initialize(ctx Context, currency string) (Workspace, string, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(currency) {
		return Workspace{}, "", appErr(ErrValidation, "currency must be a three-letter ISO code")
	}
	for attempt := 0; attempt < 4; attempt++ {
		st, err := s.LoadState()
		if err != nil {
			return Workspace{}, "", err
		}
		if st.Workspace != nil {
			if st.Workspace.Currency != currency {
				return Workspace{}, "", appErr(ErrConflict, "Martin is already initialized with currency %s", st.Workspace.Currency)
			}
			return *st.Workspace, st.Root, nil
		}
		if err := ensureAccess(st, ctx, accessManage); err != nil {
			return Workspace{}, "", err
		}
		workspace := Workspace{Currency: currency, InitializedAt: s.now(), InitializedBy: ctx.Actor}
		root, err := s.appendAt(ctx, TypeWorkspaceInitialized, "workspace:default", "init", workspace, st.Root)
		if err == nil {
			return workspace, root, nil
		}
		var appError *AppError
		if !errors.As(err, &appError) || appError.Code != ErrConflict {
			return Workspace{}, "", err
		}
	}
	return Workspace{}, "", appErr(ErrConflict, "Martin initialization conflicted repeatedly; retry")
}

func (s *Store) appendAt(ctx Context, typ, entityID, command string, payload any, expectedRoot string) (string, error) {
	if strings.TrimSpace(ctx.Actor) == "" {
		return "", appErr(ErrPermission, "actor is required")
	}
	hash, err := s.db.AppendAt(jaybase.Context{Actor: ctx.Actor, Role: ctx.Role}, jaybase.AppendOptions{
		Type: typ, EntityID: entityID, Command: command, Payload: payload, CreatedAt: s.now(),
	}, expectedRoot)
	return hash, storageError(err)
}

func (s *Store) applyNode(st *State, node jaybase.Node) error {
	switch node.Type {
	case "store.init", "rbac.role", "rbac.user", "customer", "invoice":
		return s.applyMagpieNode(st, node)
	}
	if !strings.HasPrefix(node.Type, "martin.") {
		return nil
	}
	payload, err := s.db.NodePayload(node)
	if err != nil {
		return storageError(err)
	}
	switch node.Type {
	case TypeWorkspaceInitialized:
		var workspace Workspace
		if err := json.Unmarshal(payload, &workspace); err != nil {
			return err
		}
		st.Workspace = &workspace
	case TypeOrganizationCreated, TypeOrganizationUpdated, TypeOrganizationArchived:
		var event entityPayload[Organization]
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		st.Organizations[event.Entity.ID] = event.Entity
	case TypeOrganizationMerged:
		var event mergePayload
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		from, ok := st.Organizations[event.FromID]
		if !ok {
			return appErr(ErrIntegrity, "organization merge references unknown source %s", event.FromID)
		}
		from.Archived = true
		from.ArchiveReason = event.Reason
		from.MergedIntoID = event.IntoID
		st.Organizations[event.FromID] = from
		repointOrganization(st, event.FromID, event.IntoID)
	case TypePersonCreated, TypePersonUpdated, TypePersonArchived:
		var event entityPayload[Person]
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		st.People[event.Entity.ID] = event.Entity
	case TypePersonMerged:
		var event mergePayload
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		from, ok := st.People[event.FromID]
		if !ok {
			return appErr(ErrIntegrity, "person merge references unknown source %s", event.FromID)
		}
		from.Archived = true
		from.ArchiveReason = event.Reason
		from.MergedIntoID = event.IntoID
		st.People[event.FromID] = from
		repointPerson(st, event.FromID, event.IntoID)
	case TypeDealCreated, TypeDealAdvanced, TypeDealTouched, TypeDealWon, TypeDealLost, TypeDealReopened:
		var event dealWorkflowPayload
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		st.Deals[event.Deal.ID] = event.Deal
		if event.CompletedTask != nil {
			st.Tasks[event.CompletedTask.ID] = *event.CompletedTask
		}
		if event.NextTask != nil {
			st.Tasks[event.NextTask.ID] = *event.NextTask
		}
		if event.Activity != nil {
			st.Activities[event.Activity.ID] = *event.Activity
		}
	case TypeActivityLogged:
		var event entityPayload[Activity]
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		st.Activities[event.Entity.ID] = event.Entity
	case TypeTaskCreated, TypeTaskCompleted, TypeTaskCanceled:
		var event entityPayload[Task]
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		st.Tasks[event.Entity.ID] = event.Entity
	case TypeCustomerLinkCreated, TypeCustomerLinkRemoved:
		var event entityPayload[CustomerLink]
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		st.CustomerLinks[event.Entity.ID] = event.Entity
	case TypeImportApplied:
		var event importPayload
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		applyImport(st, event)
		event.Receipt.Root = node.Hash
		st.Imports[importKey(event.Receipt.Source, event.Receipt.SourceKey)] = event.Receipt
	default:
		return appErr(ErrIntegrity, "unknown Martin event type %q in node %s", node.Type, node.Hash)
	}
	return nil
}

func (s *Store) applyMagpieNode(st *State, node jaybase.Node) error {
	payload, err := s.db.NodePayload(node)
	if err != nil {
		return storageError(err)
	}
	var envelope magpieEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	switch envelope.Kind {
	case "init":
		var event struct {
			Roles map[string]MagpieRole `json:"roles"`
			Users map[string]MagpieUser `json:"users"`
		}
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			return err
		}
		st.MagpieRoles = event.Roles
		st.MagpieUsers = event.Users
	case "role.upsert":
		var event struct {
			Role MagpieRole `json:"role"`
		}
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			return err
		}
		st.MagpieRoles[event.Role.Name] = event.Role
	case "user.upsert":
		var event struct {
			User MagpieUser `json:"user"`
		}
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			return err
		}
		st.MagpieUsers[event.User.ID] = event.User
	case "customer.upsert":
		var event struct {
			Customer MagpieCustomer `json:"customer"`
		}
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			return err
		}
		st.MagpieCustomers[event.Customer.ID] = event.Customer
	case "invoice.create", "invoice.update":
		var event struct {
			Invoice MagpieInvoice `json:"invoice"`
		}
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			return err
		}
		st.MagpieInvoices[event.Invoice.ID] = event.Invoice
	}
	return nil
}

func (s *Store) Audit(ctx Context) ([]jaybase.Node, error) {
	st, err := s.LoadState()
	if err != nil {
		return nil, err
	}
	if err := ensureAccess(st, ctx, accessRead); err != nil {
		return nil, err
	}
	nodes, err := s.db.AuditLog()
	if err != nil {
		return nil, storageError(err)
	}
	out := make([]jaybase.Node, 0)
	for _, node := range nodes {
		if strings.HasPrefix(node.Type, "martin.") {
			node.Payload = nil
			node.SealedPayload = nil
			out = append(out, node)
		}
	}
	return out, nil
}

var snapshotNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func (s *Store) CreateSnapshot(ctx Context, name string) (string, error) {
	st, err := s.LoadState()
	if err != nil {
		return "", err
	}
	if err := ensureAccess(st, ctx, accessManage); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if !snapshotNamePattern.MatchString(name) {
		return "", appErr(ErrValidation, "snapshot name must be a simple file-safe name")
	}
	if st.Root == "" {
		return "", appErr(ErrValidation, "cannot snapshot an empty store")
	}
	refName := "martin-" + name
	expected, err := s.db.NamedRef(refName)
	if err != nil {
		var dbErr *jaybase.AppError
		if !errors.As(err, &dbErr) || dbErr.Code != jaybase.ErrNotFound {
			return "", storageError(err)
		}
		expected = ""
	}
	if expected == st.Root {
		return st.Root, nil
	}
	if err := s.db.WriteNamedRefAt(refName, st.Root, expected); err != nil {
		current, readErr := s.db.NamedRef(refName)
		if readErr == nil && current == st.Root {
			return st.Root, nil
		}
		return "", storageError(err)
	}
	return st.Root, nil
}

func makeID(prefix string) (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + ":" + hex.EncodeToString(raw[:]), nil
}

func applyImport(st *State, event importPayload) {
	for _, entity := range event.Organizations {
		st.Organizations[entity.ID] = entity
	}
	for _, entity := range event.People {
		st.People[entity.ID] = entity
	}
	for _, entity := range event.Deals {
		st.Deals[entity.ID] = entity
	}
	for _, entity := range event.Activities {
		st.Activities[entity.ID] = entity
	}
	for _, entity := range event.Tasks {
		st.Tasks[entity.ID] = entity
	}
	for _, entity := range event.CustomerLinks {
		st.CustomerLinks[entity.ID] = entity
	}
}

func repointOrganization(st *State, fromID, intoID string) {
	for id, person := range st.People {
		if person.OrganizationID == fromID {
			person.OrganizationID = intoID
			st.People[id] = person
		}
	}
	for id, deal := range st.Deals {
		if deal.OrganizationID == fromID {
			deal.OrganizationID = intoID
			st.Deals[id] = deal
		}
	}
	for id, activity := range st.Activities {
		if activity.OrganizationID == fromID {
			activity.OrganizationID = intoID
			st.Activities[id] = activity
		}
	}
	for id, task := range st.Tasks {
		if task.OrganizationID == fromID {
			task.OrganizationID = intoID
			st.Tasks[id] = task
		}
	}
	for id, link := range st.CustomerLinks {
		if link.OrganizationID == fromID {
			link.OrganizationID = intoID
			st.CustomerLinks[id] = link
		}
	}
}

func repointPerson(st *State, fromID, intoID string) {
	for id, deal := range st.Deals {
		if deal.PersonID == fromID {
			deal.PersonID = intoID
			st.Deals[id] = deal
		}
	}
	for id, activity := range st.Activities {
		if activity.PersonID == fromID {
			activity.PersonID = intoID
			st.Activities[id] = activity
		}
	}
	for id, task := range st.Tasks {
		if task.PersonID == fromID {
			task.PersonID = intoID
			st.Tasks[id] = task
		}
	}
	for id, link := range st.CustomerLinks {
		if link.PersonID == fromID {
			link.PersonID = intoID
			st.CustomerLinks[id] = link
		}
	}
}
