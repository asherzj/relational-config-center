package http

import (
	"bytes"
	stdcontext "context"
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func NewRouter(discovery *application.DatabaseTableDiscovery, readiness application.Readiness, policies *application.TablePolicyManagement, queries *application.ManagedTableQuery, mutations *application.ManagedTableMutation, options RouterOptions) stdhttp.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(requestIdentity(), structuredAccessLog(options.AccessLog), safeRecovery(), limitRequestBody(), exactCORS(options), bearerAuthentication(options))
	router.NoRoute(func(context *gin.Context) {
		if isAPIRequest(context.Request.URL.Path) {
			writeError(context, stdhttp.StatusNotFound, "route_not_found", "route not found")
			return
		}
		context.Status(stdhttp.StatusNotFound)
	})
	router.NoMethod(func(context *gin.Context) {
		if isAPIRequest(context.Request.URL.Path) {
			writeError(context, stdhttp.StatusBadRequest, "method_not_allowed", "method is not allowed")
			return
		}
		context.Status(stdhttp.StatusMethodNotAllowed)
	})

	router.GET("/health/live", func(context *gin.Context) {
		context.JSON(stdhttp.StatusOK, gin.H{"status": "live"})
	})
	router.GET("/health/ready", func(context *gin.Context) {
		if err := readiness.Ready(context.Request.Context()); err != nil {
			context.JSON(stdhttp.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}
		context.JSON(stdhttp.StatusOK, gin.H{"status": "ready"})
	})

	router.GET("/api/v1/database-tables", func(context *gin.Context) {
		tables, err := discovery.List(context.Request.Context())
		if err != nil {
			writeError(context, stdhttp.StatusServiceUnavailable, "database_unavailable", "database table discovery is unavailable")
			return
		}
		response := make([]databaseTableResponse, 0, len(tables))
		for _, table := range tables {
			response = append(response, tableResponse(table))
		}
		context.JSON(stdhttp.StatusOK, gin.H{"tables": response})
	})

	router.GET("/api/v1/database-tables/:table_name", func(context *gin.Context) {
		table, err := discovery.Get(context.Request.Context(), context.Param("table_name"))
		if errors.Is(err, application.ErrDatabaseTableNotFound) {
			writeError(context, stdhttp.StatusNotFound, "database_table_not_found", "database table not found")
			return
		}
		if err != nil {
			writeError(context, stdhttp.StatusServiceUnavailable, "database_unavailable", "database table discovery is unavailable")
			return
		}
		context.JSON(stdhttp.StatusOK, tableResponse(table))
	})

	router.POST("/api/v1/table-policies", func(context *gin.Context) {
		var request createTablePolicyRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		policy, err := policies.Create(context.Request.Context(), application.CreateTablePolicy{
			TableName:            request.TableName,
			QueryPolicy:          request.QueryPolicy,
			QueryPolicyConfig:    request.QueryPolicyConfig,
			MutationPolicy:       request.MutationPolicy,
			MutationPolicyConfig: request.MutationPolicyConfig,
			AllowAdd:             request.AllowAdd,
			AllowModify:          request.AllowModify,
			AllowDelete:          request.AllowDelete,
		})
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusCreated, policyResponse(policy))
	})

	router.GET("/api/v1/table-policies", func(context *gin.Context) {
		policyList, err := policies.List(context.Request.Context())
		if writePolicyError(context, err) {
			return
		}
		response := make([]tablePolicyResponse, 0, len(policyList))
		for _, policy := range policyList {
			response = append(response, policyResponse(policy))
		}
		context.JSON(stdhttp.StatusOK, gin.H{"policies": response})
	})

	router.GET("/api/v1/table-policies/:table_name", func(context *gin.Context) {
		policy, err := policies.Get(context.Request.Context(), context.Param("table_name"))
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, policyResponse(policy))
	})

	router.PUT("/api/v1/table-policies/:table_name", func(context *gin.Context) {
		var request createTablePolicyRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		policy, err := policies.Replace(context.Request.Context(), context.Param("table_name"), application.CreateTablePolicy{
			TableName:            request.TableName,
			QueryPolicy:          request.QueryPolicy,
			QueryPolicyConfig:    request.QueryPolicyConfig,
			MutationPolicy:       request.MutationPolicy,
			MutationPolicyConfig: request.MutationPolicyConfig,
			AllowAdd:             request.AllowAdd,
			AllowModify:          request.AllowModify,
			AllowDelete:          request.AllowDelete,
		})
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, policyResponse(policy))
	})

	router.POST("/api/v1/table-policies/:table_name/enable", func(context *gin.Context) {
		policy, err := policies.Enable(context.Request.Context(), context.Param("table_name"))
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, policyResponse(policy))
	})

	router.POST("/api/v1/table-policies/:table_name/disable", func(context *gin.Context) {
		policy, err := policies.Disable(context.Request.Context(), context.Param("table_name"))
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, policyResponse(policy))
	})

	router.POST("/api/v1/tables/:table_name/query", func(context *gin.Context) {
		var request tableQueryRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		result, err := queries.Execute(context.Request.Context(), context.Param("table_name"), request.spec())
		if writeManagedQueryError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, queryResponse(result))
	})

	router.POST("/api/v1/tables/:table_name/rows", func(context *gin.Context) {
		var request tableAddRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		id, err := mutations.Add(context.Request.Context(), context.Param("table_name"), request.Content)
		if writeManagedMutationError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusCreated, gin.H{"id": id})
	})

	router.PATCH("/api/v1/tables/:table_name/rows/:id", func(context *gin.Context) {
		var request tablePatchRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		if request.Content == nil {
			writeError(context, stdhttp.StatusBadRequest, "invalid_request", "request body must be valid JSON with only supported fields")
			return
		}
		affected, err := mutations.Modify(context.Request.Context(), context.Param("table_name"), domain.JSONString(context.Param("id")), *request.Content)
		if writeManagedMutationError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, gin.H{"affected": affected})
	})

	router.DELETE("/api/v1/tables/:table_name/rows/:id", func(context *gin.Context) {
		affected, err := mutations.Delete(context.Request.Context(), context.Param("table_name"), domain.JSONString(context.Param("id")))
		if writeManagedMutationError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, gin.H{"affected": affected})
	})

	return router
}

