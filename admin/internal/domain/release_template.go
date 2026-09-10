package domain

import (
	"context"
	"errors"
	"time"
)

type ReleaseType string

const (
	ReleaseTypeStandard  ReleaseType = "STANDARD"
	ReleaseTypeEmergency ReleaseType = "EMERGENCY"
)

type ReleaseTemplateNode struct {
	Code         string `json:"code"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	RequiredRole string `json:"required_role"`
}

type ReleaseTemplate struct {
	Code        string
	Name        string
	Description string
	Type        ReleaseType
	Nodes       []ReleaseTemplateNode
	MonitorList []string
	Enabled     bool
	Version     uint64
	Creator     string
	Modifier    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var (
	ErrInvalidReleaseTemplate             = errors.New("invalid Release Template")
	ErrReleaseTemplateExists              = errors.New("Release Template already exists")
	ErrReleaseTemplateNotFound            = errors.New("Release Template not found")
	ErrReleaseTemplateVersionConflict     = errors.New("Release Template changed")
	ErrReleaseTemplateIdempotencyConflict = errors.New("Release Template request key reused for different content")
	ErrEmergencyReleaseTemplateProtected  = errors.New("Emergency Release Template must remain available")
)

type ReleaseTemplateCatalog interface {
	CreateReleaseTemplate(context.Context, ReleaseTemplate, string, string, string) (ReleaseTemplate, error)
	ListReleaseTemplates(context.Context) ([]ReleaseTemplate, error)
	GetReleaseTemplate(context.Context, string) (ReleaseTemplate, error)
	ReplaceReleaseTemplate(context.Context, ReleaseTemplate, string, uint64, string, string) (ReleaseTemplate, error)
	SetReleaseTemplateEnabled(context.Context, string, bool, string, uint64, string, string) (ReleaseTemplate, error)
	DeleteReleaseTemplate(context.Context, string, uint64, string, string, string) error
}
