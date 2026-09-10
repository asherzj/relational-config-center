package mysql

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

const createReleaseTemplateOperation = "release-template:create"

type releaseTemplateRecord struct {
	ID          uint64                       `gorm:"column:id;primaryKey"`
	Code        string                       `gorm:"column:code"`
	Name        string                       `gorm:"column:name"`
	Description string                       `gorm:"column:description"`
	Type        domain.ReleaseType           `gorm:"column:release_type"`
	Nodes       []domain.ReleaseTemplateNode `gorm:"column:node_list;serializer:json"`
	MonitorList []string                     `gorm:"column:monitor_list;serializer:json"`
	Enabled     bool                         `gorm:"column:enabled"`
	Version     uint64                       `gorm:"column:version"`
	Creator     string                       `gorm:"column:creator"`
	Modifier    string                       `gorm:"column:modifier"`
	CreatedAt   time.Time                    `gorm:"column:created_at"`
	UpdatedAt   time.Time                    `gorm:"column:updated_at"`
}

func (releaseTemplateRecord) TableName() string { return "rcc_release_templates" }

// Every template write locks its request before its target and saves the result
// in the same transaction. Replays therefore never consult a later template or
// a new template that reused the code after deletion.
func (a *Adapter) executeReleaseTemplateRequest(ctx context.Context, operator, operation, requestKey, digestHex string, write func(*gorm.DB) (domain.ReleaseTemplate, error)) (domain.ReleaseTemplate, error) {
	digest, err := hex.DecodeString(digestHex)
	if err != nil || len(digest) != 32 {
		return domain.ReleaseTemplate{}, fmt.Errorf("write Release Template: invalid request digest")
	}
	transaction := a.gorm.WithContext(ctx).Begin()
	if transaction.Error != nil {
		return domain.ReleaseTemplate{}, fmt.Errorf("begin Release Template request: %w", transaction.Error)
	}
	defer func() { _ = transaction.Rollback().Error }()
	if err := transaction.Exec(`INSERT INTO rcc_release_requests(actor_id,operation,request_key,digest,result) VALUES(?,?,?,?,NULL) ON DUPLICATE KEY UPDATE request_key=request_key`, operator, operation, requestKey, digest).Error; err != nil {
		return domain.ReleaseTemplate{}, fmt.Errorf("begin Release Template request: %w", err)
	}
	var storedDigest, result []byte
	if err := transaction.Raw(`SELECT digest,result FROM rcc_release_requests WHERE actor_id=? AND operation=? AND request_key=? FOR UPDATE`, operator, operation, requestKey).Row().Scan(&storedDigest, &result); err != nil {
		return domain.ReleaseTemplate{}, fmt.Errorf("read Release Template request: %w", err)
	}
	if !bytes.Equal(storedDigest, digest) {
		return domain.ReleaseTemplate{}, domain.ErrReleaseTemplateIdempotencyConflict
	}
	var saved domain.ReleaseTemplate
	if result != nil {
		if err := json.Unmarshal(result, &saved); err != nil {
			return domain.ReleaseTemplate{}, fmt.Errorf("read Release Template request result: %w", err)
		}
	} else {
		saved, err = write(transaction)
		if err != nil {
			return domain.ReleaseTemplate{}, err
		}
		encoded, err := json.Marshal(saved)
		if err != nil {
			return domain.ReleaseTemplate{}, fmt.Errorf("encode Release Template result: %w", err)
		}
		if err := transaction.Exec(`UPDATE rcc_release_requests SET result=? WHERE actor_id=? AND operation=? AND request_key=?`, encoded, operator, operation, requestKey).Error; err != nil {
			return domain.ReleaseTemplate{}, fmt.Errorf("save Release Template request result: %w", err)
		}
	}
	if err := transaction.Commit().Error; err != nil {
		return domain.ReleaseTemplate{}, fmt.Errorf("commit Release Template request: %w", err)
	}
	return saved, nil
}

func (a *Adapter) CreateReleaseTemplate(ctx context.Context, template domain.ReleaseTemplate, operator, requestKey, digestHex string) (domain.ReleaseTemplate, error) {
	return a.executeReleaseTemplateRequest(ctx, operator, createReleaseTemplateOperation, requestKey, digestHex, func(transaction *gorm.DB) (domain.ReleaseTemplate, error) {
		record := releaseTemplateRecord{Code: template.Code, Name: template.Name, Description: template.Description, Type: template.Type, Nodes: template.Nodes, MonitorList: []string{}, Enabled: true, Version: 1, Creator: operator, Modifier: operator}
		if err := transaction.Create(&record).Error; err != nil {
			var mysqlError *driver.MySQLError
			if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
				return domain.ReleaseTemplate{}, domain.ErrReleaseTemplateExists
			}
			return domain.ReleaseTemplate{}, fmt.Errorf("create Release Template: %w", err)
		}
		stored, err := readReleaseTemplate(transaction, record.Code)
		return stored.template(), err
	})
}