type createTablePolicyRequest struct {
	TableName            string          `json:"table_name"`
	QueryPolicy          string          `json:"query_policy"`
	QueryPolicyConfig    json.RawMessage `json:"query_policy_config"`
	MutationPolicy       string          `json:"mutation_policy"`
	MutationPolicyConfig json.RawMessage `json:"mutation_policy_config"`
	AllowAdd             bool            `json:"allow_add"`
	AllowModify          bool            `json:"allow_modify"`
	AllowDelete          bool            `json:"allow_delete"`
}

type tablePolicyResponse struct {
	TableName            string          `json:"table_name"`
	QueryPolicy          string          `json:"query_policy"`
	QueryPolicyConfig    json.RawMessage `json:"query_policy_config"`
	MutationPolicy       string          `json:"mutation_policy"`
	MutationPolicyConfig json.RawMessage `json:"mutation_policy_config"`
	AllowAdd             bool            `json:"allow_add"`
	AllowModify          bool            `json:"allow_modify"`
	AllowDelete          bool            `json:"allow_delete"`
	Enabled              bool            `json:"enabled"`
}

type tableQueryRequest struct {
	Conditions []tableQueryCondition `json:"conditions,omitempty"`
	Order      *tableQueryOrder      `json:"order,omitempty"`
	PageNumber int                   `json:"page_number,omitempty"`
	PageSize   int                   `json:"page_size,omitempty"`
}

type tableAddRequest struct {
	Content domain.MutationContent `json:"content"`
}

type tablePatchRequest struct {
	Content *domain.MutationContent `json:"content"`
}

type tableQueryCondition struct {
	Field    string                  `json:"field"`
	Operator string                  `json:"operator"`
	Value    optionalJSONString      `json:"value"`
	From     optionalJSONString      `json:"from"`
	To       optionalJSONString      `json:"to"`
	Values   optionalJSONStringArray `json:"values"`
}

type optionalJSONString struct {
	Present bool
	Value   *string
}

func (value *optionalJSONString) UnmarshalJSON(data []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		value.Value = nil
		return nil
	}
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = &decoded
	return nil
}

type optionalJSONStringArray struct {
	Present bool
	Value   []*string
}

func (values *optionalJSONStringArray) UnmarshalJSON(data []byte) error {
	values.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		values.Value = nil
		return nil
	}
	return json.Unmarshal(data, &values.Value)
}

