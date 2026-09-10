package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
)

// DraftChanges addresses stable detail identities, independent of pagination
// and position. Unmentioned details retain their saved intent and baselines.
type DraftChanges struct {
	Upserts         []DraftItemInput `json:"upserts"`
	DeleteDetailIDs []string         `json:"delete_detail_ids"`
	DetailOrder     []string         `json:"detail_order"`
}

func newDetailID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", ErrReleaseUnavailable
	}
	return hex.EncodeToString(b), nil
}

func (r *ReleaseOrders) editDraftDetails(ctx context.Context, s ReleaseOrderSession, order ReleaseOrder, changes DraftChanges, defaultTable string) ([]ReleaseItem, error) {
	existing := map[string]ReleaseItem{}
	deleted := map[string]bool{}
	updated := map[string]ReleaseItem{}
	for _, item := range order.Items {
		existing[item.DetailID] = item
	}
	for _, id := range changes.DeleteDetailIDs {
		if _, found := existing[id]; !found || deleted[id] {
			return nil, ErrReleaseInvalid
		}
		deleted[id] = true
	}
	for i := range changes.Upserts {
		if item, found := existing[changes.Upserts[i].DetailID]; found && changes.Upserts[i].TableName == "" {
			changes.Upserts[i].TableName = item.TableName
		}
	}
	seen := map[string]bool{}
	for index, input := range changes.Upserts {
		if input.DetailID == "" {
			continue
		}
		if _, found := existing[input.DetailID]; !found || seen[input.DetailID] || deleted[input.DetailID] {
			return nil, &ReleaseItemError{Index: draftChangeIndex(order, changes, index), Cause: ErrReleaseInvalid}
		}
		if input.Operation == "ADD" && input.Content["id"] != nil && ValidateRecordVersion(input.ExpectedRecordVersion) != nil {
			return nil, &ReleaseItemError{Index: draftChangeIndex(order, changes, index), Cause: ErrRecordVersionRequired}
		}
		seen[input.DetailID] = true
	}
	var prepared []ReleaseItem
	if len(changes.Upserts) > 0 {
		var err error
		prepared, err = r.prepare(ctx, s, DraftInput{TableName: defaultTable, Items: changes.Upserts}, false)
		if err != nil {
			var itemError *ReleaseItemError
			if errors.As(err, &itemError) {
				itemError.Index = draftChangeIndex(order, changes, itemError.Index)
			}
			return nil, err
		}
	}
	for _, item := range prepared {
		updated[item.DetailID] = item
	}
	items := []ReleaseItem{}
	for _, item := range order.Items {
		if deleted[item.DetailID] {
			continue
		}
		if replacement, found := updated[item.DetailID]; found {
			item = replacement
		}
		items = append(items, item)
	}
	for _, item := range prepared {
		if _, found := existing[item.DetailID]; !found {
			items = append(items, item)
		}
	}
	if len(items) > 1000 {
		return nil, ErrReleaseItemLimit
	}
	if changes.DetailOrder != nil {
		if len(changes.DetailOrder) != len(items) {
			return nil, ErrReleaseInvalid
		}
		byID := map[string]ReleaseItem{}
		for _, item := range items {
			byID[item.DetailID] = item
		}
		for index, id := range changes.DetailOrder {
			item, found := byID[id]
			if !found {
				return nil, ErrReleaseInvalid
			}
			items[index] = item
			delete(byID, id)
		}
	}
	identities := map[string]map[string]bool{}
	for index, item := range items {
		if len(item.RecordKey) == 0 {
			continue
		}
		if identities[item.TableName] == nil {
			identities[item.TableName] = map[string]bool{}
		}
		if identities[item.TableName][string(item.RecordKey)] {
			return nil, &ReleaseItemError{Index: index, Cause: ErrReleaseDuplicateTarget}
		}
		identities[item.TableName][string(item.RecordKey)] = true
	}
	return items, nil
}

// Preparation sees only the changed page. Error positions refer to the complete
// candidate order, after deletions and explicit sorting, just like target errors.
func draftChangeIndex(order ReleaseOrder, changes DraftChanges, changed int) int {
	if changed < 0 || changed >= len(changes.Upserts) {
		return changed
	}
	id := changes.Upserts[changed].DetailID
	if id != "" && changes.DetailOrder != nil {
		if index := slices.Index(changes.DetailOrder, id); index >= 0 {
			return index
		}
	}
	index := 0
	for _, item := range order.Items {
		if slices.Contains(changes.DeleteDetailIDs, item.DetailID) {
			continue
		}
		if item.DetailID == id {
			return index
		}
		index++
	}
	for _, item := range changes.Upserts[:changed] {
		if item.DetailID == "" {
			index++
		}
	}
	return index
}
