package domain

import (
	"context"
	"errors"
	"time"
)

// TableReleaseTemplate selects the single template for one table and release type.
type TableReleaseTemplate struct {
	TableName       string
	Type            ReleaseType
	TemplateCode    string
	TemplateName    string
	TemplateEnabled bool
	Enabled         bool
	Version         uint64
	Creator         string
	Modifier        string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

var (
	ErrInvalidTableReleaseTemplate   = errors.New("invalid table Release Template association")
	ErrTableReleaseTemplateConflict  = errors.New("table Release Template association changed")
	ErrEmergencyAssociationProtected = errors.New("emergency association must remain available")
	ErrReleaseTemplateInUse          = errors.New("Release Template is referenced by a table")
)

type TableReleaseTemplateCatalog interface {
	ListTableReleaseTemplates(context.Context, string) ([]TableReleaseTemplate, error)
	PutTableReleaseTemplate(context.Context, TableReleaseTemplate, uint64, string, string, string) (TableReleaseTemplate, error)
}
