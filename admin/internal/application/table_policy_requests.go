package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var ErrTablePolicyVersionConflict = errors.New("table policy changed")
var ErrInvalidTablePolicyRequest = errors.New("invalid table policy request")

// A request session gives validation and persistence the same transaction.
type TablePolicyRequestSession interface {
	TableMetadataReader
	domain.TablePolicyCatalog
	domain.QueryPolicyCatalog
	domain.MutationPolicyCatalog
}
type TablePolicyRequestStore interface {
	ExecuteTablePolicyRequest(context.Context, string, string, string, string, string, uint64, func(TablePolicyRequestSession) (domain.TablePolicy, error)) (domain.TablePolicy, error)
}

func (m *TablePolicyManagement) Execute(ctx context.Context, action, table string, candidate CreateTablePolicy, version uint64, key string) (domain.TablePolicy, error) {
	actor, err := requireRole(ctx, RoleAdmin)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if protectedTable(table) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	if !roleRequestKey.MatchString(key) || (action != "create" && version == 0) {
		return domain.TablePolicy{}, ErrInvalidTablePolicyRequest
	}
	encoded, _ := json.Marshal(struct {
		Action, Table string
		Candidate     CreateTablePolicy
		Version       uint64
	}{action, table, candidate, version})
	digest := sha256.Sum256(encoded)
	store, ok := m.catalog.(TablePolicyRequestStore)
	if !ok {
		return domain.TablePolicy{}, errors.New("table policy request store unavailable")
	}
	return store.ExecuteTablePolicyRequest(ctx, actor, action, table, key, hex.EncodeToString(digest[:]), version, func(session TablePolicyRequestSession) (domain.TablePolicy, error) {
		management := NewTablePolicyManagement(session, session, NewQueryPolicyManagement(session, m.queryPolicies.registry), NewMutationPolicyManagement(session, m.mutationPolicies.registry))
		switch action {
		case "create":
			return management.Create(ctx, candidate)
		case "replace":
			return management.Replace(ctx, table, candidate)
		case "enable":
			return management.Enable(ctx, table)
		case "disable":
			return management.Disable(ctx, table)
		default:
			return domain.TablePolicy{}, ErrInvalidTablePolicyRequest
		}
	})
}
