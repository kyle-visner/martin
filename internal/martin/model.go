package martin

import (
	"sort"
	"strings"
	"time"
)

type Context struct {
	Actor string
	Role  string
}

type Workspace struct {
	Currency      string    `json:"currency"`
	InitializedAt time.Time `json:"initialized_at"`
	InitializedBy string    `json:"initialized_by"`
}

type ExternalRef struct {
	SourceSystem string            `json:"source_system"`
	ExternalID   string            `json:"external_id"`
	ExternalType string            `json:"external_type,omitempty"`
	DisplayName  string            `json:"display_name,omitempty"`
	URL          string            `json:"url,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type Organization struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Domain        string        `json:"domain,omitempty"`
	Email         string        `json:"email,omitempty"`
	Phone         string        `json:"phone,omitempty"`
	OwnerID       string        `json:"owner_id,omitempty"`
	Tags          []string      `json:"tags,omitempty"`
	ExternalRefs  []ExternalRef `json:"external_refs,omitempty"`
	Archived      bool          `json:"archived,omitempty"`
	ArchiveReason string        `json:"archive_reason,omitempty"`
	MergedIntoID  string        `json:"merged_into_id,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	CreatedBy     string        `json:"created_by"`
	UpdatedBy     string        `json:"updated_by"`
}

type Person struct {
	ID             string        `json:"id"`
	DisplayName    string        `json:"display_name"`
	OrganizationID string        `json:"organization_id,omitempty"`
	Title          string        `json:"title,omitempty"`
	Email          string        `json:"email,omitempty"`
	Phone          string        `json:"phone,omitempty"`
	OwnerID        string        `json:"owner_id,omitempty"`
	Tags           []string      `json:"tags,omitempty"`
	ExternalRefs   []ExternalRef `json:"external_refs,omitempty"`
	Archived       bool          `json:"archived,omitempty"`
	ArchiveReason  string        `json:"archive_reason,omitempty"`
	MergedIntoID   string        `json:"merged_into_id,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	CreatedBy      string        `json:"created_by"`
	UpdatedBy      string        `json:"updated_by"`
}

type DealStage string

const (
	DealNew       DealStage = "new"
	DealQualified DealStage = "qualified"
	DealProposal  DealStage = "proposal"
	DealWon       DealStage = "won"
	DealLost      DealStage = "lost"
)

type Deal struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	OrganizationID string    `json:"organization_id,omitempty"`
	PersonID       string    `json:"person_id,omitempty"`
	OwnerID        string    `json:"owner_id"`
	ValueCents     int64     `json:"value_cents"`
	Currency       string    `json:"currency"`
	Stage          DealStage `json:"stage"`
	ExpectedClose  string    `json:"expected_close"`
	NextTaskID     string    `json:"next_task_id,omitempty"`
	PreviousStage  DealStage `json:"previous_stage,omitempty"`
	ClosedOn       string    `json:"closed_on,omitempty"`
	LostReason     string    `json:"lost_reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedBy      string    `json:"created_by"`
	UpdatedBy      string    `json:"updated_by"`
}

type ActivityKind string

const (
	ActivityCall    ActivityKind = "call"
	ActivityEmail   ActivityKind = "email"
	ActivityMeeting ActivityKind = "meeting"
	ActivityNote    ActivityKind = "note"
)