type tableQueryOrder struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

func (request tableQueryRequest) spec() domain.QuerySpec {
	conditions := make([]domain.QueryCondition, 0, len(request.Conditions))
	for _, condition := range request.Conditions {
		var value *domain.JSONString
		if condition.Value.Value != nil {
			converted := domain.JSONString(*condition.Value.Value)
			value = &converted
		}
		var from *domain.JSONString
		if condition.From.Value != nil {
			converted := domain.JSONString(*condition.From.Value)
			from = &converted
		}
		var to *domain.JSONString
		if condition.To.Value != nil {
			converted := domain.JSONString(*condition.To.Value)
			to = &converted
		}
		var values []*domain.JSONString
		if condition.Values.Value != nil {
			values = make([]*domain.JSONString, 0, len(condition.Values.Value))
			for _, item := range condition.Values.Value {
				if item == nil {
					values = append(values, nil)
					continue
				}
				converted := domain.JSONString(*item)
				values = append(values, &converted)
			}
		}
		conditions = append(conditions, domain.QueryCondition{
			Field:         condition.Field,
			Operator:      domain.QueryOperator(condition.Operator),
			Value:         value,
			ValuePresent:  condition.Value.Present,
			From:          from,
			FromPresent:   condition.From.Present,
			To:            to,
			ToPresent:     condition.To.Present,
			Values:        values,
			ValuesPresent: condition.Values.Present,
		})
	}
	var order *domain.QueryOrder
	if request.Order != nil {
		order = &domain.QueryOrder{Field: request.Order.Field, Direction: request.Order.Direction}
	}
	return domain.QuerySpec{
		Conditions: conditions,
		Order:      order,
		PageNumber: request.PageNumber,
		PageSize:   request.PageSize,
	}
}

type tableQueryResponse struct {
	Columns []tableQueryColumnResponse `json:"columns"`
	Rows    []map[string]*string       `json:"rows"`
	Page    tableQueryPageResponse     `json:"page"`
}

type tableQueryColumnResponse struct {
	Name     string            `json:"name"`
	Type     domain.ColumnType `json:"type"`
	Nullable bool              `json:"nullable"`
}

type tableQueryPageResponse struct {
	PageNumber int   `json:"page_number"`
	PageSize   int   `json:"page_size"`
	TotalCount int64 `json:"total_count"`
	TotalPages int64 `json:"total_pages"`
}

func queryResponse(result domain.QueryResult) tableQueryResponse {
	columns := make([]tableQueryColumnResponse, 0, len(result.Columns))
	for _, column := range result.Columns {
		columns = append(columns, tableQueryColumnResponse{Name: column.Name, Type: column.Type, Nullable: column.Nullable})
	}
	rows := make([]map[string]*string, 0, len(result.Rows))
	for _, resultRow := range result.Rows {
		row := make(map[string]*string, len(resultRow))
		for name, cell := range resultRow {
			if cell == nil {
				row[name] = nil
				continue
			}
			value := string(*cell)
			row[name] = &value
		}
		rows = append(rows, row)
	}
	return tableQueryResponse{
		Columns: columns,
		Rows:    rows,
		Page: tableQueryPageResponse{
			PageNumber: result.Page.PageNumber,
			PageSize:   result.Page.PageSize,
			TotalCount: result.Page.TotalCount,
			TotalPages: result.Page.TotalPages,
		},
	}
}

func policyResponse(policy domain.TablePolicy) tablePolicyResponse {
	return tablePolicyResponse{
		TableName:            policy.TableName,
		QueryPolicy:          policy.QueryPolicy,
		QueryPolicyConfig:    json.RawMessage(policy.QueryPolicyConfig),
		MutationPolicy:       policy.MutationPolicy,
		MutationPolicyConfig: json.RawMessage(policy.MutationPolicyConfig),
		AllowAdd:             policy.AllowAdd,
		AllowModify:          policy.AllowModify,
		AllowDelete:          policy.AllowDelete,
		Enabled:              policy.Enabled,
	}
}

func decodeRequest(context *gin.Context, destination any) error {
	decoder := json.NewDecoder(context.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request contains trailing JSON")
	}
	return nil
}

