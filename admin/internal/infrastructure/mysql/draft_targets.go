package mysql

import (
	"context"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// The parent order lock serializes its writers. Read the old set without an
// empty-range lock, acquire the full desired set first, then delete exact keys.
// Shared references are collapsed by the target identity, so the last reference
// controls release. A failed acquisition rolls the entire save back.
func (s *releaseOrderSession) ReplaceReleaseTargets(ctx context.Context, orderID string, targets []domain.ActiveTarget) error {
	if err := s.available(); err != nil {
		return err
	}
	var previous []struct {
		TableName string
		RecordKey []byte
	}
	if err := s.database.WithContext(ctx).Raw(`SELECT table_name,record_key FROM rcc_release_targets WHERE order_id=?`, orderID).Scan(&previous).Error; err != nil {
		return application.ErrReleaseUnavailable
	}
	if err := s.ReserveReleaseTargets(ctx, orderID, targets); err != nil {
		return err
	}
	needed := map[string]map[string]bool{}
	for _, target := range targets {
		if needed[target.TableName] == nil {
			needed[target.TableName] = map[string]bool{}
		}
		needed[target.TableName][string(target.RecordKey)] = true
	}
	for _, target := range previous {
		if needed[target.TableName][string(target.RecordKey)] {
			continue
		}
		if err := s.database.WithContext(ctx).Exec(`DELETE FROM rcc_release_targets WHERE table_name=? AND record_key=? AND order_id=?`, target.TableName, target.RecordKey, orderID).Error; err != nil {
			return application.ErrReleaseUnavailable
		}
	}
	return nil
}
