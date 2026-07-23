package martin

import (
	"sort"
	"strings"
	"time"
)

func (s *Store) CreateDeal(ctx Context, deal Deal, nextAction, nextDue string) (Deal, Task, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Deal{}, Task{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Deal{}, Task{}, "", err
	}
	deal, err = s.normalizeNewDeal(st, ctx, deal)
	if err != nil {
		return Deal{}, Task{}, "", err
	}
	task, err := s.newDealTask(ctx, deal, nextAction, nextDue)
	if err != nil {
		return Deal{}, Task{}, "", err
	}
	deal.NextTaskID = task.ID
	payload := dealWorkflowPayload{Deal: deal, NextTask: &task}
	root, err := s.appendAt(ctx, TypeDealCreated, deal.ID, "deal create", payload, st.Root)
	return deal, task, root, err
}

func (s *Store) AdvanceDeal(ctx Context, dealID, nextAction, nextDue string) (Deal, Task, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Deal{}, Task{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Deal{}, Task{}, "", err
	}
	deal, ok := st.Deals[strings.TrimSpace(dealID)]
	if !ok {
		return Deal{}, Task{}, "", appErr(ErrNotFound, "deal %s not found", dealID)
	}
	nextStage := DealStage("")
	switch deal.Stage {
	case DealNew:
		nextStage = DealQualified
	case DealQualified:
		nextStage = DealProposal
	case DealProposal:
		return Deal{}, Task{}, "", appErr(ErrConflict, "proposal-stage deals must be won or lost")
	default:
		return Deal{}, Task{}, "", appErr(ErrConflict, "closed deal %s cannot advance", deal.ID)
	}
	completed, err := completeCurrentDealTask(st, deal, ctx, s.now())
	if err != nil {
		return Deal{}, Task{}, "", err
	}
	deal.PreviousStage = deal.Stage
	deal.Stage = nextStage
	deal.UpdatedAt = s.now()
	deal.UpdatedBy = ctx.Actor
	next, err := s.newDealTask(ctx, deal, nextAction, nextDue)
	if err != nil {
		return Deal{}, Task{}, "", err
	}
	deal.NextTaskID = next.ID
	root, err := s.appendAt(ctx, TypeDealAdvanced, deal.ID, "deal advance", dealWorkflowPayload{
		Deal: deal, CompletedTask: &completed, NextTask: &next,
	}, st.Root)
	return deal, next, root, err
}

func (s *Store) TouchDeal(ctx Context, dealID string, kind ActivityKind, summary string, occurredAt time.Time, nextAction, nextDue string) (Deal, Activity, Task, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Deal{}, Activity{}, Task{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Deal{}, Activity{}, Task{}, "", err
	}
	deal, ok := st.Deals[strings.TrimSpace(dealID)]
	if !ok {
		return Deal{}, Activity{}, Task{}, "", appErr(ErrNotFound, "deal %s not found", dealID)
	}
	if !isOpenStage(deal.Stage) {
		return Deal{}, Activity{}, Task{}, "", appErr(ErrConflict, "closed deal %s cannot be touched", deal.ID)
	}
	activity, err := s.normalizeActivity(st, ctx, Activity{
		Kind: kind, Summary: summary, OccurredAt: occurredAt,
		OrganizationID: deal.OrganizationID, PersonID: deal.PersonID, DealID: deal.ID,
	})
	if err != nil {
		return Deal{}, Activity{}, Task{}, "", err
	}
	completed, err := completeCurrentDealTask(st, deal, ctx, s.now())
	if err != nil {
		return Deal{}, Activity{}, Task{}, "", err
	}
	next, err := s.newDealTask(ctx, deal, nextAction, nextDue)
	if err != nil {
		return Deal{}, Activity{}, Task{}, "", err
	}
	deal.NextTaskID = next.ID
	deal.UpdatedAt = s.now()
	deal.UpdatedBy = ctx.Actor
	root, err := s.appendAt(ctx, TypeDealTouched, deal.ID, "deal touch", dealWorkflowPayload{
		Deal: deal, CompletedTask: &completed, NextTask: &next, Activity: &activity,
	}, st.Root)
	return deal, activity, next, root, err
}

func (s *Store) WinDeal(ctx Context, dealID, closedOn string) (Deal, string, error) {
	return s.closeDeal(ctx, dealID, DealWon, closedOn, "")
}

