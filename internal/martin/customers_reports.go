package martin

import (
	"sort"
	"strings"
	"time"
)

func (s *Store) LinkCustomer(ctx Context, organizationID, personID, customerID string) (CustomerLink, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return CustomerLink{}, "", err
	}
	if err := ensureReady(st, ctx, accessManage); err != nil {
		return CustomerLink{}, "", err
	}
	organizationID = strings.TrimSpace(organizationID)
	personID = strings.TrimSpace(personID)
	customerID = strings.TrimSpace(customerID)
	if (organizationID == "") == (personID == "") {
		return CustomerLink{}, "", appErr(ErrValidation, "customer link requires exactly one organization or person")
	}
	if _, ok := st.MagpieCustomers[customerID]; !ok {
		return CustomerLink{}, "", appErr(ErrNotFound, "Magpie customer %s not found", customerID)
	}
	if organizationID != "" {
		entity, ok := st.Organizations[organizationID]
		if !ok || entity.Archived {
			return CustomerLink{}, "", appErr(ErrValidation, "organization %s is not active", organizationID)
		}
	}
	if personID != "" {
		entity, ok := st.People[personID]
		if !ok || entity.Archived {
			return CustomerLink{}, "", appErr(ErrValidation, "person %s is not active", personID)
		}
	}
	for _, link := range st.CustomerLinks {
		if link.RemovedAt != nil {
			continue
		}
		if link.MagpieCustomerID == customerID || (organizationID != "" && link.OrganizationID == organizationID) || (personID != "" && link.PersonID == personID) {
			if link.MagpieCustomerID == customerID && link.OrganizationID == organizationID && link.PersonID == personID {
				return link, st.Root, nil
			}
			return CustomerLink{}, "", appErr(ErrConflict, "customer or CRM entity already has an active link")
		}
	}
	id, err := makeID("customer-link")
	if err != nil {
		return CustomerLink{}, "", err
	}
	link := CustomerLink{
		ID: id, OrganizationID: organizationID, PersonID: personID, MagpieCustomerID: customerID,
		LinkedAt: s.now(), LinkedBy: ctx.Actor,
	}
	root, err := s.appendAt(ctx, TypeCustomerLinkCreated, link.ID, "customer link", entityPayload[CustomerLink]{Entity: link}, st.Root)
	return link, root, err
}

func (s *Store) UnlinkCustomer(ctx Context, customerID, reason string) (CustomerLink, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return CustomerLink{}, "", err
	}
	if err := ensureReady(st, ctx, accessManage); err != nil {
		return CustomerLink{}, "", err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return CustomerLink{}, "", appErr(ErrValidation, "unlink reason is required")
	}
	var found *CustomerLink
	for _, link := range st.CustomerLinks {
		if link.MagpieCustomerID == strings.TrimSpace(customerID) && link.RemovedAt == nil {
			copy := link
			found = &copy
			break
		}
	}
	if found == nil {
		return CustomerLink{}, "", appErr(ErrNotFound, "active link for Magpie customer %s not found", customerID)
	}
	now := s.now()
	found.RemovedAt = &now
	found.RemovedBy = ctx.Actor
	found.RemoveReason = reason
	root, err := s.appendAt(ctx, TypeCustomerLinkRemoved, found.ID, "customer unlink", entityPayload[CustomerLink]{Entity: *found}, st.Root)
	return *found, root, err
}

func (s *Store) Customer(ctx Context, customerID string) (CustomerView, error) {
	st, err := s.LoadState()
	if err != nil {
		return CustomerView{}, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return CustomerView{}, err
	}
	customerID = strings.TrimSpace(customerID)
	customer, ok := st.MagpieCustomers[customerID]
	if !ok {
		return CustomerView{}, appErr(ErrNotFound, "Magpie customer %s not found", customerID)
	}
	var link CustomerLink
	found := false
	for _, candidate := range st.CustomerLinks {
		if candidate.MagpieCustomerID == customerID && candidate.RemovedAt == nil {
			link, found = candidate, true
			break
		}
	}
	if !found {
		return CustomerView{}, appErr(ErrNotFound, "Magpie customer %s is not linked in Martin", customerID)
	}
	view := CustomerView{Link: link, Customer: customer}
	if link.OrganizationID != "" {
		entity := st.Organizations[link.OrganizationID]
		view.Organization = &entity
	}
	if link.PersonID != "" {
		entity := st.People[link.PersonID]
		view.Person = &entity
	}
	for _, invoice := range st.MagpieInvoices {
		if invoice.CustomerID == customerID {
			view.Invoices = append(view.Invoices, invoice)
		}
	}
	for _, deal := range st.Deals {
		if (link.OrganizationID != "" && deal.OrganizationID == link.OrganizationID) || (link.PersonID != "" && deal.PersonID == link.PersonID) {
			view.Deals = append(view.Deals, deal)
		}
	}
	for _, activity := range st.Activities {
		if (link.OrganizationID != "" && activity.OrganizationID == link.OrganizationID) || (link.PersonID != "" && activity.PersonID == link.PersonID) {
			view.Activities = append(view.Activities, activity)
		}
	}
	for _, task := range st.Tasks {
		if (link.OrganizationID != "" && task.OrganizationID == link.OrganizationID) || (link.PersonID != "" && task.PersonID == link.PersonID) {
			view.Tasks = append(view.Tasks, task)
		}
	}
	sort.Slice(view.Invoices, func(i, j int) bool { return view.Invoices[i].InvoiceDate > view.Invoices[j].InvoiceDate })
	sort.Slice(view.Deals, func(i, j int) bool { return view.Deals[i].UpdatedAt.After(view.Deals[j].UpdatedAt) })
	sort.Slice(view.Activities, func(i, j int) bool { return view.Activities[i].OccurredAt.After(view.Activities[j].OccurredAt) })
	sort.Slice(view.Tasks, func(i, j int) bool { return view.Tasks[i].DueDate < view.Tasks[j].DueDate })
	return view, nil
}

