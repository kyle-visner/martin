package martin

import (
	"sort"
	"strings"
)

func (s *Store) CreateOrganization(ctx Context, organization Organization) (Organization, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Organization{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Organization{}, "", err
	}
	if strings.TrimSpace(organization.OwnerID) == "" {
		organization.OwnerID = ctx.Actor
	}
	organization, err = normalizeOrganization(st, organization, "")
	if err != nil {
		return Organization{}, "", err
	}
	if organization.ID == "" {
		organization.ID, err = makeID("org")
		if err != nil {
			return Organization{}, "", err
		}
	}
	if _, exists := st.Organizations[organization.ID]; exists {
		return Organization{}, "", appErr(ErrConflict, "organization %s already exists", organization.ID)
	}
	now := s.now()
	organization.CreatedAt = now
	organization.UpdatedAt = now
	organization.CreatedBy = ctx.Actor
	organization.UpdatedBy = ctx.Actor
	root, err := s.appendAt(ctx, TypeOrganizationCreated, organization.ID, "organization create", entityPayload[Organization]{Entity: organization}, st.Root)
	return organization, root, err
}

func (s *Store) UpdateOrganization(ctx Context, organization Organization) (Organization, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Organization{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Organization{}, "", err
	}
	existing, ok := st.Organizations[strings.TrimSpace(organization.ID)]
	if !ok {
		return Organization{}, "", appErr(ErrNotFound, "organization %s not found", organization.ID)
	}
	if existing.Archived {
		return Organization{}, "", appErr(ErrConflict, "organization %s is archived", organization.ID)
	}
	organization, err = normalizeOrganization(st, organization, existing.ID)
	if err != nil {
		return Organization{}, "", err
	}
	organization.CreatedAt = existing.CreatedAt
	organization.CreatedBy = existing.CreatedBy
	organization.UpdatedAt = s.now()
	organization.UpdatedBy = ctx.Actor
	root, err := s.appendAt(ctx, TypeOrganizationUpdated, organization.ID, "organization update", entityPayload[Organization]{Entity: organization}, st.Root)
	return organization, root, err
}

func (s *Store) GetOrganization(ctx Context, id string) (Organization, error) {
	st, err := s.LoadState()
	if err != nil {
		return Organization{}, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return Organization{}, err
	}
	entity, ok := st.Organizations[strings.TrimSpace(id)]
	if !ok {
		return Organization{}, appErr(ErrNotFound, "organization %s not found", id)
	}
	return entity, nil
}