func writeRequestDecodeError(context *gin.Context, err error) {
	var maximumBytesError *stdhttp.MaxBytesError
	if errors.As(err, &maximumBytesError) {
		writeError(context, stdhttp.StatusBadRequest, "request_body_too_large", "request body exceeds the 1 MiB limit")
		return
	}
	writeError(context, stdhttp.StatusBadRequest, "invalid_request", "request body must be valid JSON with only supported fields")
}

func writePolicyError(context *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, application.ErrInvalidPolicyDefinition):
		writeError(context, stdhttp.StatusBadRequest, "invalid_policy_definition", "Table Policy definition is incomplete")
	case errors.Is(err, application.ErrProtectedTable):
		writeError(context, stdhttp.StatusForbidden, "protected_table", "protected tables cannot have a Table Policy")
	case errors.Is(err, application.ErrDatabaseTableNotFound):
		writeError(context, stdhttp.StatusNotFound, "database_table_not_found", "database table not found")
	case errors.Is(err, domain.ErrTablePolicyNotFound):
		writeError(context, stdhttp.StatusNotFound, "table_policy_not_found", "Table Policy not found")
	case errors.Is(err, domain.ErrTablePolicyExists):
		writeError(context, stdhttp.StatusConflict, "table_policy_exists", "Table Policy already exists")
	case errors.Is(err, application.ErrIncompatibleTable):
		writeError(context, stdhttp.StatusUnprocessableEntity, "incompatible_table", "database table is incompatible with Table Policy management")
	case errors.Is(err, application.ErrUnknownQueryStrategy), errors.Is(err, application.ErrUnknownMutationStrategy):
		writeError(context, stdhttp.StatusUnprocessableEntity, "unknown_policy_strategy", "Table Policy references an unknown strategy")
	case errors.Is(err, application.ErrInvalidPolicyConfig):
		writeError(context, stdhttp.StatusUnprocessableEntity, "invalid_policy_config", "Table Policy configuration is invalid")
	default:
		writeError(context, stdhttp.StatusServiceUnavailable, "policy_catalog_unavailable", "Policy Catalog is unavailable")
	}
	return true
}

func writeManagedQueryError(context *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, application.ErrProtectedTable):
		writeError(context, stdhttp.StatusForbidden, "protected_table", "protected tables cannot be queried")
	case errors.Is(err, domain.ErrTablePolicyNotFound):
		writeError(context, stdhttp.StatusNotFound, "table_policy_not_found", "Table Policy not found")
	case errors.Is(err, application.ErrTablePolicyDisabled):
		writeError(context, stdhttp.StatusForbidden, "table_policy_disabled", "Table Policy is disabled")
	case errors.Is(err, application.ErrDatabaseTableNotFound):
		writeError(context, stdhttp.StatusNotFound, "database_table_not_found", "database table not found")
	case errors.Is(err, application.ErrInvalidQueryCondition):
		writeError(context, stdhttp.StatusBadRequest, "invalid_query_condition", "query condition is invalid")
	case errors.Is(err, application.ErrInvalidQueryOrder):
		writeError(context, stdhttp.StatusBadRequest, "invalid_query_order", "query order is invalid")
	case errors.Is(err, application.ErrInvalidPagination):
		writeError(context, stdhttp.StatusBadRequest, "invalid_pagination", "query pagination is invalid")
	case errors.Is(err, application.ErrUnknownQueryStrategy), errors.Is(err, application.ErrUnknownMutationStrategy):
		writeError(context, stdhttp.StatusUnprocessableEntity, "unknown_policy_strategy", "Table Policy references an unknown strategy")
	case errors.Is(err, application.ErrInvalidPolicyConfig):
		writeError(context, stdhttp.StatusUnprocessableEntity, "invalid_policy_config", "Table Policy configuration is invalid")
	case errors.Is(err, application.ErrIncompatibleTable):
		writeError(context, stdhttp.StatusUnprocessableEntity, "incompatible_table", "database table is incompatible with Table Policy management")
	case errors.Is(err, application.ErrPolicyCatalogUnavailable):
		writeError(context, stdhttp.StatusServiceUnavailable, "policy_catalog_unavailable", "Policy Catalog is unavailable")
	case errors.Is(err, application.ErrQueryTimeout), errors.Is(err, stdcontext.DeadlineExceeded):
		writeError(context, stdhttp.StatusGatewayTimeout, "query_timeout", "Managed Table query timed out")
	case errors.Is(err, application.ErrQueryUnavailable):
		writeError(context, stdhttp.StatusServiceUnavailable, "query_unavailable", "Managed Table query is unavailable")
	default:
		writeError(context, stdhttp.StatusInternalServerError, "internal_error", "internal server error")
	}
	return true
}