type Activity struct {
	ID             string       `json:"id"`
	Kind           ActivityKind `json:"kind"`
	Summary        string       `json:"summary"`
	OccurredAt     time.Time    `json:"occurred_at"`
	OrganizationID string       `json:"organization_id,omitempty"`
	PersonID       string       `json:"person_id,omitempty"`
	DealID         string       `json:"deal_id,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	CreatedBy      string       `json:"created_by"`
}

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskCompleted TaskStatus = "completed"
	TaskCanceled  TaskStatus = "canceled"
)

type Task struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	DueDate        string     `json:"due_date"`
	OwnerID        string     `json:"owner_id"`
	Status         TaskStatus `json:"status"`
	OrganizationID string     `json:"organization_id,omitempty"`
	PersonID       string     `json:"person_id,omitempty"`
	DealID         string     `json:"deal_id,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CanceledAt     *time.Time `json:"canceled_at,omitempty"`
	CancelReason   string     `json:"cancel_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CreatedBy      string     `json:"created_by"`
	UpdatedBy      string     `json:"updated_by"`
}

type CustomerLink struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organization_id,omitempty"`
	PersonID         string     `json:"person_id,omitempty"`
	MagpieCustomerID string     `json:"magpie_customer_id"`
	LinkedAt         time.Time  `json:"linked_at"`
	LinkedBy         string     `json:"linked_by"`
	RemovedAt        *time.Time `json:"removed_at,omitempty"`
	RemovedBy        string     `json:"removed_by,omitempty"`
	RemoveReason     string     `json:"remove_reason,omitempty"`
}

type MagpieRole struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type MagpieUser struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type MagpieCustomer struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	ExternalRefs []ExternalRef `json:"external_refs,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type MagpieInvoice struct {
	ID            string `json:"id"`
	InvoiceNumber string `json:"invoice_number"`
	CustomerID    string `json:"customer_id"`
	InvoiceDate   string `json:"invoice_date"`
	DueDate       string `json:"due_date,omitempty"`
	Status        string `json:"status"`
	TotalCents    int64  `json:"total_cents"`
}

type ImportReceipt struct {
	Source    string `json:"source"`
	SourceKey string `json:"source_key"`
	Digest    string `json:"digest"`
	Root      string `json:"root"`
}

type State struct {
	Workspace       *Workspace                `json:"workspace,omitempty"`
	Organizations   map[string]Organization   `json:"organizations"`
	People          map[string]Person         `json:"people"`
	Deals           map[string]Deal           `json:"deals"`
	Activities      map[string]Activity       `json:"activities"`
	Tasks           map[string]Task           `json:"tasks"`
	CustomerLinks   map[string]CustomerLink   `json:"customer_links"`
	MagpieRoles     map[string]MagpieRole     `json:"magpie_roles"`
	MagpieUsers     map[string]MagpieUser     `json:"magpie_users"`
	MagpieCustomers map[string]MagpieCustomer `json:"magpie_customers"`
	MagpieInvoices  map[string]MagpieInvoice  `json:"magpie_invoices"`
	Imports         map[string]ImportReceipt  `json:"imports"`
	Root            string                    `json:"root,omitempty"`
}

func emptyState() State {
	return State{
		Organizations:   map[string]Organization{},
		People:          map[string]Person{},
		Deals:           map[string]Deal{},
		Activities:      map[string]Activity{},
		Tasks:           map[string]Task{},
		CustomerLinks:   map[string]CustomerLink{},
		MagpieRoles:     map[string]MagpieRole{},
		MagpieUsers:     map[string]MagpieUser{},
		MagpieCustomers: map[string]MagpieCustomer{},
		MagpieInvoices:  map[string]MagpieInvoice{},
		Imports:         map[string]ImportReceipt{},
	}
}

type PipelineStageSummary struct {
	Stage      DealStage `json:"stage"`
	DealCount  int       `json:"deal_count"`
	ValueCents int64     `json:"value_cents"`
}

type PipelineReport struct {
	Currency string                 `json:"currency"`
	Stages   []PipelineStageSummary `json:"stages"`
}

type TodayReport struct {
	AsOf    string `json:"as_of"`
	Overdue []Task `json:"overdue"`
	Due     []Task `json:"due"`
}

type CustomerView struct {
	Link         CustomerLink    `json:"link"`
	Organization *Organization   `json:"organization,omitempty"`
	Person       *Person         `json:"person,omitempty"`
	Customer     MagpieCustomer  `json:"magpie_customer"`
	Invoices     []MagpieInvoice `json:"magpie_invoices"`
	Deals        []Deal          `json:"deals"`
	Activities   []Activity      `json:"activities"`
	Tasks        []Task          `json:"tasks"`
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
