package mysql

import (
	"context"
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

func (a *Adapter) CreateReleaseTemplate(ctx context.Context, template domain.ReleaseTemplate, operator, requestKey, digestHex string) (domain.ReleaseTemplate, error) {
	return executeManagementRequest(ctx, a, operator, createReleaseTemplateOperation, requestKey, digestHex, func(transaction *gorm.DB) (domain.ReleaseTemplate, error) {
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
	return executeManagementRequest(ctx, a, operator, "release-template:replace", requestKey, digestHex, func(transaction *gorm.DB) (domain.ReleaseTemplate, error) {
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
	return executeManagementRequest(ctx, a, operator, "release-template:"+action, requestKey, digestHex, func(transaction *gorm.DB) (domain.ReleaseTemplate, error) {
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
	_, err := executeManagementRequest(ctx, a, operator, "release-template:delete", requestKey, digestHex, func(transaction *gorm.DB) (domain.ReleaseTemplate, error) {
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
			var mysqlError *driver.MySQLError
			if errors.As(err, &mysqlError) && mysqlError.Number == 1451 {
				return domain.ReleaseTemplate{}, domain.ErrReleaseTemplateInUse
			}
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
