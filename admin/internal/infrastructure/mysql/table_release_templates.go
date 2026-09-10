package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tableReleaseTemplateRecord struct {
	ID            uint64             `gorm:"column:id;primaryKey"`
	TablePolicyID uint64             `gorm:"column:table_policy_id"`
	Type          domain.ReleaseType `gorm:"column:release_type"`
	TemplateID    uint64             `gorm:"column:template_id"`
	Enabled       bool               `gorm:"column:enabled"`
	Version       uint64             `gorm:"column:version"`
	Creator       string             `gorm:"column:creator"`
	Modifier      string             `gorm:"column:modifier"`
	CreatedAt     time.Time          `gorm:"column:created_at"`
	UpdatedAt     time.Time          `gorm:"column:updated_at"`
}

func (tableReleaseTemplateRecord) TableName() string { return "rcc_table_release_templates" }

func (a *Adapter) ListTableReleaseTemplates(ctx context.Context, table string) ([]domain.TableReleaseTemplate, error) {
	var policy policyRecord
	if err := a.gorm.WithContext(ctx).Where("table_name = ?", table).Take(&policy).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrTablePolicyNotFound
	} else if err != nil {
		return nil, err
	}
	return readTableReleaseTemplates(a.gorm.WithContext(ctx), policy)
}

func readTableReleaseTemplates(db *gorm.DB, policy policyRecord) ([]domain.TableReleaseTemplate, error) {
	// One joined statement observes a complete association and template projection.
	var rows []struct {
		Record          tableReleaseTemplateRecord `gorm:"embedded"`
		TemplateCode    string
		TemplateName    string
		TemplateEnabled bool
	}
	err := db.Table("rcc_table_release_templates AS a").Select("a.*, t.code AS template_code, t.name AS template_name, t.enabled AS template_enabled").Joins("JOIN rcc_release_templates AS t ON t.id=a.template_id AND t.release_type=a.release_type").Where("a.table_policy_id=?", policy.ID).Order("a.release_type").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.TableReleaseTemplate, 0, len(rows))
	for _, row := range rows {
		r := row.Record
		result = append(result, domain.TableReleaseTemplate{TableName: policy.Table, Type: r.Type, TemplateCode: row.TemplateCode, TemplateName: row.TemplateName, TemplateEnabled: row.TemplateEnabled, Enabled: r.Enabled, Version: r.Version, Creator: r.Creator, Modifier: r.Modifier, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	}
	return result, nil
}

func (a *Adapter) PutTableReleaseTemplate(ctx context.Context, candidate domain.TableReleaseTemplate, version uint64, actor, key, digest string) (domain.TableReleaseTemplate, error) {
	return executeManagementRequest(ctx, a, actor, "table-release-template:put", key, digest, func(tx *gorm.DB) (domain.TableReleaseTemplate, error) {
		var policy policyRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("table_name=?", candidate.TableName).Take(&policy).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.TableReleaseTemplate{}, domain.ErrTablePolicyNotFound
		} else if err != nil {
			return domain.TableReleaseTemplate{}, err
		}
		var current tableReleaseTemplateRecord
		err := tx.Where("table_policy_id=? AND release_type=?", policy.ID, candidate.Type).Take(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.TableReleaseTemplate{}, err
		}
		if current.Version != version {
			return domain.TableReleaseTemplate{}, domain.ErrTableReleaseTemplateConflict
		}
		template, err := readReleaseTemplate(tx.Clauses(clause.Locking{Strength: "SHARE"}), candidate.TemplateCode)
		if err != nil {
			return domain.TableReleaseTemplate{}, err
		}
		if template.Type != candidate.Type || (candidate.Enabled && !template.Enabled) || domain.ValidateReleaseTemplateNodes(template.Type, template.Nodes) != nil || len(template.MonitorList) != 0 {
			return domain.TableReleaseTemplate{}, domain.ErrInvalidTableReleaseTemplate
		}
		if candidate.Type == domain.ReleaseTypeEmergency && !candidate.Enabled {
			return domain.TableReleaseTemplate{}, domain.ErrEmergencyAssociationProtected
		}
		if current.ID == 0 {
			current = tableReleaseTemplateRecord{TablePolicyID: policy.ID, Type: candidate.Type, TemplateID: template.ID, Enabled: candidate.Enabled, Version: 1, Creator: actor, Modifier: actor}
			err = tx.Create(&current).Error
		} else {
			err = tx.Model(&current).Updates(map[string]any{"template_id": template.ID, "enabled": candidate.Enabled, "modifier": actor, "version": gorm.Expr("version + 1")}).Error
		}
		if err != nil {
			return domain.TableReleaseTemplate{}, err
		}
		rows, err := readTableReleaseTemplates(tx, policy)
		if err != nil {
			return domain.TableReleaseTemplate{}, err
		}
		for _, row := range rows {
			if row.Type == candidate.Type {
				return row, nil
			}
		}
		return domain.TableReleaseTemplate{}, fmt.Errorf("saved association not found")
	})
}

// Called under the table-policy guard, in the same transaction as creation or
// enabling. Existing choices are validated and never replaced with defaults.
func ensureEmergencyTableReleaseTemplate(tx *gorm.DB, policy policyRecord, actor string) error {
	var current tableReleaseTemplateRecord
	err := tx.Where("table_policy_id=? AND release_type='EMERGENCY'", policy.ID).Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		template, err := readReleaseTemplate(tx.Clauses(clause.Locking{Strength: "SHARE"}), "default_emergency_v1")
		if err != nil {
			return err
		}
		if template.Type != domain.ReleaseTypeEmergency || !template.Enabled || domain.ValidateReleaseTemplateNodes(template.Type, template.Nodes) != nil || len(template.MonitorList) != 0 {
			return domain.ErrEmergencyAssociationProtected
		}
		return tx.Create(&tableReleaseTemplateRecord{TablePolicyID: policy.ID, Type: domain.ReleaseTypeEmergency, TemplateID: template.ID, Enabled: true, Version: 1, Creator: actor, Modifier: actor}).Error
	}
	if err != nil {
		return err
	}
	var template releaseTemplateRecord
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id=?", current.TemplateID).Take(&template).Error; err != nil {
		return err
	}
	if !current.Enabled || !template.Enabled || template.Type != domain.ReleaseTypeEmergency || domain.ValidateReleaseTemplateNodes(template.Type, template.Nodes) != nil || len(template.MonitorList) != 0 {
		return domain.ErrEmergencyAssociationProtected
	}
	return nil
}
