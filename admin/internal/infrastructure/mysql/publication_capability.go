package mysql

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	driver "github.com/go-sql-driver/mysql"
)

var publicationDictionaryName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func (s *publicationSession) checkPublicationCapability(ctx context.Context, plan application.PublicationTable) error {
	// Full InnoDB dictionary visibility is independent of per-schema SELECT.
	// Restricted deployments must grant PROCESS explicitly. Dictionary names
	// outside this verified, unencoded subset are rejected rather than guessed.
	var database string
	var checks, unique int
	if err := s.database.WithContext(ctx).Raw(`SELECT DATABASE(),@@session.foreign_key_checks,@@session.unique_checks`).Row().Scan(&database, &checks, &unique); err != nil {
		return application.ErrReleaseUnavailable
	}
	if !publicationDictionaryName.MatchString(database) || !publicationDictionaryName.MatchString(plan.Execution.TableName) || checks != 1 || unique != 1 {
		return application.ErrPublicationUnsupported
	}
	var references []struct {
		Parent string
		Type   uint64
	}
	if err := s.database.WithContext(ctx).Raw(`SELECT REF_NAME AS parent,TYPE AS type FROM information_schema.INNODB_FOREIGN`).Scan(&references).Error; err != nil {
		return publicationMetadataError(err, ctx.Err())
	}
	var name string
	if err := s.database.WithContext(ctx).Raw(`SELECT NAME FROM information_schema.INNODB_TABLES WHERE BINARY NAME=BINARY CONCAT(?,'/',?)`, database, plan.Execution.TableName).Row().Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return application.ErrPublicationUnsupported
		}
		return publicationMetadataError(err, ctx.Err())
	}
	for _, reference := range references {
		if reference.Parent == name && reference.Type&15 != 0 {
			return application.ErrPublicationUnsupported
		}
	}
	for _, section := range plan.Execution.Sections {
		if section.Name == "triggers" {
			for _, row := range section.Rows {
				if len(row) != 14 || row[4] == nil || row[6] == nil || *row[6] != "BEFORE" || !supportedTargetTrigger(*row[4], plan) {
					return application.ErrPublicationUnsupported
				}
			}
		}
	}
	return nil
}

func publicationMetadataError(err, contextErr error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(contextErr, context.DeadlineExceeded) {
		return application.ErrMutationTimeout
	}
	var mysqlError *driver.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1044, 1142, 1227:
			return application.ErrPublicationMetadataPermission
		}
	}
	return application.ErrReleaseUnavailable
}

// The accepted trigger language is deliberately closed: one BEFORE SET NEW
// assignment to a non-identity, non-audit column, with literals, row fields,
// arithmetic and a fixed list of pure builtins. It cannot call a routine,
// reference another table, assign a session variable or hide a second statement.
var triggerToken = regexp.MustCompile("(?is)^([a-z_][a-z0-9_]*|`[^`]+`|'(?:[^'\\\\]|'')*'|[0-9]+(?:\\.[0-9]+)?|[().,=+*/;-])")

type triggerExpression struct {
	tokens   []string
	position int
	columns  map[string]bool
}

func supportedTargetTrigger(body string, plan application.PublicationTable) bool {
	if len(body) > 8192 {
		return false
	}
	tokens := []string{}
	for body != "" {
		body = strings.TrimSpace(body)
		if body == "" {
			break
		}
		token := triggerToken.FindString(body)
		if token == "" {
			return false
		}
		tokens = append(tokens, token)
		body = body[len(token):]
	}
	columns := map[string]bool{}
	for _, c := range plan.Schema.Columns {
		columns[strings.ToLower(c.Name)] = true
	}
	p := triggerExpression{tokens: tokens, columns: columns}
	if !p.take("SET") || !p.take("NEW") || !p.take(".") {
		return false
	}
	target := p.identifier()
	if !columns[target] || target == "id" {
		return false
	}
	for _, c := range plan.Schema.Columns {
		if strings.EqualFold(c.Name, target) && c.Generated {
			return false
		}
	}
	for _, field := range []*string{plan.Policy.CreateOperatorField, plan.Policy.CreateTimeField, plan.Policy.ModifyOperatorField, plan.Policy.ModifyTimeField} {
		if field != nil && strings.EqualFold(*field, target) {
			return false
		}
	}
	if !p.take("=") || !p.expression(0) {
		return false
	}
	p.take(";")
	return p.position == len(tokens)
}
func (p *triggerExpression) take(want string) bool {
	if p.position < len(p.tokens) && strings.EqualFold(p.tokens[p.position], want) {
		p.position++
		return true
	}
	return false
}
func (p *triggerExpression) identifier() string {
	if p.position >= len(p.tokens) {
		return ""
	}
	value := p.tokens[p.position]
	if len(value) > 1 && value[0] == '`' {
		p.position++
		return strings.ToLower(value[1 : len(value)-1])
	}
	if (value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z') || value[0] == '_' {
		p.position++
		return strings.ToLower(value)
	}
	return ""
}
func (p *triggerExpression) expression(depth int) bool {
	if depth > 32 || !p.atom(depth) {
		return false
	}
	for p.position < len(p.tokens) {
		switch p.tokens[p.position] {
		case "+", "-", "*", "/":
			p.position++
			if !p.atom(depth) {
				return false
			}
		default:
			return true
		}
	}
	return true
}
func (p *triggerExpression) atom(depth int) bool {
	if depth > 32 || p.position >= len(p.tokens) {
		return false
	}
	if p.take("+") || p.take("-") {
		return p.atom(depth + 1)
	}
	if p.take("(") {
		return p.expression(depth+1) && p.take(")")
	}
	value := p.tokens[p.position]
	if value[0] == '\'' || value[0] >= '0' && value[0] <= '9' || p.take("NULL") {
		if !strings.EqualFold(value, "NULL") {
			p.position++
		}
		return true
	}
	name := p.identifier()
	if name == "new" || name == "old" {
		return p.take(".") && p.columns[p.identifier()]
	}
	switch name {
	case "concat", "lower", "upper", "coalesce", "ifnull", "abs", "round":
	default:
		return false
	}
	if !p.take("(") || !p.expression(depth+1) {
		return false
	}
	for p.take(",") {
		if !p.expression(depth + 1) {
			return false
		}
	}
	return p.take(")")
}
