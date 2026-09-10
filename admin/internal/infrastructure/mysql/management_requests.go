package mysql

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

// Request identity, business changes and the original result share one commit.
// The request lock precedes target locks, so concurrent replays observe one result.
func executeManagementRequest[T any](ctx context.Context, a *Adapter, actor, operation, key, digestHex string, write func(*gorm.DB) (T, error)) (T, error) {
	var saved T
	digest, err := hex.DecodeString(digestHex)
	if err != nil || len(digest) != 32 {
		return saved, fmt.Errorf("invalid management request digest")
	}
	tx := a.gorm.WithContext(ctx).Begin()
	if tx.Error != nil {
		return saved, tx.Error
	}
	defer func() { _ = tx.Rollback().Error }()
	if err := tx.Exec(`INSERT INTO rcc_release_requests(actor_id,operation,request_key,digest,result) VALUES(?,?,?,?,NULL) ON DUPLICATE KEY UPDATE request_key=request_key`, actor, operation, key, digest).Error; err != nil {
		return saved, err
	}
	var previous, result []byte
	if err := tx.Raw(`SELECT digest,result FROM rcc_release_requests WHERE actor_id=? AND operation=? AND request_key=? FOR UPDATE`, actor, operation, key).Row().Scan(&previous, &result); err != nil {
		return saved, err
	}
	if !bytes.Equal(previous, digest) {
		return saved, domain.ErrReleaseTemplateIdempotencyConflict
	}
	if result != nil {
		if err := json.Unmarshal(result, &saved); err != nil {
			return saved, err
		}
	} else {
		saved, err = write(tx)
		if err != nil {
			return saved, err
		}
		encoded, err := json.Marshal(saved)
		if err != nil {
			return saved, err
		}
		if err := tx.Exec(`UPDATE rcc_release_requests SET result=? WHERE actor_id=? AND operation=? AND request_key=?`, encoded, actor, operation, key).Error; err != nil {
			return saved, err
		}
	}
	return saved, tx.Commit().Error
}
