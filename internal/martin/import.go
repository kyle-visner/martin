package martin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type ImportBundle struct {
	Source        string         `json:"source"`
	SourceKey     string         `json:"source_key"`
	Organizations []Organization `json:"organizations,omitempty"`
	People        []Person       `json:"people,omitempty"`
	Deals         []Deal         `json:"deals,omitempty"`
	Activities    []Activity     `json:"activities,omitempty"`
	Tasks         []Task         `json:"tasks,omitempty"`
	CustomerLinks []CustomerLink `json:"customer_links,omitempty"`
}

type ImportResult struct {
	Receipt ImportReceipt  `json:"receipt"`
	Counts  map[string]int `json:"counts"`
}

func (s *Store) Import(ctx Context, bundle ImportBundle) (ImportResult, string, error) {
	st, err := s.LoadState()
	if err != nil {
		return ImportResult{}, "", err
	}
	if err := ensureReady(st, ctx, accessManage); err != nil {
		return ImportResult{}, "", err
	}
	bundle.Source = strings.TrimSpace(bundle.Source)
	bundle.SourceKey = strings.TrimSpace(bundle.SourceKey)
	if bundle.Source == "" || bundle.SourceKey == "" {
		return ImportResult{}, "", appErr(ErrValidation, "import requires source and source_key")
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return ImportResult{}, "", err
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	key := importKey(bundle.Source, bundle.SourceKey)
	if existing, ok := st.Imports[key]; ok {
		if existing.Digest != digest {
			return ImportResult{}, "", appErr(ErrConflict, "source key %s was already used with different content", key)
		}
		return importResult(bundle, existing), st.Root, nil
	}
	if len(bundle.Organizations)+len(bundle.People)+len(bundle.Deals)+len(bundle.Activities)+len(bundle.Tasks)+len(bundle.CustomerLinks) == 0 {
		return ImportResult{}, "", appErr(ErrValidation, "import bundle is empty")
	}
	if err := validateImport(st, bundle); err != nil {
		return ImportResult{}, "", err
	}
	receipt := ImportReceipt{Source: bundle.Source, SourceKey: bundle.SourceKey, Digest: digest}
	payload := importPayload{
		Receipt: receipt, Organizations: bundle.Organizations, People: bundle.People, Deals: bundle.Deals,
		Activities: bundle.Activities, Tasks: bundle.Tasks, CustomerLinks: bundle.CustomerLinks,
	}
	root, err := s.appendAt(ctx, TypeImportApplied, "import:"+digest[:24], "import json", payload, st.Root)
	if err != nil {
		return ImportResult{}, "", err
	}
	receipt.Root = root
	return importResult(bundle, receipt), root, nil
}

func validateImport(st State, bundle ImportBundle) error {
	combined := st
	for _, entity := range bundle.Organizations {
		if strings.TrimSpace(entity.ID) == "" || strings.TrimSpace(entity.Name) == "" {
			return appErr(ErrValidation, "imported organizations require id and name")
		}
		if _, exists := combined.Organizations[entity.ID]; exists {
			return appErr(ErrConflict, "imported organization %s already exists", entity.ID)
		}
		combined.Organizations[entity.ID] = entity
	}
	for _, entity := range bundle.People {
		if strings.TrimSpace(entity.ID) == "" || strings.TrimSpace(entity.DisplayName) == "" {
			return appErr(ErrValidation, "imported people require id and display_name")
		}
		if _, exists := combined.People[entity.ID]; exists {
			return appErr(ErrConflict, "imported person %s already exists", entity.ID)
		}
		if entity.OrganizationID != "" {
			if _, ok := combined.Organizations[entity.OrganizationID]; !ok {
				return appErr(ErrValidation, "imported person %s references unknown organization %s", entity.ID, entity.OrganizationID)
			}
		}
		combined.People[entity.ID] = entity
	}
	for _, entity := range bundle.Tasks {
		if entity.ID == "" || entity.Title == "" {
			return appErr(ErrValidation, "imported tasks require id and title")
		}
		if _, exists := combined.Tasks[entity.ID]; exists {
			return appErr(ErrConflict, "imported task %s already exists", entity.ID)
		}
		if err := validateDate("imported task due_date", entity.DueDate); err != nil {
			return err
		}
		combined.Tasks[entity.ID] = entity
	}
	for _, entity := range bundle.Deals {
		if entity.ID == "" || entity.Name == "" {
			return appErr(ErrValidation, "imported deals require id and name")
		}
		if _, exists := combined.Deals[entity.ID]; exists {
			return appErr(ErrConflict, "imported deal %s already exists", entity.ID)
		}
		if isOpenStage(entity.Stage) {
			task, ok := combined.Tasks[entity.NextTaskID]
			if !ok || task.Status != TaskPending || task.DealID != entity.ID {
				return appErr(ErrValidation, "imported open deal %s requires one matching pending next task", entity.ID)
			}
		}
		if err := validateRelations(combined, entity.OrganizationID, entity.PersonID, ""); err != nil {
			return err
		}
		combined.Deals[entity.ID] = entity
	}
	for _, entity := range bundle.Activities {
		if entity.ID == "" || entity.Summary == "" {
			return appErr(ErrValidation, "imported activities require id and summary")
		}
		if _, exists := combined.Activities[entity.ID]; exists {
			return appErr(ErrConflict, "imported activity %s already exists", entity.ID)
		}
		if err := validateRelations(combined, entity.OrganizationID, entity.PersonID, entity.DealID); err != nil {
			return err
		}
	}
	for _, entity := range bundle.CustomerLinks {
		if entity.ID == "" || entity.MagpieCustomerID == "" {
			return appErr(ErrValidation, "imported customer links require id and magpie_customer_id")
		}
		if _, exists := combined.CustomerLinks[entity.ID]; exists {
			return appErr(ErrConflict, "imported customer link %s already exists", entity.ID)
		}
		if _, ok := combined.MagpieCustomers[entity.MagpieCustomerID]; !ok {
			return appErr(ErrValidation, "imported link references unknown Magpie customer %s", entity.MagpieCustomerID)
		}
	}
	return nil
}

func importResult(bundle ImportBundle, receipt ImportReceipt) ImportResult {
	return ImportResult{Receipt: receipt, Counts: map[string]int{
		"organizations": len(bundle.Organizations), "people": len(bundle.People), "deals": len(bundle.Deals),
		"activities": len(bundle.Activities), "tasks": len(bundle.Tasks), "customer_links": len(bundle.CustomerLinks),
	}}
}

func importKey(source, sourceKey string) string {
	return strings.TrimSpace(source) + "\x00" + strings.TrimSpace(sourceKey)
}
