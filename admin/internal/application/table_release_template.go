package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

type TableReleaseTemplate = domain.TableReleaseTemplate

var ErrInvalidTableReleaseTemplate = domain.ErrInvalidTableReleaseTemplate
var ErrTableReleaseTemplateConflict = domain.ErrTableReleaseTemplateConflict
var ErrEmergencyAssociationProtected = domain.ErrEmergencyAssociationProtected
var ErrReleaseTemplateInUse = domain.ErrReleaseTemplateInUse

type TableReleaseTemplateManagement struct {
	catalog domain.TableReleaseTemplateCatalog
}

func NewTableReleaseTemplateManagement(catalog domain.TableReleaseTemplateCatalog) *TableReleaseTemplateManagement {
	return &TableReleaseTemplateManagement{catalog: catalog}
}

func (m *TableReleaseTemplateManagement) List(ctx context.Context, table string) ([]TableReleaseTemplate, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return nil, err
	}
	if protectedTable(table) {
		return nil, ErrProtectedTable
	}
	return m.catalog.ListTableReleaseTemplates(ctx, table)
}

func (m *TableReleaseTemplateManagement) Put(ctx context.Context, table, releaseType, code string, enabled bool, version uint64, key string) (TableReleaseTemplate, error) {
	actor, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return TableReleaseTemplate{}, err
	}
	if protectedTable(table) {
		return TableReleaseTemplate{}, ErrProtectedTable
	}
	if strings.TrimSpace(table) == "" || !templateCodePattern.MatchString(code) || !roleRequestKey.MatchString(key) {
		return TableReleaseTemplate{}, ErrInvalidTableReleaseTemplate
	}
	kind := domain.ReleaseType(releaseType)
	if kind != domain.ReleaseTypeStandard && kind != domain.ReleaseTypeEmergency {
		return TableReleaseTemplate{}, ErrUnknownReleaseType
	}
	if kind == domain.ReleaseTypeEmergency && !enabled {
		return TableReleaseTemplate{}, ErrEmergencyAssociationProtected
	}
	candidate := TableReleaseTemplate{TableName: table, Type: kind, TemplateCode: code, Enabled: enabled}
	encoded, _ := json.Marshal(struct {
		Candidate       TableReleaseTemplate
		ExpectedVersion uint64
	}{candidate, version})
	digest := sha256.Sum256(encoded)
	return m.catalog.PutTableReleaseTemplate(ctx, candidate, version, actor, key, hex.EncodeToString(digest[:]))
}
