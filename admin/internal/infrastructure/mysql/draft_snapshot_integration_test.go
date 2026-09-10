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

// These two real transaction barriers exercise the same policy guard used by
// prepareInput and publication/rollback. Acquiring a target after a rollback
// releases it must never retain an earlier RR snapshot of the old business key.
func TestDraftBaselineAndPublicationSerializeBeforeSnapshot(t *testing.T) {
	ctx, adapter, db, _ := identityGuardDatabase(t)
	for _, statement := range []string{
		`CREATE TABLE guard_snapshot_a(id INT PRIMARY KEY,code INT) ENGINE=InnoDB`,
		`CREATE TABLE guard_snapshot_b(id INT PRIMARY KEY,code INT) ENGINE=InnoDB`,
		`INSERT INTO guard_snapshot_a VALUES(1,20)`,
		`INSERT INTO guard_snapshot_b VALUES(1,20)`,
		`INSERT INTO rcc_query_policies(code,name,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,status,creator,modifier) VALUES('snapshot_query_v1','snapshot','page_query','id','ASC',5,20,'ACTIVE','test','test')`,
		`INSERT INTO rcc_mutation_policies(code,name,type_code,allow_add,allow_modify,allow_delete,status,creator,modifier) VALUES('snapshot_mutation_v1','snapshot','single_table_mutation',1,1,1,'ACTIVE','test','test')`,
		`INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES('guard_snapshot_a','snapshot_query_v1','snapshot_mutation_v1',1,'test','test'),('guard_snapshot_b','snapshot_query_v1','snapshot_mutation_v1',1,'test','test')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := adapter.GetTableSchema(ctx, "guard_snapshot_a")
	if err != nil {
		t.Fatal(err)
	}
	read := func(operation context.Context, s application.ReleaseOrderSession) ([]domain.RecordBaseline, error) {
		policy, err := s.GetTablePolicy(operation, "guard_snapshot_a")
		if err != nil {
			return nil, err
		}
		// The catalog read precedes business reads in the real resolver and is an
		// ordinary InnoDB consistent read. The table guard must precede even this.
		if _, err = s.GetQueryPolicy(operation, policy.QueryPolicyCode); err != nil {
			return nil, err
		}
		return s.ReadRecordBaselines(operation, schema, []any{int64(1)})
	}
	t.Run("publication-first", func(t *testing.T) {
		operation, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ready, resume := make(chan struct{}), make(chan struct{})
		publisher := make(chan error, 1)
		go func() {
			publisher <- adapter.ExecutePublication(operation, func(session application.PublicationSession) error {
				if err := session.LockPublicationTable(operation, "guard_snapshot_a"); err != nil {
					return err
				}
				if err := session.(*publicationSession).database.Exec(`UPDATE guard_snapshot_a SET code=10 WHERE id=1`).Error; err != nil {
					return err
				}
				close(ready)
				select {
				case <-resume:
					return nil
				case <-operation.Done():
					return operation.Err()
				}
			})
		}()
		select {
		case <-ready:
		case err := <-publisher:
			t.Fatal(err)
		case <-operation.Done():
			t.Fatal(operation.Err())
		}
		drafts := make(chan error, 1)
		go func() {
			drafts <- adapter.ExecuteReleaseOrder(operation, func(s application.ReleaseOrderSession) error {
				rows, err := read(operation, s)
				if err != nil {
					return err
				}
				if len(rows) != 1 || rows[0].Row["code"] == nil || *rows[0].Row["code"] != "10" {
					return fmt.Errorf("draft kept pre-publication snapshot: %+v", rows)
				}
				return nil
			})
		}()
		var early error
		completed := false
		select {
		case early = <-drafts:
			completed = true
		case <-time.After(150 * time.Millisecond):
		}
		close(resume)
		if err := <-publisher; err != nil {
			t.Fatal(err)
		}
		if completed {
			t.Fatalf("draft crossed the unfinished publication: %v", early)
		}
		if err := <-drafts; err != nil {
			t.Fatal(err)
		}
	})
	t.Run("draft-first-and-independent-table", func(t *testing.T) {
		operation, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ready, resume := make(chan struct{}), make(chan struct{})
		draft := make(chan error, 1)
		go func() {
			draft <- adapter.ExecuteReleaseOrder(operation, func(s application.ReleaseOrderSession) error {
				rows, err := read(operation, s)
				if err != nil {
					return err
				}
				if *rows[0].Row["code"] != "10" {
					return fmt.Errorf("wrong draft baseline")
				}
				close(ready)
				select {
				case <-resume:
					return nil
				case <-operation.Done():
					return operation.Err()
				}
			})
		}()
		select {
		case <-ready:
		case err := <-draft:
			t.Fatal(err)
		case <-operation.Done():
			t.Fatal(operation.Err())
		}
		publisher := make(chan error, 1)
		go func() {
			publisher <- adapter.ExecutePublication(operation, func(s application.PublicationSession) error {
				return s.LockPublicationTable(operation, "guard_snapshot_a")
			})
		}()
		// The same draft's read lock cannot block a publication for another table.
		independent := make(chan error, 1)
		go func() {
			independent <- adapter.ExecutePublication(operation, func(s application.PublicationSession) error {
				return s.LockPublicationTable(operation, "guard_snapshot_b")
			})
		}()
		var early error
		completed := false
		select {
		case early = <-publisher:
			completed = true
		case <-time.After(150 * time.Millisecond):
		}
		select {
		case err := <-independent:
			if err != nil {
				close(resume)
				t.Fatal(err)
			}
		case <-operation.Done():
			close(resume)
			t.Fatal(operation.Err())
		}
		close(resume)
		if err := <-draft; err != nil {
			t.Fatal(err)
		}
		if completed {
			t.Fatalf("publication crossed draft baseline transaction: %v", early)
		}
		if err := <-publisher; err != nil {
			t.Fatal(err)
		}
	})
}
