package martin

import (
	"sort"
	"strings"
	"time"
)

func (s *Store) LogActivity(ctx Context, activity Activity) (Activity, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Activity{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Activity{}, "", err
	}
	activity, err = s.normalizeActivity(st, ctx, activity)
	if err != nil {
		return Activity{}, "", err
	}
	root, err := s.appendAt(ctx, TypeActivityLogged, activity.ID, "activity log", entityPayload[Activity]{Entity: activity}, st.Root)
	return activity, root, err
}

func (s *Store) normalizeActivity(st State, ctx Context, activity Activity) (Activity, error) {
	var err error
	if activity.ID == "" {
		activity.ID, err = makeID("activity")
		if err != nil {
			return Activity{}, err
		}
	}
	activity.Summary = strings.TrimSpace(activity.Summary)
	activity.OrganizationID = strings.TrimSpace(activity.OrganizationID)
	activity.PersonID = strings.TrimSpace(activity.PersonID)
	activity.DealID = strings.TrimSpace(activity.DealID)
	if activity.Summary == "" {
		return Activity{}, appErr(ErrValidation, "activity summary is required")
	}
	switch activity.Kind {
	case ActivityCall, ActivityEmail, ActivityMeeting, ActivityNote:
	default:
		return Activity{}, appErr(ErrValidation, "activity kind must be call, email, meeting, or note")
	}
	if activity.OrganizationID == "" && activity.PersonID == "" && activity.DealID == "" {
		return Activity{}, appErr(ErrValidation, "activity must relate to an organization, person, or deal")
	}
	if err := validateRelations(st, activity.OrganizationID, activity.PersonID, activity.DealID); err != nil {
		return Activity{}, err
	}
	if activity.OccurredAt.IsZero() {
		activity.OccurredAt = s.now()
	} else {
		activity.OccurredAt = activity.OccurredAt.UTC()
	}
	activity.CreatedAt = s.now()
	activity.CreatedBy = ctx.Actor
	return activity, nil
}

func (s *Store) ListActivities(ctx Context, organizationID, personID, dealID string) ([]Activity, error) {
	st, err := s.LoadState()
	if err != nil {
		return nil, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return nil, err
	}
	var out []Activity
	for _, activity := range st.Activities {
		if organizationID != "" && activity.OrganizationID != organizationID {
			continue
		}
		if personID != "" && activity.PersonID != personID {
			continue
		}
		if dealID != "" && activity.DealID != dealID {
			continue
		}
		out = append(out, activity)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OccurredAt.Equal(out[j].OccurredAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].OccurredAt.After(out[j].OccurredAt)
	})
	return out, nil
}

func (s *Store) CreateTask(ctx Context, task Task) (Task, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Task{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Task{}, "", err
	}
	if task.DealID != "" {
		return Task{}, "", appErr(ErrValidation, "deal tasks are managed through deal create, advance, and touch")
	}
	task, err = s.normalizeTask(st, ctx, task)
	if err != nil {
		return Task{}, "", err
	}
	root, err := s.appendAt(ctx, TypeTaskCreated, task.ID, "task create", entityPayload[Task]{Entity: task}, st.Root)
	return task, root, err
}

func (s *Store) normalizeTask(st State, ctx Context, task Task) (Task, error) {
	var err error
	if task.ID == "" {
		task.ID, err = makeID("task")
		if err != nil {
			return Task{}, err
		}
	}
	task.Title = strings.TrimSpace(task.Title)
	task.OwnerID = strings.TrimSpace(task.OwnerID)
	task.OrganizationID = strings.TrimSpace(task.OrganizationID)
	task.PersonID = strings.TrimSpace(task.PersonID)
	task.DealID = strings.TrimSpace(task.DealID)
	if task.Title == "" {
		return Task{}, appErr(ErrValidation, "task title is required")
	}
	if err := validateDate("due", task.DueDate); err != nil {
		return Task{}, err
	}
	if task.OrganizationID == "" && task.PersonID == "" && task.DealID == "" {
		return Task{}, appErr(ErrValidation, "task must relate to an organization, person, or deal")
	}
	if err := validateRelations(st, task.OrganizationID, task.PersonID, task.DealID); err != nil {
		return Task{}, err
	}
	if task.OwnerID == "" {
		task.OwnerID = ctx.Actor
	}
	now := s.now()
	task.Status = TaskPending
	task.CreatedAt, task.UpdatedAt = now, now
	task.CreatedBy, task.UpdatedBy = ctx.Actor, ctx.Actor
	return task, nil
}

func (s *Store) CompleteTask(ctx Context, id string) (Task, string, error) {
	return s.finishTask(ctx, id, "", true)
}

func (s *Store) CancelTask(ctx Context, id, reason string) (Task, string, error) {
	return s.finishTask(ctx, id, reason, false)
}

func (s *Store) finishTask(ctx Context, id, reason string, complete bool) (Task, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Task{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Task{}, "", err
	}
	task, ok := st.Tasks[strings.TrimSpace(id)]
	if !ok {
		return Task{}, "", appErr(ErrNotFound, "task %s not found", id)
	}
	if task.DealID != "" {
		return Task{}, "", appErr(ErrConflict, "deal next actions must be completed through deal touch, win, or lose")
	}
	if task.Status != TaskPending {
		return task, st.Root, nil
	}
	now := s.now()
	eventType, command := TypeTaskCompleted, "task complete"
	if complete {
		task.Status = TaskCompleted
		task.CompletedAt = &now
	} else {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return Task{}, "", appErr(ErrValidation, "cancel reason is required")
		}
		task.Status = TaskCanceled
		task.CanceledAt = &now
		task.CancelReason = reason
		eventType, command = TypeTaskCanceled, "task cancel"
	}
	task.UpdatedAt = now
	task.UpdatedBy = ctx.Actor
	root, err := s.appendAt(ctx, eventType, task.ID, command, entityPayload[Task]{Entity: task}, st.Root)
	return task, root, err
}

func (s *Store) ListTasks(ctx Context, ownerID string, status TaskStatus) ([]Task, error) {
	st, err := s.LoadState()
	if err != nil {
		return nil, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return nil, err
	}
	switch status {
	case "", TaskPending, TaskCompleted, TaskCanceled:
	default:
		return nil, appErr(ErrValidation, "task status must be pending, completed, or canceled")
	}
	var out []Task
	for _, task := range st.Tasks {
		if ownerID != "" && task.OwnerID != ownerID {
			continue
		}
		if status != "" && task.Status != status {
			continue
		}
		out = append(out, task)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DueDate == out[j].DueDate {
			return out[i].ID < out[j].ID
		}
		return out[i].DueDate < out[j].DueDate
	})
	return out, nil
}

func validateRelations(st State, organizationID, personID, dealID string) error {
	if organizationID != "" {
		entity, ok := st.Organizations[organizationID]
		if !ok || entity.Archived {
			return appErr(ErrValidation, "organization %s is not active", organizationID)
		}
	}
	if personID != "" {
		entity, ok := st.People[personID]
		if !ok || entity.Archived {
			return appErr(ErrValidation, "person %s is not active", personID)
		}
	}
	if dealID != "" {
		if _, ok := st.Deals[dealID]; !ok {
			return appErr(ErrValidation, "deal %s does not exist", dealID)
		}
	}
	return nil
}

func parseOccurredAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, appErr(ErrValidation, "occurred-at must use RFC3339")
	}
	return value.UTC(), nil
}
