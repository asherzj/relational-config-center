//go:build integration

package mysql

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// Requests serialize authorization before taking an order's locks. Once a
// transaction grows that order's children, its exact-key locks must still let
// an independent SQL transaction insert into adjacent child index ranges.
func TestIndependentReleaseDetailGrowthDoesNotLockIndexGaps(t *testing.T) {
	ctx, adapter, db, _ := identityGuardDatabase(t)
	for _, initial := range []int{0, 1} {
		t.Run(fmt.Sprint(initial), func(t *testing.T) {
			ids := []string{fmt.Sprintf("detail-%d-a", initial), fmt.Sprintf("detail-%d-b", initial)}
			for _, id := range ids {
				for _, seededID := range []string{id, id + "-probe"} {
					order := domain.ReleaseOrder{ID: seededID, Title: "storage concurrency", ApplicantID: "fixture", State: "DRAFT", Version: "1", Items: []domain.ReleaseItem{}}
					if seededID == id {
						for range initial {
							order.Items = append(order.Items, domain.ReleaseItem{TableName: "fixture", Operation: "ADD"})
						}
					}
					if err := adapter.ExecuteReleaseOrder(ctx, func(s application.ReleaseOrderSession) error { return s.SaveReleaseOrder(ctx, order, true) }); err != nil {
						t.Fatal(err)
					}
				}
			}
			operation, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			type admitted struct {
				id         string
				connection int64
			}
			ready := make(chan admitted, len(ids))
			continueWrites := make(chan struct{})
			written := make(chan struct{}, len(ids))
			allowCommit := make(chan struct{})
			var commitOnce sync.Once
			releaseCommit := func() { commitOnce.Do(func() { close(allowCommit) }) }
			defer releaseCommit()
			results := make(chan error, len(ids))
			for _, id := range ids {
				go func(id string) {
					results <- adapter.ExecuteReleaseOrder(operation, func(s application.ReleaseOrderSession) error {
						order, err := s.GetReleaseOrder(operation, id)
						if err != nil {
							return err
						}
						var connection int64
						if err := s.(*releaseOrderSession).database.Raw("SELECT CONNECTION_ID()").Row().Scan(&connection); err != nil {
							return err
						}
						ready <- admitted{id, connection}
						select {
						case <-continueWrites:
						case <-operation.Done():
							return operation.Err()
						}
						order.Items = append(order.Items, domain.ReleaseItem{TableName: "fixture", Operation: "ADD"})
						if err := s.SaveReleaseOrder(operation, order, false); err != nil {
							return err
						}
						key := sha256.Sum256([]byte(id))
						if err := s.ReplaceReleaseTargets(operation, id, []domain.ActiveTarget{{TableName: "fixture", RecordKey: key[:]}}); err != nil {
							return err
						}
						if err := s.ReplaceReleaseTableReferences(operation, id, []string{"fixture"}); err != nil {
							return err
						}
						written <- struct{}{}
						select {
						case <-allowCommit:
							return nil
						case <-operation.Done():
							return operation.Err()
						}
					})
				}(id)
			}
			var first admitted
			select {
			case first = <-ready:
			case err := <-results:
				t.Fatalf("request failed before order barrier: %v", err)
			case <-operation.Done():
				t.Fatal(operation.Err())
			}
			deadline := time.Now().Add(time.Second)
			for {
				waiting, err := authorizationWaiterCount(operation, db, first.connection)
				if err != nil {
					t.Fatal(err)
				}
				if waiting == 1 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("second order did not wait for first order authorization", waiting)
				}
				time.Sleep(10 * time.Millisecond)
			}
			close(continueWrites)
			select {
			case <-written:
			case err := <-results:
				t.Fatalf("first child write failed: %v", err)
			case <-operation.Done():
				t.Fatal(operation.Err())
			}
			// This dedicated adjacent order traverses the same primary/secondary child
			// indexes while the first transaction is still uncommitted. The probe is
			// deliberately rolled back before either real request can finish.
			probeID := first.id + "-probe"
			probeKey := sha256.Sum256([]byte(probeID))
			probeContext, probeCancel := context.WithTimeout(operation, time.Second)
			defer probeCancel()
			probe, err := db.BeginTx(probeContext, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer probe.Rollback()
			for _, statement := range []struct {
				query string
				args  []any
			}{
				{`INSERT INTO rcc_release_details(order_id,position,table_name,application) VALUES(?,0,'fixture','{}')`, []any{probeID}},
				{`INSERT INTO rcc_release_targets(table_name,record_key,order_id) VALUES('fixture',?,?)`, []any{probeKey[:], probeID}},
				{`INSERT INTO rcc_release_table_references(table_name,order_id) VALUES('fixture',?)`, []any{probeID}},
			} {
				if _, err := probe.ExecContext(probeContext, statement.query, statement.args...); err != nil {
					t.Fatalf("uncommitted order blocked independent child index growth: %v", err)
				}
			}
			select {
			case err := <-results:
				t.Fatalf("owning transaction completed before independent index probe: %v", err)
			default:
			}
			if err := probe.Rollback(); err != nil {
				t.Fatal(err)
			}
			t.Logf("order %s connection %d held uncommitted children; adjacent order probe inserted all three indexes and rolled back", first.id, first.connection)
			releaseCommit()
			for range ids {
				if err := <-results; err != nil {
					t.Error("independent detail growth failed", err)
				}
			}
			for _, id := range ids {
				order, err := adapter.ReadReleaseHeader(ctx, id)
				if err != nil || order.ItemCount != initial+1 || order.ID != id || order.Version != "1" {
					t.Fatalf("saved header: %+v, error: %v", order, err)
				}
				rows, err := db.QueryContext(ctx, `SELECT position,table_name,JSON_UNQUOTE(JSON_EXTRACT(application,'$.table_name')),JSON_UNQUOTE(JSON_EXTRACT(application,'$.operation')) FROM rcc_release_details WHERE order_id=? ORDER BY position`, id)
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				for rows.Next() {
					var position int
					var table, intentTable, operation string
					if err := rows.Scan(&position, &table, &intentTable, &operation); err != nil {
						t.Fatal(err)
					}
					if position != count || table != "fixture" || intentTable != "fixture" || operation != "ADD" {
						t.Fatal("wrong saved detail", position, table, intentTable, operation)
					}
					count++
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				rows.Close()
				if count != initial+1 {
					t.Fatal("incomplete persisted details", count)
				}
				var table, storedKey string
				key := sha256.Sum256([]byte(id))
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*),MIN(table_name),MIN(HEX(record_key)) FROM rcc_release_targets WHERE order_id=?`, id).Scan(&count, &table, &storedKey); err != nil || count != 1 || table != "fixture" || storedKey != fmt.Sprintf("%X", key) {
					t.Fatalf("wrong saved target %s: %d %s %s %v", id, count, table, storedKey, err)
				}
				if err := db.QueryRowContext(ctx, `SELECT COUNT(*),MIN(table_name) FROM rcc_release_table_references WHERE order_id=?`, id).Scan(&count, &table); err != nil || count != 1 || table != "fixture" {
					t.Fatalf("wrong saved reference %s: %d %s %v", id, count, table, err)
				}
			}
			for _, table := range []string{"rcc_release_details", "rcc_release_targets", "rcc_release_table_references"} {
				var count int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE order_id=?", probeID).Scan(&count); err != nil || count != 0 {
					t.Fatalf("probe did not roll back %s: %d %v", table, count, err)
				}
			}
		})
	}
}