func (s *Store) LoseDeal(ctx Context, dealID, closedOn, reason string) (Deal, string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Deal{}, "", appErr(ErrValidation, "lost reason is required")
	}
	return s.closeDeal(ctx, dealID, DealLost, closedOn, reason)
}

func (s *Store) closeDeal(ctx Context, dealID string, stage DealStage, closedOn, reason string) (Deal, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Deal{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Deal{}, "", err
	}
	deal, ok := st.Deals[strings.TrimSpace(dealID)]
	if !ok {
		return Deal{}, "", appErr(ErrNotFound, "deal %s not found", dealID)
	}
	if !isOpenStage(deal.Stage) {
		if deal.Stage == stage && deal.ClosedOn == closedOn && deal.LostReason == reason {
			return deal, st.Root, nil
		}
		return Deal{}, "", appErr(ErrConflict, "deal %s is already closed", deal.ID)
	}
	if err := validateDate("closed-on", closedOn); err != nil {
		return Deal{}, "", err
	}
	canceled, err := cancelCurrentDealTask(st, deal, ctx, s.now(), "deal "+string(stage))
	if err != nil {
		return Deal{}, "", err
	}
	deal.PreviousStage = deal.Stage
	deal.Stage = stage
	deal.ClosedOn = closedOn
	deal.LostReason = reason
	deal.NextTaskID = ""
	deal.UpdatedAt = s.now()
	deal.UpdatedBy = ctx.Actor
	eventType, command := TypeDealWon, "deal win"
	if stage == DealLost {
		eventType, command = TypeDealLost, "deal lose"
	}
	root, err := s.appendAt(ctx, eventType, deal.ID, command, dealWorkflowPayload{
		Deal: deal, CompletedTask: &canceled,
	}, st.Root)
	return deal, root, err
}

func (s *Store) ReopenDeal(ctx Context, dealID, reason, nextAction, nextDue string) (Deal, Task, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Deal{}, Task{}, "", err
	}
	if err := ensureReady(st, ctx, accessManage); err != nil {
		return Deal{}, Task{}, "", err
	}
	deal, ok := st.Deals[strings.TrimSpace(dealID)]
	if !ok {
		return Deal{}, Task{}, "", appErr(ErrNotFound, "deal %s not found", dealID)
	}
	if isOpenStage(deal.Stage) {
		return Deal{}, Task{}, "", appErr(ErrConflict, "deal %s is already open", deal.ID)
	}
	if strings.TrimSpace(reason) == "" {
		return Deal{}, Task{}, "", appErr(ErrValidation, "reopen reason is required")
	}
	stage := deal.PreviousStage
	if !isOpenStage(stage) {
		stage = DealQualified
	}
	deal.PreviousStage = deal.Stage
	deal.Stage = stage
	deal.ClosedOn = ""
	deal.LostReason = ""
	deal.UpdatedAt = s.now()
	deal.UpdatedBy = ctx.Actor
	task, err := s.newDealTask(ctx, deal, nextAction, nextDue)
	if err != nil {
		return Deal{}, Task{}, "", err
	}
	deal.NextTaskID = task.ID
	root, err := s.appendAt(ctx, TypeDealReopened, deal.ID, "deal reopen: "+strings.TrimSpace(reason), dealWorkflowPayload{
		Deal: deal, NextTask: &task,
	}, st.Root)
	return deal, task, root, err
}

func (s *Store) GetDeal(ctx Context, id string) (Deal, error) {
	st, err := s.LoadState()
	if err != nil {
		return Deal{}, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return Deal{}, err
	}
	deal, ok := st.Deals[strings.TrimSpace(id)]
	if !ok {
		return Deal{}, appErr(ErrNotFound, "deal %s not found", id)
	}
	return deal, nil
}

func (s *Store) ListDeals(ctx Context, includeClosed bool) ([]Deal, error) {
	st, err := s.LoadState()
	if err != nil {
		return nil, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return nil, err
	}
	var deals []Deal
	for _, deal := range st.Deals {
		if includeClosed || isOpenStage(deal.Stage) {
			deals = append(deals, deal)
		}
	}
	sort.Slice(deals, func(i, j int) bool {
		if deals[i].ExpectedClose == deals[j].ExpectedClose {
			return deals[i].ID < deals[j].ID
		}
		return deals[i].ExpectedClose < deals[j].ExpectedClose
	})
	return deals, nil
}