func (a *Adapter) ListReleaseTemplates(ctx context.Context) ([]domain.ReleaseTemplate, error) {
	var records []releaseTemplateRecord
	if err := a.gorm.WithContext(ctx).Order("release_type, code").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list Release Templates: %w", err)
	}
	templates := make([]domain.ReleaseTemplate, 0, len(records))
	for _, record := range records {
		templates = append(templates, record.template())
	}
	return templates, nil
}

func (a *Adapter) GetReleaseTemplate(ctx context.Context, code string) (domain.ReleaseTemplate, error) {
	record, err := readReleaseTemplate(a.gorm.WithContext(ctx), code)
	return record.template(), err
}

func readReleaseTemplate(database *gorm.DB, code string) (releaseTemplateRecord, error) {
	var record releaseTemplateRecord
	err := database.Where("code = ?", code).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, domain.ErrReleaseTemplateNotFound
	}
	if err != nil {
		return record, fmt.Errorf("get Release Template: %w", err)
	}
	return record, nil
}

func (a *Adapter) ReplaceReleaseTemplate(ctx context.Context, template domain.ReleaseTemplate, operator string, expectedVersion uint64, requestKey, digestHex string) (domain.ReleaseTemplate, error) {
	return a.executeReleaseTemplateRequest(ctx, operator, "release-template:replace", requestKey, digestHex, func(transaction *gorm.DB) (domain.ReleaseTemplate, error) {
		current, err := readReleaseTemplate(transaction.Clauses(clause.Locking{Strength: "UPDATE"}), template.Code)
		if err != nil {
			return domain.ReleaseTemplate{}, err
		}
		if current.Type != template.Type {
			return domain.ReleaseTemplate{}, domain.ErrInvalidReleaseTemplate
		}
		if current.Version != expectedVersion {
			return domain.ReleaseTemplate{}, domain.ErrReleaseTemplateVersionConflict
		}
		nodes, err := json.Marshal(template.Nodes)
		if err != nil {
			return domain.ReleaseTemplate{}, fmt.Errorf("replace Release Template: encode nodes: %w", err)
		}
		if err := transaction.Model(&current).Updates(map[string]any{"name": template.Name, "description": template.Description, "node_list": nodes, "modifier": operator, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return domain.ReleaseTemplate{}, fmt.Errorf("replace Release Template: %w", err)
		}
		stored, err := readReleaseTemplate(transaction, template.Code)
		return stored.template(), err
	})
}

func (a *Adapter) SetReleaseTemplateEnabled(ctx context.Context, code string, enabled bool, operator string, expectedVersion uint64, requestKey, digestHex string) (domain.ReleaseTemplate, error) {
	action := "disable"
	if enabled {
		action = "enable"
	}
	return a.executeReleaseTemplateRequest(ctx, operator, "release-template:"+action, requestKey, digestHex, func(transaction *gorm.DB) (domain.ReleaseTemplate, error) {
		current, err := readReleaseTemplate(transaction.Clauses(clause.Locking{Strength: "UPDATE"}), code)
		if err != nil {
			return domain.ReleaseTemplate{}, err
		}
		if current.Type == domain.ReleaseTypeEmergency && !enabled {
			return domain.ReleaseTemplate{}, domain.ErrEmergencyReleaseTemplateProtected
		}
		if current.Version != expectedVersion {
			return domain.ReleaseTemplate{}, domain.ErrReleaseTemplateVersionConflict
		}
		if err := transaction.Model(&current).Updates(map[string]any{"enabled": enabled, "modifier": operator, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return domain.ReleaseTemplate{}, fmt.Errorf("set Release Template enabled state: %w", err)
		}
		stored, err := readReleaseTemplate(transaction, code)
		return stored.template(), err
	})
}

func (a *Adapter) DeleteReleaseTemplate(ctx context.Context, code string, expectedVersion uint64, operator, requestKey, digestHex string) error {
	_, err := a.executeReleaseTemplateRequest(ctx, operator, "release-template:delete", requestKey, digestHex, func(transaction *gorm.DB) (domain.ReleaseTemplate, error) {
		current, err := readReleaseTemplate(transaction.Clauses(clause.Locking{Strength: "UPDATE"}), code)
		if err != nil {
			return domain.ReleaseTemplate{}, err
		}
		if current.Type == domain.ReleaseTypeEmergency {
			return domain.ReleaseTemplate{}, domain.ErrEmergencyReleaseTemplateProtected
		}
		if current.Version != expectedVersion {
			return domain.ReleaseTemplate{}, domain.ErrReleaseTemplateVersionConflict
		}
		if err := transaction.Delete(&current).Error; err != nil {
			return domain.ReleaseTemplate{}, fmt.Errorf("delete Release Template: %w", err)
		}
		// Persist the completed removal independently of the deleted row. The
		// transport still returns 204 for both the first response and replays.
		return current.template(), nil
	})
	return err
}

func (record releaseTemplateRecord) template() domain.ReleaseTemplate {
	return domain.ReleaseTemplate{Code: record.Code, Name: record.Name, Description: record.Description, Type: record.Type, Nodes: append([]domain.ReleaseTemplateNode{}, record.Nodes...), MonitorList: append([]string{}, record.MonitorList...), Enabled: record.Enabled, Version: record.Version, Creator: record.Creator, Modifier: record.Modifier, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}
