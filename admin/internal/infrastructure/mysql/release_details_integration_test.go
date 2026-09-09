//go:build integration

package mysql

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// The adapter owns the lock protocol. Independent order owners must be able to
// read and grow their details together, including the empty storage state.
func TestIndependentReleaseDetailGrowthDoesNotLockIndexGaps(t *testing.T) {
	ctx, adapter, _, _ := identityGuardDatabase(t)
	for _, initial := range []int{0, 1} {
		t.Run(fmt.Sprint(initial), func(t *testing.T) {
			ids := []string{fmt.Sprintf("detail-%d-a", initial), fmt.Sprintf("detail-%d-b", initial)}
			for _, id := range ids {
				order := domain.ReleaseOrder{ID: id, Title: "storage concurrency", ApplicantID: "fixture", TableName: "fixture", State: "DRAFT", Version: "1", Items: make([]domain.ReleaseItem, initial)}
				if err := adapter.ExecuteReleaseOrder(ctx, func(s application.ReleaseOrderSession) error { return s.SaveReleaseOrder(ctx, order, true) }); err != nil {
					t.Fatal(err)
				}
			}
			operation, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			ready := make(chan struct{}, len(ids))
			continueWrites := make(chan struct{})
			results := make(chan error, len(ids))
			for _, id := range ids {
				go func(id string) {
					results <- adapter.ExecuteReleaseOrder(operation, func(s application.ReleaseOrderSession) error {
						order, err := s.GetReleaseOrder(operation, id)
						ready <- struct{}{}
						if err != nil {
							return err
						}
						select {
						case <-continueWrites:
						case <-operation.Done():
							return operation.Err()
						}
						order.Items = append(order.Items, domain.ReleaseItem{Operation: "ADD"})
						return s.SaveReleaseOrder(operation, order, false)
					})
				}(id)
			}
			for range ids {
				select {
				case <-ready:
				case <-operation.Done():
				}
			}
			close(continueWrites)
			for range ids {
				if err := <-results; err != nil {
					t.Error("independent detail growth failed", err)
				}
			}
			for _, id := range ids {
				order, err := adapter.GetReleaseOrder(ctx, id)
				if err != nil || len(order.Items) != initial+1 {
					t.Errorf("saved detail count: %d, error: %v", len(order.Items), err)
				}
			}
		})
	}
}