func (s *Store) ListOrganizations(ctx Context, includeArchived bool) ([]Organization, error) {
	st, err := s.LoadState()
	if err != nil {
		return nil, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return nil, err
	}
	var out []Organization
	for _, entity := range st.Organizations {
		if includeArchived || !entity.Archived {
			out = append(out, entity)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Store) ArchiveOrganization(ctx Context, id, reason string) (Organization, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Organization{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Organization{}, "", err
	}
	entity, ok := st.Organizations[strings.TrimSpace(id)]
	if !ok {
		return Organization{}, "", appErr(ErrNotFound, "organization %s not found", id)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Organization{}, "", appErr(ErrValidation, "archive reason is required")
	}
	for _, deal := range st.Deals {
		if deal.OrganizationID == entity.ID && isOpenStage(deal.Stage) {
			return Organization{}, "", appErr(ErrConflict, "organization %s has open deal %s", entity.ID, deal.ID)
		}
	}
	if entity.Archived {
		return entity, st.Root, nil
	}
	entity.Archived = true
	entity.ArchiveReason = reason
	entity.UpdatedAt = s.now()
	entity.UpdatedBy = ctx.Actor
	root, err := s.appendAt(ctx, TypeOrganizationArchived, entity.ID, "organization archive", entityPayload[Organization]{Entity: entity}, st.Root)
	return entity, root, err
}

func (s *Store) MergeOrganizations(ctx Context, fromID, intoID, reason string) (string, error) {
	st, err := s.LoadState()
	if err != nil {
		return "", err
	}
	if err := ensureReady(st, ctx, accessManage); err != nil {
		return "", err
	}
	fromID, intoID, reason = strings.TrimSpace(fromID), strings.TrimSpace(intoID), strings.TrimSpace(reason)
	if fromID == intoID || fromID == "" || intoID == "" {
		return "", appErr(ErrValidation, "merge requires different source and destination organization ids")
	}
	from, fromOK := st.Organizations[fromID]
	into, intoOK := st.Organizations[intoID]
	if !fromOK || !intoOK {
		return "", appErr(ErrNotFound, "both merge organizations must exist")
	}
	if from.Archived || into.Archived {
		return "", appErr(ErrConflict, "both merge organizations must be active")
	}
	if reason == "" {
		return "", appErr(ErrValidation, "merge reason is required")
	}
	return s.appendAt(ctx, TypeOrganizationMerged, fromID, "organization merge", mergePayload{FromID: fromID, IntoID: intoID, Reason: reason}, st.Root)
}

func (s *Store) CreatePerson(ctx Context, person Person) (Person, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Person{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Person{}, "", err
	}
	if strings.TrimSpace(person.OwnerID) == "" {
		person.OwnerID = ctx.Actor
	}
	person, err = normalizePerson(st, person, "")
	if err != nil {
		return Person{}, "", err
	}
	if person.ID == "" {
		person.ID, err = makeID("person")
		if err != nil {
			return Person{}, "", err
		}
	}
	if _, exists := st.People[person.ID]; exists {
		return Person{}, "", appErr(ErrConflict, "person %s already exists", person.ID)
	}
	now := s.now()
	person.CreatedAt = now
	person.UpdatedAt = now
	person.CreatedBy = ctx.Actor
	person.UpdatedBy = ctx.Actor
	root, err := s.appendAt(ctx, TypePersonCreated, person.ID, "person create", entityPayload[Person]{Entity: person}, st.Root)
	return person, root, err
}

func (s *Store) UpdatePerson(ctx Context, person Person) (Person, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Person{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Person{}, "", err
	}
	existing, ok := st.People[strings.TrimSpace(person.ID)]
	if !ok {
		return Person{}, "", appErr(ErrNotFound, "person %s not found", person.ID)
	}
	if existing.Archived {
		return Person{}, "", appErr(ErrConflict, "person %s is archived", person.ID)
	}
	person, err = normalizePerson(st, person, existing.ID)
	if err != nil {
		return Person{}, "", err
	}
	person.CreatedAt = existing.CreatedAt
	person.CreatedBy = existing.CreatedBy
	person.UpdatedAt = s.now()
	person.UpdatedBy = ctx.Actor
	root, err := s.appendAt(ctx, TypePersonUpdated, person.ID, "person update", entityPayload[Person]{Entity: person}, st.Root)
	return person, root, err
}

func (s *Store) GetPerson(ctx Context, id string) (Person, error) {
	st, err := s.LoadState()
	if err != nil {
		return Person{}, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return Person{}, err
	}
	entity, ok := st.People[strings.TrimSpace(id)]
	if !ok {
		return Person{}, appErr(ErrNotFound, "person %s not found", id)
	}
	return entity, nil
}

func (s *Store) ListPeople(ctx Context, includeArchived bool) ([]Person, error) {
	st, err := s.LoadState()
	if err != nil {
		return nil, err
	}
	if err := ensureReady(st, ctx, accessRead); err != nil {
		return nil, err
	}
	var out []Person
	for _, entity := range st.People {
		if includeArchived || !entity.Archived {
			out = append(out, entity)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayName == out[j].DisplayName {
			return out[i].ID < out[j].ID
		}
		return strings.ToLower(out[i].DisplayName) < strings.ToLower(out[j].DisplayName)
	})
	return out, nil
}

func (s *Store) ArchivePerson(ctx Context, id, reason string) (Person, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return Person{}, "", err
	}
	if err := ensureReady(st, ctx, accessWrite); err != nil {
		return Person{}, "", err
	}
	entity, ok := st.People[strings.TrimSpace(id)]
	if !ok {
		return Person{}, "", appErr(ErrNotFound, "person %s not found", id)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Person{}, "", appErr(ErrValidation, "archive reason is required")
	}
	for _, deal := range st.Deals {
		if deal.PersonID == entity.ID && isOpenStage(deal.Stage) {
			return Person{}, "", appErr(ErrConflict, "person %s has open deal %s", entity.ID, deal.ID)
		}
	}
	if entity.Archived {
		return entity, st.Root, nil
	}
	entity.Archived = true
	entity.ArchiveReason = reason
	entity.UpdatedAt = s.now()
	entity.UpdatedBy = ctx.Actor
	root, err := s.appendAt(ctx, TypePersonArchived, entity.ID, "person archive", entityPayload[Person]{Entity: entity}, st.Root)
	return entity, root, err
}

func (s *Store) MergePeople(ctx Context, fromID, intoID, reason string) (string, error) {
	st, err := s.LoadState()
	if err != nil {
		return "", err
	}
	if err := ensureReady(st, ctx, accessManage); err != nil {
		return "", err
	}
	fromID, intoID, reason = strings.TrimSpace(fromID), strings.TrimSpace(intoID), strings.TrimSpace(reason)
	if fromID == intoID || fromID == "" || intoID == "" {
		return "", appErr(ErrValidation, "merge requires different source and destination person ids")
	}
	from, fromOK := st.People[fromID]
	into, intoOK := st.People[intoID]
	if !fromOK || !intoOK {
		return "", appErr(ErrNotFound, "both merge people must exist")
	}
	if from.Archived || into.Archived {
		return "", appErr(ErrConflict, "both merge people must be active")
	}
	if reason == "" {
		return "", appErr(ErrValidation, "merge reason is required")
	}
	return s.appendAt(ctx, TypePersonMerged, fromID, "person merge", mergePayload{FromID: fromID, IntoID: intoID, Reason: reason}, st.Root)
}

func normalizeOrganization(st State, entity Organization, currentID string) (Organization, error) {
	entity.ID = strings.TrimSpace(entity.ID)
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Domain = normalizeDomain(entity.Domain)
	entity.Email = strings.ToLower(strings.TrimSpace(entity.Email))
	entity.Phone = strings.TrimSpace(entity.Phone)
	entity.OwnerID = strings.TrimSpace(entity.OwnerID)
	entity.Tags = normalizeTags(entity.Tags)
	if entity.Name == "" {
		return Organization{}, appErr(ErrValidation, "organization name is required")
	}
	if entity.OwnerID == "" {
		return Organization{}, appErr(ErrValidation, "organization owner is required")
	}
	if entity.Domain != "" {
		for id, existing := range st.Organizations {
			if id != currentID && !existing.Archived && normalizeDomain(existing.Domain) == entity.Domain {
				return Organization{}, appErr(ErrConflict, "domain %s already belongs to organization %s", entity.Domain, id)
			}
		}
	}
	return entity, nil
}

func normalizePerson(st State, entity Person, currentID string) (Person, error) {
	entity.ID = strings.TrimSpace(entity.ID)
	entity.DisplayName = strings.TrimSpace(entity.DisplayName)
	entity.OrganizationID = strings.TrimSpace(entity.OrganizationID)
	entity.Title = strings.TrimSpace(entity.Title)
	entity.Email = strings.ToLower(strings.TrimSpace(entity.Email))
	entity.Phone = strings.TrimSpace(entity.Phone)
	entity.OwnerID = strings.TrimSpace(entity.OwnerID)
	entity.Tags = normalizeTags(entity.Tags)
	if entity.DisplayName == "" {
		return Person{}, appErr(ErrValidation, "person display name is required")
	}
	if entity.Email == "" && entity.Phone == "" && entity.OrganizationID == "" {
		return Person{}, appErr(ErrValidation, "person requires an email, phone, or organization")
	}
	if entity.OrganizationID != "" {
		organization, ok := st.Organizations[entity.OrganizationID]
		if !ok || organization.Archived {
			return Person{}, appErr(ErrValidation, "organization %s is not active", entity.OrganizationID)
		}
	}
	if entity.OwnerID == "" {
		return Person{}, appErr(ErrValidation, "person owner is required")
	}
	if entity.Email != "" {
		for id, existing := range st.People {
			if id != currentID && !existing.Archived && strings.EqualFold(existing.Email, entity.Email) {
				return Person{}, appErr(ErrConflict, "email %s already belongs to person %s", entity.Email, id)
			}
		}
	}
	return entity, nil
}

func normalizeDomain(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "www.")
	raw = strings.Trim(raw, "/.")
	return raw
}