func writeManagedMutationError(context *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, application.ErrProtectedTable):
		writeError(context, stdhttp.StatusForbidden, "protected_table", "protected tables cannot be mutated")
	case errors.Is(err, domain.ErrTablePolicyNotFound):
		writeError(context, stdhttp.StatusNotFound, "table_policy_not_found", "Table Policy not found")
	case errors.Is(err, application.ErrTablePolicyDisabled):
		writeError(context, stdhttp.StatusForbidden, "table_policy_disabled", "Table Policy is disabled")
	case errors.Is(err, application.ErrMutationNotAllowed):
		writeError(context, stdhttp.StatusForbidden, "mutation_not_allowed", "Mutation Policy does not allow the requested operation")
	case errors.Is(err, application.ErrMutationRowNotFound):
		writeError(context, stdhttp.StatusNotFound, "mutation_row_not_found", "Managed Table row not found")
	case errors.Is(err, application.ErrDatabaseTableNotFound):
		writeError(context, stdhttp.StatusNotFound, "database_table_not_found", "database table not found")
	case errors.Is(err, application.ErrInvalidMutation):
		writeError(context, stdhttp.StatusBadRequest, "invalid_mutation_content", "mutation content is invalid")
	case errors.Is(err, application.ErrMissingRequiredField):
		writeError(context, stdhttp.StatusBadRequest, "missing_required_field", "required mutation field is missing")
	case errors.Is(err, application.ErrDuplicateKey):
		writeError(context, stdhttp.StatusConflict, "duplicate_key", "a row with the same unique key already exists")
	case errors.Is(err, application.ErrUnknownQueryStrategy), errors.Is(err, application.ErrUnknownMutationStrategy):
		writeError(context, stdhttp.StatusUnprocessableEntity, "unknown_policy_strategy", "Table Policy references an unknown strategy")
	case errors.Is(err, application.ErrInvalidPolicyConfig):
		writeError(context, stdhttp.StatusUnprocessableEntity, "invalid_policy_config", "Table Policy configuration is invalid")
	case errors.Is(err, application.ErrIncompatibleTable):
		writeError(context, stdhttp.StatusUnprocessableEntity, "incompatible_table", "database table is incompatible with Table Policy management")
	case errors.Is(err, application.ErrPolicyCatalogUnavailable):
		writeError(context, stdhttp.StatusServiceUnavailable, "policy_catalog_unavailable", "Policy Catalog is unavailable")
	case errors.Is(err, application.ErrMutationTimeout), errors.Is(err, stdcontext.DeadlineExceeded):
		writeError(context, stdhttp.StatusGatewayTimeout, "mutation_timeout", "Managed Table mutation timed out")
	case errors.Is(err, application.ErrMutationUnavailable):
		writeError(context, stdhttp.StatusServiceUnavailable, "mutation_unavailable", "Managed Table mutation is unavailable")
	default:
		writeError(context, stdhttp.StatusInternalServerError, "internal_error", "internal server error")
	}
	return true
}

type databaseTableResponse struct {
	TableName             string                        `json:"table_name"`
	TableComment          string                        `json:"table_comment"`
	PolicyExists          bool                          `json:"policy_exists"`
	PolicyEnabled         bool                          `json:"policy_enabled"`
	Compatible            bool                          `json:"compatible"`
	IncompatibilityReason *domain.IncompatibilityReason `json:"incompatibility_reason"`
}

func tableResponse(table domain.DatabaseTable) databaseTableResponse {
	return databaseTableResponse{
		TableName:             table.Name,
		TableComment:          table.Comment,
		PolicyExists:          table.PolicyExists,
		PolicyEnabled:         table.PolicyEnabled,
		Compatible:            table.Compatible,
		IncompatibilityReason: table.IncompatibilityReason,
	}
}

func writeError(context *gin.Context, status int, code, message string) {
	context.JSON(status, gin.H{"error": gin.H{"code": code, "message": message, "request_id": requestID(context)}})
}