func (s *Store) normalizeNewDeal(st State, ctx Context, deal Deal) (Deal, error) {
	var err error
	deal.ID = strings.TrimSpace(deal.ID)
	if deal.ID == "" {
		deal.ID, err = makeID("deal")
		if err != nil {
			return Deal{}, err
		}
	}
	if _, exists := st.Deals[deal.ID]; exists {
		return Deal{}, appErr(ErrConflict, "deal %s already exists", deal.ID)
	}
	deal.Name = strings.TrimSpace(deal.Name)
	deal.OrganizationID = strings.TrimSpace(deal.OrganizationID)
	deal.PersonID = strings.TrimSpace(deal.PersonID)
	deal.OwnerID = strings.TrimSpace(deal.OwnerID)
	if deal.Name == "" {
		return Deal{}, appErr(ErrValidation, "deal name is required")
	}
	if deal.OrganizationID == "" && deal.PersonID == "" {
		return Deal{}, appErr(ErrValidation, "deal requires an organization or person")
	}
	if deal.OrganizationID != "" {
		entity, ok := st.Organizations[deal.OrganizationID]
		if !ok || entity.Archived {
			return Deal{}, appErr(ErrValidation, "organization %s is not active", deal.OrganizationID)
		}
	}
	if deal.PersonID != "" {
		entity, ok := st.People[deal.PersonID]
		if !ok || entity.Archived {
			return Deal{}, appErr(ErrValidation, "person %s is not active", deal.PersonID)
		}
		if deal.OrganizationID != "" && entity.OrganizationID != "" && entity.OrganizationID != deal.OrganizationID {
			return Deal{}, appErr(ErrValidation, "person %s belongs to a different organization", deal.PersonID)
		}
	}
	if deal.ValueCents < 0 {
		return Deal{}, appErr(ErrValidation, "deal value must not be negative")
	}
	if err := validateDate("expected-close", deal.ExpectedClose); err != nil {
		return Deal{}, err
	}
	if deal.OwnerID == "" {
		deal.OwnerID = ctx.Actor
	}
	deal.Currency = st.Workspace.Currency
	deal.Stage = DealNew
	now := s.now()
	deal.CreatedAt, deal.UpdatedAt = now, now
	deal.CreatedBy, deal.UpdatedBy = ctx.Actor, ctx.Actor
	return deal, nil
}

func (s *Store) newDealTask(ctx Context, deal Deal, title, due string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, appErr(ErrValidation, "next action is required")
	}
	if err := validateDate("next-due", due); err != nil {
		return Task{}, err
	}
	id, err := makeID("task")
	if err != nil {
		return Task{}, err
	}
	now := s.now()
	return Task{
		ID: id, Title: title, DueDate: due, OwnerID: deal.OwnerID, Status: TaskPending,
		OrganizationID: deal.OrganizationID, PersonID: deal.PersonID, DealID: deal.ID,
		CreatedAt: now, UpdatedAt: now, CreatedBy: ctx.Actor, UpdatedBy: ctx.Actor,
	}, nil
}

func completeCurrentDealTask(st State, deal Deal, ctx Context, at time.Time) (Task, error) {
	task, ok := st.Tasks[deal.NextTaskID]
	if !ok || task.Status != TaskPending {
		return Task{}, appErr(ErrIntegrity, "open deal %s does not have one pending next action", deal.ID)
	}
	task.Status = TaskCompleted
	task.CompletedAt = &at
	task.UpdatedAt = at
	task.UpdatedBy = ctx.Actor
	return task, nil
}

func cancelCurrentDealTask(st State, deal Deal, ctx Context, at time.Time, reason string) (Task, error) {
	task, ok := st.Tasks[deal.NextTaskID]
	if !ok || task.Status != TaskPending {
		return Task{}, appErr(ErrIntegrity, "open deal %s does not have one pending next action", deal.ID)
	}
	task.Status = TaskCanceled
	task.CanceledAt = &at
	task.CancelReason = reason
	task.UpdatedAt = at
	task.UpdatedBy = ctx.Actor
	return task, nil
}

func isOpenStage(stage DealStage) bool {
	return stage == DealNew || stage == DealQualified || stage == DealProposal
}

func validateDate(field, value string) error {
	if _, err := time.Parse("2006-01-02", strings.TrimSpace(value)); err != nil {
		return appErr(ErrValidation, "%s must use YYYY-MM-DD", field)
	}
	return nil
}
