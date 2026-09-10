package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"slices"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type concurrencyColumns []string

func (c concurrencyColumns) Value() (driver.Value, error) {
	if c == nil {
		c = concurrencyColumns{}
	}
	b, e := json.Marshal(c)
	return string(b), e
}
func (c *concurrencyColumns) Scan(value any) error {
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, c)
	case string:
		return json.Unmarshal([]byte(v), c)
	default:
		return application.ErrConcurrencyKeyInvalid
	}
}
func (concurrencyColumns) GormDataType() string { return "json" }

// Normalize MySQL identifier case without reading a table. Even a dictionary
// view can establish the RR snapshot, so existence/schema reads must wait until
// every table guard is held. This scalar read uses the transaction connection.
func (s *releaseOrderSession) ResolveReleaseTable(ctx context.Context, name string) (string, error) {
	if err := s.available(); err != nil {
		return "", err
	}
	physical, err := canonicalTableName(ctx, s.database, name)
	if err != nil {
		return "", application.ErrReleaseUnavailable
	}
	return physical, nil
}

func canonicalTableName(ctx context.Context, db *gorm.DB, name string) (string, error) {
	var canonical string
	err := db.WithContext(ctx).Raw(`SELECT IF(@@lower_case_table_names=0,?,LOWER(?))`, name, name).Row().Scan(&canonical)
	return canonical, err
}

// Drafts take a shared lock before reading their Policy Snapshot. Publication
// and rollback take this guard exclusively through business writes and release.
// Replacement also takes it exclusively before checking references; no order
// lock is needed, avoiding a policy → order / order → policy cycle.
func (s *releaseOrderSession) GetTablePolicy(ctx context.Context, table string) (domain.TablePolicy, error) {
	if err := s.available(); err != nil {
		return domain.TablePolicy{}, err
	}
	var record policyRecord
	err := s.database.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("table_name = ? AND BINARY table_name = BINARY ?", table, table).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	return record.policy(), err
}

func (a *Adapter) replaceTablePolicy(ctx context.Context, policy domain.TablePolicy, operator string, active bool) (domain.TablePolicy, error) {
	var physical string
	if err := a.gorm.WithContext(ctx).Raw(`SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND (BINARY TABLE_NAME=BINARY ? OR (@@lower_case_table_names<>0 AND TABLE_NAME=?))`, policy.TableName, policy.TableName).Row().Scan(&physical); err != nil {
		return domain.TablePolicy{}, application.ErrDatabaseTableNotFound
	}
	policy.TableName = physical
	err := a.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current policyRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("table_name = ? AND BINARY table_name = BINARY ?", policy.TableName, policy.TableName).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrTablePolicyNotFound
			}
			return err
		}
		// Use a current read even if validation later gains earlier snapshot reads.
		// Only definition replacement reads this range, under the exclusive table
		// guard; it never inserts references. Draft writers lock no empty ranges.
		if !slices.Equal([]string(current.ConcurrencyKey), policy.ConcurrencyKey) {
			var reference string
			err := tx.Raw(`SELECT order_id FROM rcc_release_table_references WHERE table_name=? LIMIT 1 FOR SHARE`, policy.TableName).Row().Scan(&reference)
			if err == nil {
				return application.ErrConcurrencyKeyInUse
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if active {
			if err := activeAssignmentDefinitions(tx, policy.QueryPolicyCode, policy.MutationPolicyCode); err != nil {
				return err
			}
		}
		schema, err := a.getTableSchema(ctx, tx, policy.TableName)
		if err != nil {
			return err
		}
		var mutation mutationPolicyRecord
		if err := tx.Where("code = ?", policy.MutationPolicyCode).First(&mutation).Error; err != nil {
			return err
		}
		if err := application.ValidateConcurrencyKey(schema, mutation.policy(), policy.ConcurrencyKey); err != nil {
			return err
		}
		return tx.Model(&policyRecord{}).Where("id = ?", current.ID).Updates(map[string]any{"query_policy_code": policy.QueryPolicyCode, "mutation_policy_code": policy.MutationPolicyCode, "concurrency_key": concurrencyColumns(policy.ConcurrencyKey), "modifier": operator}).Error
	})
	if err != nil {
		return domain.TablePolicy{}, err
	}
	return a.Get(ctx, policy.TableName)
}

func (s *releaseOrderSession) ReplaceReleaseTableReferences(ctx context.Context, order string, tables []string) error {
	if err := s.available(); err != nil {
		return err
	}
	var previous []string
	if err := s.database.WithContext(ctx).Raw(`SELECT table_name FROM rcc_release_table_references WHERE order_id=?`, order).Scan(&previous).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	tables = slices.Clone(tables)
	slices.Sort(tables)
	tables = slices.Compact(tables)
	for _, table := range tables {
		if slices.Contains(previous, table) {
			continue
		}
		if err := s.database.WithContext(ctx).Exec(`INSERT INTO rcc_release_table_references(table_name,order_id) VALUES(?,?)`, table, order).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	for _, table := range previous {
		if slices.Contains(tables, table) {
			continue
		}
		if err := s.database.WithContext(ctx).Exec(`DELETE FROM rcc_release_table_references WHERE table_name=? AND order_id=?`, table, order).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	return nil
}
