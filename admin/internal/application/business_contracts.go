package application

import "github.com/asherzj/relational-config-center/admin/internal/domain"

// Business use-case inputs and results are exposed at the Application boundary.
// HTTP converts these contracts to transport DTOs without importing Domain.
type ColumnType = domain.ColumnType
type DatabaseTable = domain.DatabaseTable

var ErrMutationPolicyExists = domain.ErrMutationPolicyExists
var ErrMutationPolicyNotFound = domain.ErrMutationPolicyNotFound
var ErrMutationPolicyStateConflict = domain.ErrMutationPolicyStateConflict
var ErrQueryPolicyExists = domain.ErrQueryPolicyExists
var ErrQueryPolicyNotFound = domain.ErrQueryPolicyNotFound
var ErrQueryPolicyStateConflict = domain.ErrQueryPolicyStateConflict
var ErrTablePolicyExists = domain.ErrTablePolicyExists
var ErrTablePolicyNotFound = domain.ErrTablePolicyNotFound

type IncompatibilityReason = domain.IncompatibilityReason
type JSONString = domain.JSONString
type MutationContent = domain.MutationContent
type MutationPolicy = domain.MutationPolicy
type PolicyStatus = domain.PolicyStatus
type QueryCondition = domain.QueryCondition
type QueryOperator = domain.QueryOperator
type QueryOrder = domain.QueryOrder
type QueryPolicy = domain.QueryPolicy
type QueryResult = domain.QueryResult
type QuerySpec = domain.QuerySpec
type TablePolicy = domain.TablePolicy
