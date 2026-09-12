package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrInvalidReleaseTemplate      = domain.ErrInvalidReleaseTemplate
	ErrUnknownReleaseType          = errors.New("unknown Release Type")
	ErrInvalidReleaseTemplateNodes = errors.New("invalid Release Template nodes")
)

var templateCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*_v[1-9][0-9]*$`)
var templateNodeCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type PutReleaseTemplateNode struct {
	Code         string `json:"code"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	RequiredRole string `json:"required_role"`
}

type PutReleaseTemplate struct {
	Code        string                   `json:"code"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Type        string                   `json:"type"`
	Nodes       []PutReleaseTemplateNode `json:"node_list"`
}

type ReleaseTemplateManagement struct{ catalog domain.ReleaseTemplateCatalog }

func NewReleaseTemplateManagement(catalog domain.ReleaseTemplateCatalog) *ReleaseTemplateManagement {
	return &ReleaseTemplateManagement{catalog: catalog}
}

func (m *ReleaseTemplateManagement) Create(ctx context.Context, candidate PutReleaseTemplate, requestKey string) (domain.ReleaseTemplate, error) {
	operator, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return domain.ReleaseTemplate{}, err
	}
	template, err := validatedReleaseTemplate(candidate)
	if err != nil {
		return domain.ReleaseTemplate{}, err
	}
	if !roleRequestKey.MatchString(requestKey) {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplate
	}
	digest, err := releaseTemplateDigest("create", template.Code, 0, template)
	if err != nil {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplate
	}
	return m.catalog.CreateReleaseTemplate(ctx, template, operator, requestKey, digest)
}

func (m *ReleaseTemplateManagement) List(ctx context.Context) ([]domain.ReleaseTemplate, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return nil, err
	}
	return m.catalog.ListReleaseTemplates(ctx)
}

func (m *ReleaseTemplateManagement) Get(ctx context.Context, code string) (domain.ReleaseTemplate, error) {
	if _, err := requireRole(ctx, RoleAdmin); err != nil {
		return domain.ReleaseTemplate{}, err
	}
	if !templateCodePattern.MatchString(code) {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplate
	}
	return m.catalog.GetReleaseTemplate(ctx, code)
}

func (m *ReleaseTemplateManagement) Replace(ctx context.Context, code string, candidate PutReleaseTemplate, version uint64, requestKey string) (domain.ReleaseTemplate, error) {
	operator, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return domain.ReleaseTemplate{}, err
	}
	if version == 0 || candidate.Code != code || !roleRequestKey.MatchString(requestKey) {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplate
	}
	template, err := validatedReleaseTemplate(candidate)
	if err != nil {
		return domain.ReleaseTemplate{}, err
	}
	digest, err := releaseTemplateDigest("replace", code, version, template)
	if err != nil {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplate
	}
	return m.catalog.ReplaceReleaseTemplate(ctx, template, operator, version, requestKey, digest)
}

func (m *ReleaseTemplateManagement) SetEnabled(ctx context.Context, code string, enabled bool, version uint64, requestKey string) (domain.ReleaseTemplate, error) {
	operator, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return domain.ReleaseTemplate{}, err
	}
	if version == 0 || !templateCodePattern.MatchString(code) || !roleRequestKey.MatchString(requestKey) {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplate
	}
	action := "disable"
	if enabled {
		action = "enable"
	}
	digest, err := releaseTemplateDigest(action, code, version, domain.ReleaseTemplate{})
	if err != nil {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplate
	}
	return m.catalog.SetReleaseTemplateEnabled(ctx, code, enabled, operator, version, requestKey, digest)
}

func (m *ReleaseTemplateManagement) Delete(ctx context.Context, code string, version uint64, requestKey string) error {
	operator, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return err
	}
	if version == 0 || !templateCodePattern.MatchString(code) || !roleRequestKey.MatchString(requestKey) {
		return ErrInvalidReleaseTemplate
	}
	digest, err := releaseTemplateDigest("delete", code, version, domain.ReleaseTemplate{})
	if err != nil {
		return ErrInvalidReleaseTemplate
	}
	return m.catalog.DeleteReleaseTemplate(ctx, code, version, operator, requestKey, digest)
}

func validatedReleaseTemplate(candidate PutReleaseTemplate) (domain.ReleaseTemplate, error) {
	code := strings.TrimSpace(candidate.Code)
	name := strings.TrimSpace(candidate.Name)
	description := strings.TrimSpace(candidate.Description)
	if !templateCodePattern.MatchString(code) || name == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(description) > 500 {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplate
	}
	typeCode := domain.ReleaseType(strings.TrimSpace(candidate.Type))
	var expected []string
	switch typeCode {
	case domain.ReleaseTypeStandard:
		expected = []string{"APPROVAL", "PUBLICATION", "COMPLETION"}
	case domain.ReleaseTypeEmergency:
		expected = []string{"PUBLICATION", "COMPLETION"}
	default:
		return domain.ReleaseTemplate{}, ErrUnknownReleaseType
	}
	if len(candidate.Nodes) != len(expected) {
		return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplateNodes
	}
	nodes := make([]domain.ReleaseTemplateNode, len(candidate.Nodes))
	seen := make(map[string]struct{}, len(candidate.Nodes))
	for i, input := range candidate.Nodes {
		code := strings.TrimSpace(input.Code)
		nodeType := strings.TrimSpace(input.Type)
		name := strings.TrimSpace(input.Name)
		role := strings.TrimSpace(input.RequiredRole)
		if !templateNodeCodePattern.MatchString(code) || name == "" || utf8.RuneCountInString(name) > 100 || nodeType != expected[i] {
			return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplateNodes
		}
		if _, duplicate := seen[code]; duplicate {
			return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplateNodes
		}
		seen[code] = struct{}{}
		requiredRole := "PUBLISHER"
		if nodeType == "APPROVAL" {
			requiredRole = "TABLE_APPROVER"
		}
		if role != requiredRole {
			return domain.ReleaseTemplate{}, ErrInvalidReleaseTemplateNodes
		}
		nodes[i] = domain.ReleaseTemplateNode{Code: code, Type: nodeType, Name: name, RequiredRole: role}
	}
	return domain.ReleaseTemplate{Code: code, Name: name, Description: description, Type: typeCode, Nodes: nodes, MonitorList: []string{}, Enabled: true}, nil
}

func releaseTemplateDigest(action, target string, version uint64, template domain.ReleaseTemplate) (string, error) {
	payload, err := json.Marshal(struct {
		Action, Target          string
		ExpectedVersion         uint64
		Code, Name, Description string
		Type                    domain.ReleaseType
		Nodes                   []domain.ReleaseTemplateNode
		MonitorList             []string
	}{action, target, version, template.Code, template.Name, template.Description, template.Type, template.Nodes, template.MonitorList})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

var ErrReleaseTemplateExists = domain.ErrReleaseTemplateExists
var ErrReleaseTemplateNotFound = domain.ErrReleaseTemplateNotFound
var ErrReleaseTemplateVersionConflict = domain.ErrReleaseTemplateVersionConflict
var ErrReleaseTemplateIdempotencyConflict = domain.ErrReleaseTemplateIdempotencyConflict
var ErrEmergencyReleaseTemplateProtected = domain.ErrEmergencyReleaseTemplateProtected

type ReleaseTemplate = domain.ReleaseTemplate
type ReleaseTemplateNode = domain.ReleaseTemplateNode
