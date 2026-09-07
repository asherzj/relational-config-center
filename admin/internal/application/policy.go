package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

var (
	ErrProtectedTable          = errors.New("protected table")
	ErrIncompatibleTable       = errors.New("incompatible table")
	ErrInvalidPolicyDefinition = errors.New("invalid Policy definition")
)

type CreateTablePolicy struct {
	TableName          string
	QueryPolicyCode    string
	MutationPolicyCode string
}

// TablePolicyManagement owns only the final Code-based assignment contract.
// Definitions and execution semantics remain in their dedicated modules.
type TablePolicyManagement struct {
	metadata         TableMetadataReader
	catalog          domain.TablePolicyCatalog
	queryPolicies    *QueryPolicyManagement
	mutationPolicies *MutationPolicyManagement
}

type activePolicyAssignmentCatalog interface {
	CreateWithActivePolicyCodes(context.Context, domain.TablePolicy, string) error
	ReplaceWithActivePolicyCodes(context.Context, domain.TablePolicy, string) (domain.TablePolicy, error)
}

func NewTablePolicyManagement(metadata TableMetadataReader, catalog domain.TablePolicyCatalog, queryPolicies *QueryPolicyManagement, mutationPolicies *MutationPolicyManagement) *TablePolicyManagement {
	return &TablePolicyManagement{
		metadata: metadata, catalog: catalog, queryPolicies: queryPolicies,
		mutationPolicies: mutationPolicies,
	}
}

func (management *TablePolicyManagement) Create(ctx context.Context, candidate CreateTablePolicy) (domain.TablePolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.TablePolicy{}, identityErr
	}
	candidate.TableName = strings.TrimSpace(candidate.TableName)
	candidate.QueryPolicyCode = strings.TrimSpace(candidate.QueryPolicyCode)
	candidate.MutationPolicyCode = strings.TrimSpace(candidate.MutationPolicyCode)
	if candidate.TableName == "" || candidate.QueryPolicyCode == "" || candidate.MutationPolicyCode == "" {
		return domain.TablePolicy{}, ErrInvalidPolicyDefinition
	}
	if protectedTable(candidate.TableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	schema, err := management.metadata.GetTableSchema(ctx, candidate.TableName)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if !schema.Compatible {
		return domain.TablePolicy{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
	}
	policy, err := management.assignmentFromActiveDefinitions(ctx, candidate.TableName, candidate.QueryPolicyCode, candidate.MutationPolicyCode, schema)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if catalog, ok := management.catalog.(activePolicyAssignmentCatalog); ok {
		if err := catalog.CreateWithActivePolicyCodes(ctx, policy, operator); err != nil {
			return domain.TablePolicy{}, err
		}
	} else if err := management.catalog.Create(ctx, policy, operator); err != nil {
		return domain.TablePolicy{}, err
	}
	return management.catalog.Get(ctx, policy.TableName)
}

func (management *TablePolicyManagement) List(ctx context.Context) ([]domain.TablePolicy, error) {
	return management.catalog.List(ctx)
}

func (management *TablePolicyManagement) Get(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	if protectedTable(tableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	return management.catalog.Get(ctx, tableName)
}

func (management *TablePolicyManagement) Replace(ctx context.Context, tableName string, candidate CreateTablePolicy) (domain.TablePolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.TablePolicy{}, identityErr
	}
	if protectedTable(tableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	if strings.TrimSpace(candidate.TableName) != tableName {
		return domain.TablePolicy{}, ErrInvalidPolicyDefinition
	}
	queryCode := strings.TrimSpace(candidate.QueryPolicyCode)
	mutationCode := strings.TrimSpace(candidate.MutationPolicyCode)
	if queryCode == "" || mutationCode == "" {
		return domain.TablePolicy{}, ErrInvalidPolicyDefinition
	}
	schema, err := management.metadata.GetTableSchema(ctx, tableName)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if !schema.Compatible {
		return domain.TablePolicy{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
	}
	policy, err := management.assignmentFromActiveDefinitions(ctx, tableName, queryCode, mutationCode, schema)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if catalog, ok := management.catalog.(activePolicyAssignmentCatalog); ok {
		return catalog.ReplaceWithActivePolicyCodes(ctx, policy, operator)
	}
	return management.catalog.Replace(ctx, policy, operator)
}

func (management *TablePolicyManagement) Enable(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.TablePolicy{}, identityErr
	}
	if protectedTable(tableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	policy, err := management.catalog.Get(ctx, tableName)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	schema, err := management.metadata.GetTableSchema(ctx, tableName)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if !schema.Compatible {
		return domain.TablePolicy{}, fmt.Errorf("%w: %s", ErrIncompatibleTable, *schema.IncompatibilityReason)
	}
	if err := management.validateExistingAssignment(ctx, policy, schema); err != nil {
		return domain.TablePolicy{}, err
	}
	return management.catalog.SetEnabled(ctx, tableName, true, operator)
}

func (management *TablePolicyManagement) Disable(ctx context.Context, tableName string) (domain.TablePolicy, error) {
	operator, identityErr := requestOperator(ctx)
	if identityErr != nil {
		return domain.TablePolicy{}, identityErr
	}
	if protectedTable(tableName) {
		return domain.TablePolicy{}, ErrProtectedTable
	}
	return management.catalog.SetEnabled(ctx, tableName, false, operator)
}

func (management *TablePolicyManagement) assignmentFromActiveDefinitions(ctx context.Context, tableName, queryCode, mutationCode string, schema domain.TableSchema) (domain.TablePolicy, error) {
	queryPolicy, err := management.queryPolicies.GetForNewAssignment(ctx, queryCode)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	mutationPolicy, err := management.mutationPolicies.GetForNewAssignment(ctx, mutationCode)
	if err != nil {
		return domain.TablePolicy{}, err
	}
	if err := management.queryPolicies.ValidateForTable(queryPolicy, schema); err != nil {
		return domain.TablePolicy{}, err
	}
	if err := management.mutationPolicies.ValidateForTable(mutationPolicy, schema); err != nil {
		return domain.TablePolicy{}, err
	}
	return domain.TablePolicy{TableName: tableName, QueryPolicyCode: queryCode, MutationPolicyCode: mutationCode}, nil
}

func (management *TablePolicyManagement) validateExistingAssignment(ctx context.Context, policy domain.TablePolicy, schema domain.TableSchema) error {
	queryPolicy, err := management.queryPolicies.Get(ctx, policy.QueryPolicyCode)
	if err != nil {
		return err
	}
	mutationPolicy, err := management.mutationPolicies.Get(ctx, policy.MutationPolicyCode)
	if err != nil {
		return err
	}
	if !runtimePolicyStatus(queryPolicy.Status) {
		return ErrQueryPolicyNotAssignable
	}
	if !runtimePolicyStatus(mutationPolicy.Status) {
		return ErrMutationPolicyNotAssignable
	}
	if err := management.queryPolicies.ValidateForTable(queryPolicy, schema); err != nil {
		return err
	}
	return management.mutationPolicies.ValidateForTable(mutationPolicy, schema)
}

func protectedTable(tableName string) bool {
	return strings.HasPrefix(strings.ToLower(tableName), "rcc_")
}