func (s *Store) ListCustomerLinks(ctx Context, includeRemoved bool) ([]CustomerLink, error) {
	st, err := s.LoadState()
	if err != nil {
		return nil, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return nil, err
	}
	var links []CustomerLink
	for _, link := range st.CustomerLinks {
		if includeRemoved || link.RemovedAt == nil {
			links = append(links, link)
		}
	}
	sort.Slice(links, func(i, j int) bool { return links[i].MagpieCustomerID < links[j].MagpieCustomerID })
	return links, nil
}

func (s *Store) Pipeline(ctx Context, ownerID string) (PipelineReport, error) {
	st, err := s.LoadState()
	if err != nil {
		return PipelineReport{}, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return PipelineReport{}, err
	}
	stages := []DealStage{DealNew, DealQualified, DealProposal, DealWon, DealLost}
	indexes := make(map[DealStage]int, len(stages))
	report := PipelineReport{Currency: st.Workspace.Currency}
	for _, stage := range stages {
		report.Stages = append(report.Stages, PipelineStageSummary{Stage: stage})
		indexes[stage] = len(report.Stages) - 1
	}
	for _, deal := range st.Deals {
		if ownerID != "" && deal.OwnerID != ownerID {
			continue
		}
		index, ok := indexes[deal.Stage]
		if ok {
			report.Stages[index].DealCount++
			report.Stages[index].ValueCents += deal.ValueCents
		}
	}
	return report, nil
}

func (s *Store) Today(ctx Context, ownerID, asOf string) (TodayReport, error) {
	st, err := s.LoadState()
	if err != nil {
		return TodayReport{}, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return TodayReport{}, err
	}
	if asOf == "" {
		asOf = s.now().Format("2006-01-02")
	}
	if err := validateDate("as-of", asOf); err != nil {
		return TodayReport{}, err
	}
	report := TodayReport{AsOf: asOf}
	for _, task := range st.Tasks {
		if task.Status != TaskPending || (ownerID != "" && task.OwnerID != ownerID) {
			continue
		}
		if task.DueDate < asOf {
			report.Overdue = append(report.Overdue, task)
		} else if task.DueDate == asOf {
			report.Due = append(report.Due, task)
		}
	}
	sort.Slice(report.Overdue, func(i, j int) bool { return report.Overdue[i].DueDate < report.Overdue[j].DueDate })
	sort.Slice(report.Due, func(i, j int) bool { return report.Due[i].ID < report.Due[j].ID })
	return report, nil
}

type SearchResult struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func (s *Store) Search(ctx Context, query string) ([]SearchResult, error) {
	st, err := s.LoadState()
	if err != nil {
		return nil, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, appErr(ErrValidation, "search query is required")
	}
	var out []SearchResult
	for _, entity := range st.Organizations {
		if strings.Contains(strings.ToLower(entity.Name+" "+entity.Domain+" "+entity.Email), query) {
			out = append(out, SearchResult{Type: "organization", ID: entity.ID, DisplayName: entity.Name})
		}
	}
	for _, entity := range st.People {
		if strings.Contains(strings.ToLower(entity.DisplayName+" "+entity.Email+" "+entity.Phone), query) {
			out = append(out, SearchResult{Type: "person", ID: entity.ID, DisplayName: entity.DisplayName})
		}
	}
	for _, entity := range st.Deals {
		if strings.Contains(strings.ToLower(entity.Name), query) {
			out = append(out, SearchResult{Type: "deal", ID: entity.ID, DisplayName: entity.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return strings.ToLower(out[i].DisplayName) < strings.ToLower(out[j].DisplayName)
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

func (s *Store) Export(ctx Context) (State, error) {
	st, err := s.LoadState()
	if err != nil {
		return State{}, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return State{}, err
	}
	return st, nil
}

func dateAtMidnight(raw string) time.Time {
	value, _ := time.Parse("2006-01-02", raw)
	return value.UTC()
}
