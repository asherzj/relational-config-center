package mysql

import (
	"context"
	"errors"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (a *Adapter) ExecuteTablePolicyRequest(ctx context.Context, actor, action, table, key, digest string, version uint64, write func(application.TablePolicyRequestSession) (domain.TablePolicy, error)) (domain.TablePolicy, error) {
	return executeManagementRequest(ctx, a, actor, "table-policy:"+action, key, digest, func(tx *gorm.DB) (domain.TablePolicy, error) {
		if action != "create" {
			var policy policyRecord
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("table_name=?", table).Take(&policy).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
			} else if err != nil {
				return domain.TablePolicy{}, err
			}
			if policy.Version != version {
				return domain.TablePolicy{}, application.ErrTablePolicyVersionConflict
			}
		}
		session := &Adapter{database: a.database, gorm: tx, pool: a.pool}
		return write(session)
	})
}
