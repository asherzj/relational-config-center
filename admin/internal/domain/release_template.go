package domain

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
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

var releaseNodeCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// ValidateReleaseTemplateNodes validates persisted, already-normalized definitions.
// It is shared by authoring, association changes and read-only readiness.
func ValidateReleaseTemplateNodes(kind ReleaseType, nodes []ReleaseTemplateNode) error {
	var expected []string
	switch kind {
	case ReleaseTypeStandard:
		expected = []string{"APPROVAL", "PUBLICATION", "COMPLETION"}
	case ReleaseTypeEmergency:
		expected = []string{"PUBLICATION", "COMPLETION"}
	default:
		return ErrInvalidReleaseTemplate
	}
	if len(nodes) != len(expected) {
		return ErrInvalidReleaseTemplate
	}
	seen := make(map[string]bool, len(nodes))
	for i, node := range nodes {
		role := "PUBLISHER"
		if node.Type == "APPROVAL" {
			role = "TABLE_APPROVER"
		}
		if !releaseNodeCodePattern.MatchString(node.Code) || seen[node.Code] || node.Type != expected[i] || strings.TrimSpace(node.Name) == "" || utf8.RuneCountInString(node.Name) > 100 || node.RequiredRole != role {
			return ErrInvalidReleaseTemplate
		}
		seen[node.Code] = true
	}
	return nil
}
