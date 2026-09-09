package http

import (
	"bytes"
	stdcontext "context"
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/asherzj/relational-config-center/admin/internal/application"
)

func NewRouter(discovery *application.DatabaseTableDiscovery, readiness application.Readiness, queryPolicies *application.QueryPolicyManagement, mutationPolicies *application.MutationPolicyManagement, policies *application.TablePolicyManagement, queries *application.ManagedTableQuery, options RouterOptions) stdhttp.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.UseRawPath = true
	router.UnescapePathValues = true
	router.Use(requestIdentity(), structuredAccessLog(options.AccessLog), safeRecovery(), limitRequestBody(), publicationDeadline(options.PublicationTimeout), sessionAuthentication(options))
	if options.Authentication != nil {
		registerAccountRoutes(router, options.Authentication, options.AccountHTTP)
	}
	if options.AccountRoles != nil {
		registerAccountRoleRoutes(router, options.AccountRoles)
	}
	if options.ReleaseOrders != nil {
		registerReleaseOrderRoutes(router, options.ReleaseOrders)
	}
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

	registerQueryPolicyRoutes(router, queryPolicies)
	registerMutationPolicyRoutes(router, mutationPolicies)

	router.POST("/api/v1/table-policies", func(context *gin.Context) {
		candidate, decodeErr := decodeTablePolicyCandidate(context)
		if decodeErr != nil {
			writeRequestDecodeError(context, decodeErr)
			return
		}
		policy, err := policies.Create(context.Request.Context(), candidate)
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusCreated, assignmentPolicyResponse(policy))
	})

	router.GET("/api/v1/table-policies", func(context *gin.Context) {
		policyList, err := policies.List(context.Request.Context())
		if writePolicyError(context, err) {
			return
		}
		response := make([]tablePolicyAssignmentResponse, 0, len(policyList))
		for _, policy := range policyList {
			response = append(response, assignmentPolicyResponse(policy))
		}
		context.JSON(stdhttp.StatusOK, gin.H{"policies": response})
	})

	router.GET("/api/v1/table-policies/:table_name", func(context *gin.Context) {
		policy, err := policies.Get(context.Request.Context(), context.Param("table_name"))
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, assignmentPolicyResponse(policy))
	})

	router.PUT("/api/v1/table-policies/:table_name", func(context *gin.Context) {
		candidate, decodeErr := decodeTablePolicyCandidate(context)
		if decodeErr != nil {
			writeRequestDecodeError(context, decodeErr)
			return
		}
		policy, err := policies.Replace(context.Request.Context(), context.Param("table_name"), candidate)
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, assignmentPolicyResponse(policy))
	})

	router.POST("/api/v1/table-policies/:table_name/enable", func(context *gin.Context) {
		policy, err := policies.Enable(context.Request.Context(), context.Param("table_name"))
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, assignmentPolicyResponse(policy))
	})

	router.POST("/api/v1/table-policies/:table_name/disable", func(context *gin.Context) {
		policy, err := policies.Disable(context.Request.Context(), context.Param("table_name"))
		if writePolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, assignmentPolicyResponse(policy))
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

	return router
}

func registerQueryPolicyRoutes(router *gin.Engine, policies *application.QueryPolicyManagement) {
	router.GET("/api/v1/query-policy-types", func(context *gin.Context) {
		types := policies.Types()
		response := make([]queryPolicyTypeResponse, 0, len(types))
		for _, policyType := range types {
			response = append(response, queryPolicyTypeResponse{Code: policyType.Code})
		}
		context.JSON(stdhttp.StatusOK, gin.H{"types": response})
	})

	router.POST("/api/v1/query-policies", func(context *gin.Context) {
		var request putQueryPolicyRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		policy, err := policies.Create(context.Request.Context(), request.candidate())
		if writeQueryPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusCreated, queryPolicyResponseFor(policy))
	})

	router.GET("/api/v1/query-policies", func(context *gin.Context) {
		policyList, err := policies.List(context.Request.Context())
		if writeQueryPolicyError(context, err) {
			return
		}
		response := make([]queryPolicyResponse, 0, len(policyList))
		for _, policy := range policyList {
			response = append(response, queryPolicyResponseFor(policy))
		}
		context.JSON(stdhttp.StatusOK, gin.H{"policies": response})
	})

	router.GET("/api/v1/query-policies/:code", func(context *gin.Context) {
		policy, err := policies.Get(context.Request.Context(), context.Param("code"))
		if writeQueryPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, queryPolicyResponseFor(policy))
	})

	router.PUT("/api/v1/query-policies/:code", func(context *gin.Context) {
		var request putQueryPolicyRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		policy, err := policies.ReplaceDraft(context.Request.Context(), context.Param("code"), request.candidate())
		if writeQueryPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, queryPolicyResponseFor(policy))
	})

	router.POST("/api/v1/query-policies/:code/activate", func(context *gin.Context) {
		policy, err := policies.Activate(context.Request.Context(), context.Param("code"))
		if writeQueryPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, queryPolicyResponseFor(policy))
	})

	router.POST("/api/v1/query-policies/:code/deprecate", func(context *gin.Context) {
		policy, err := policies.Deprecate(context.Request.Context(), context.Param("code"))
		if writeQueryPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, queryPolicyResponseFor(policy))
	})

	router.PATCH("/api/v1/query-policies/:code/metadata", func(context *gin.Context) {
		var request updateQueryPolicyMetadataRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		policy, err := policies.UpdateMetadata(context.Request.Context(), context.Param("code"), request.Name, request.Description)
		if writeQueryPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, queryPolicyResponseFor(policy))
	})

	router.DELETE("/api/v1/query-policies/:code", func(context *gin.Context) {
		if writeQueryPolicyError(context, policies.DeleteDraft(context.Request.Context(), context.Param("code"))) {
			return
		}
		context.Status(stdhttp.StatusNoContent)
	})
}

func registerMutationPolicyRoutes(router *gin.Engine, policies *application.MutationPolicyManagement) {
	router.GET("/api/v1/mutation-policy-types", func(context *gin.Context) {
		types := policies.Types()
		response := make([]mutationPolicyTypeResponse, 0, len(types))
		for _, policyType := range types {
			response = append(response, mutationPolicyTypeResponse{Code: policyType.Code, Operations: policyType.Operations})
		}
		context.JSON(stdhttp.StatusOK, gin.H{"types": response})
	})

	router.POST("/api/v1/mutation-policies", func(context *gin.Context) {
		var request putMutationPolicyRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		policy, err := policies.Create(context.Request.Context(), request.candidate())
		if writeMutationPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusCreated, mutationPolicyResponseFor(policy))
	})

	router.GET("/api/v1/mutation-policies", func(context *gin.Context) {
		policyList, err := policies.List(context.Request.Context())
		if writeMutationPolicyError(context, err) {
			return
		}
		response := make([]mutationPolicyResponse, 0, len(policyList))
		for _, policy := range policyList {
			response = append(response, mutationPolicyResponseFor(policy))
		}
		context.JSON(stdhttp.StatusOK, gin.H{"policies": response})
	})

	router.GET("/api/v1/mutation-policies/:code", func(context *gin.Context) {
		policy, err := policies.Get(context.Request.Context(), context.Param("code"))
		if writeMutationPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, mutationPolicyResponseFor(policy))
	})

	router.PUT("/api/v1/mutation-policies/:code", func(context *gin.Context) {
		var request putMutationPolicyRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		policy, err := policies.ReplaceDraft(context.Request.Context(), context.Param("code"), request.candidate())
		if writeMutationPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, mutationPolicyResponseFor(policy))
	})

	router.POST("/api/v1/mutation-policies/:code/activate", func(context *gin.Context) {
		policy, err := policies.Activate(context.Request.Context(), context.Param("code"))
		if writeMutationPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, mutationPolicyResponseFor(policy))
	})

	router.POST("/api/v1/mutation-policies/:code/deprecate", func(context *gin.Context) {
		policy, err := policies.Deprecate(context.Request.Context(), context.Param("code"))
		if writeMutationPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, mutationPolicyResponseFor(policy))
	})

	router.PATCH("/api/v1/mutation-policies/:code/metadata", func(context *gin.Context) {
		var request updateMutationPolicyMetadataRequest
		if err := decodeRequest(context, &request); err != nil {
			writeRequestDecodeError(context, err)
			return
		}
		policy, err := policies.UpdateMetadata(context.Request.Context(), context.Param("code"), request.Name, request.Description)
		if writeMutationPolicyError(context, err) {
			return
		}
		context.JSON(stdhttp.StatusOK, mutationPolicyResponseFor(policy))
	})

	router.DELETE("/api/v1/mutation-policies/:code", func(context *gin.Context) {
		if writeMutationPolicyError(context, policies.DeleteDraft(context.Request.Context(), context.Param("code"))) {
			return
		}
		context.Status(stdhttp.StatusNoContent)
	})
}

type assignTablePolicyRequest struct {
	TableName          string `json:"table_name"`
	QueryPolicyCode    string `json:"query_policy_code"`
	MutationPolicyCode string `json:"mutation_policy_code"`
}

func decodeTablePolicyCandidate(context *gin.Context) (application.CreateTablePolicy, error) {
	var request assignTablePolicyRequest
	if err := decodeRequest(context, &request); err != nil {
		return application.CreateTablePolicy{}, err
	}
	return application.CreateTablePolicy{TableName: request.TableName, QueryPolicyCode: request.QueryPolicyCode, MutationPolicyCode: request.MutationPolicyCode}, nil
}

type putQueryPolicyRequest struct {
	Code                  string `json:"code"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	TypeCode              string `json:"type_code"`
	DefaultOrderField     string `json:"default_order_field"`
	DefaultOrderDirection string `json:"default_order_direction"`
	DefaultPageSize       int    `json:"default_page_size"`
	MaxPageSize           int    `json:"max_page_size"`
}

func (request putQueryPolicyRequest) candidate() application.PutQueryPolicy {
	return application.PutQueryPolicy{
		Code: request.Code, Name: request.Name, Description: request.Description, TypeCode: request.TypeCode,
		DefaultOrderField: request.DefaultOrderField, DefaultOrderDirection: request.DefaultOrderDirection,
		DefaultPageSize: request.DefaultPageSize, MaxPageSize: request.MaxPageSize,
	}
}

type updateQueryPolicyMetadataRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type queryPolicyTypeResponse struct {
	Code string `json:"code"`
}

type queryPolicyResponse struct {
	Code                  string                   `json:"code"`
	Name                  string                   `json:"name"`
	Description           string                   `json:"description"`
	TypeCode              string                   `json:"type_code"`
	DefaultOrderField     string                   `json:"default_order_field"`
	DefaultOrderDirection string                   `json:"default_order_direction"`
	DefaultPageSize       int                      `json:"default_page_size"`
	MaxPageSize           int                      `json:"max_page_size"`
	Status                application.PolicyStatus `json:"status"`
	Creator               string                   `json:"creator"`
	Modifier              string                   `json:"modifier"`
	CreatedAt             string                   `json:"created_at"`
	UpdatedAt             string                   `json:"updated_at"`
}

type putMutationPolicyRequest struct {
	Code                string  `json:"code"`
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	TypeCode            string  `json:"type_code"`
	AllowAdd            bool    `json:"allow_add"`
	AllowModify         bool    `json:"allow_modify"`
	AllowDelete         bool    `json:"allow_delete"`
	CreateOperatorField *string `json:"create_operator_field"`
	CreateTimeField     *string `json:"create_time_field"`
	ModifyOperatorField *string `json:"modify_operator_field"`
	ModifyTimeField     *string `json:"modify_time_field"`
}

func (request putMutationPolicyRequest) candidate() application.PutMutationPolicy {
	return application.PutMutationPolicy{
		Code: request.Code, Name: request.Name, Description: request.Description, TypeCode: request.TypeCode,
		AllowAdd: request.AllowAdd, AllowModify: request.AllowModify, AllowDelete: request.AllowDelete,
		CreateOperatorField: request.CreateOperatorField, CreateTimeField: request.CreateTimeField,
		ModifyOperatorField: request.ModifyOperatorField, ModifyTimeField: request.ModifyTimeField,
	}
}

type updateMutationPolicyMetadataRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type mutationPolicyTypeResponse struct {
	Code       string                          `json:"code"`
	Operations []application.MutationOperation `json:"operations"`
}

type mutationPolicyResponse struct {
	Code                string                   `json:"code"`
	Name                string                   `json:"name"`
	Description         string                   `json:"description"`
	TypeCode            string                   `json:"type_code"`
	AllowAdd            bool                     `json:"allow_add"`
	AllowModify         bool                     `json:"allow_modify"`
	AllowDelete         bool                     `json:"allow_delete"`
	CreateOperatorField *string                  `json:"create_operator_field"`
	CreateTimeField     *string                  `json:"create_time_field"`
	ModifyOperatorField *string                  `json:"modify_operator_field"`
	ModifyTimeField     *string                  `json:"modify_time_field"`
	Status              application.PolicyStatus `json:"status"`
	Creator             string                   `json:"creator"`
	Modifier            string                   `json:"modifier"`
	CreatedAt           string                   `json:"created_at"`
	UpdatedAt           string                   `json:"updated_at"`
}

func mutationPolicyResponseFor(policy application.MutationPolicy) mutationPolicyResponse {
	return mutationPolicyResponse{
		Code: policy.Code, Name: policy.Name, Description: policy.Description, TypeCode: policy.TypeCode,
		AllowAdd: policy.AllowAdd, AllowModify: policy.AllowModify, AllowDelete: policy.AllowDelete,
		CreateOperatorField: policy.CreateOperatorField, CreateTimeField: policy.CreateTimeField,
		ModifyOperatorField: policy.ModifyOperatorField, ModifyTimeField: policy.ModifyTimeField,
		Status: policy.Status, Creator: policy.Creator, Modifier: policy.Modifier,
		CreatedAt: policy.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: policy.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func queryPolicyResponseFor(policy application.QueryPolicy) queryPolicyResponse {
	return queryPolicyResponse{
		Code: policy.Code, Name: policy.Name, Description: policy.Description, TypeCode: policy.TypeCode,
		DefaultOrderField: policy.DefaultOrderField, DefaultOrderDirection: policy.DefaultOrderDirection,
		DefaultPageSize: policy.DefaultPageSize, MaxPageSize: policy.MaxPageSize, Status: policy.Status,
		Creator: policy.Creator, Modifier: policy.Modifier,
		CreatedAt: policy.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: policy.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

type tablePolicyAssignmentResponse struct {
	TableName          string `json:"table_name"`
	QueryPolicyCode    string `json:"query_policy_code"`
	MutationPolicyCode string `json:"mutation_policy_code"`
	Enabled            bool   `json:"enabled"`
	Creator            string `json:"creator"`
	Modifier           string `json:"modifier"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type tableQueryRequest struct {
	Conditions []tableQueryCondition `json:"conditions,omitempty"`
	Order      *tableQueryOrder      `json:"order,omitempty"`
	PageNumber int                   `json:"page_number,omitempty"`
	PageSize   int                   `json:"page_size,omitempty"`
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

func (request tableQueryRequest) spec() application.QuerySpec {
	conditions := make([]application.QueryCondition, 0, len(request.Conditions))
	for _, condition := range request.Conditions {
		var value *application.JSONString
		if condition.Value.Value != nil {
			converted := application.JSONString(*condition.Value.Value)
			value = &converted
		}
		var from *application.JSONString
		if condition.From.Value != nil {
			converted := application.JSONString(*condition.From.Value)
			from = &converted
		}
		var to *application.JSONString
		if condition.To.Value != nil {
			converted := application.JSONString(*condition.To.Value)
			to = &converted
		}
		var values []*application.JSONString
		if condition.Values.Value != nil {
			values = make([]*application.JSONString, 0, len(condition.Values.Value))
			for _, item := range condition.Values.Value {
				if item == nil {
					values = append(values, nil)
					continue
				}
				converted := application.JSONString(*item)
				values = append(values, &converted)
			}
		}
		conditions = append(conditions, application.QueryCondition{
			Field:         condition.Field,
			Operator:      application.QueryOperator(condition.Operator),
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
	var order *application.QueryOrder
	if request.Order != nil {
		order = &application.QueryOrder{Field: request.Order.Field, Direction: request.Order.Direction}
	}
	return application.QuerySpec{
		Conditions: conditions,
		Order:      order,
		PageNumber: request.PageNumber,
		PageSize:   request.PageSize,
	}
}

type tableQueryResponse struct {
	RecordVersions []string                   `json:"record_versions"`
	Columns        []tableQueryColumnResponse `json:"columns"`
	Rows           []map[string]*string       `json:"rows"`
	Page           tableQueryPageResponse     `json:"page"`
}

type tableQueryColumnResponse struct {
	Name     string                 `json:"name"`
	Type     application.ColumnType `json:"type"`
	Nullable bool                   `json:"nullable"`
}

type tableQueryPageResponse struct {
	PageNumber int   `json:"page_number"`
	PageSize   int   `json:"page_size"`
	TotalCount int64 `json:"total_count"`
	TotalPages int64 `json:"total_pages"`
}

func queryResponse(result application.QueryResult) tableQueryResponse {
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
		RecordVersions: result.RecordVersions,
		Columns:        columns,
		Rows:           rows,
		Page: tableQueryPageResponse{
			PageNumber: result.Page.PageNumber,
			PageSize:   result.Page.PageSize,
			TotalCount: result.Page.TotalCount,
			TotalPages: result.Page.TotalPages,
		},
	}
}

func assignmentPolicyResponse(policy application.TablePolicy) tablePolicyAssignmentResponse {
	return tablePolicyAssignmentResponse{
		TableName: policy.TableName, QueryPolicyCode: policy.QueryPolicyCode, MutationPolicyCode: policy.MutationPolicyCode,
		Enabled: policy.Enabled, Creator: policy.Creator, Modifier: policy.Modifier,
		CreatedAt: policy.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: policy.UpdatedAt.UTC().Format(time.RFC3339),
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
	case errors.Is(err, application.ErrTablePolicyNotFound):
		writeError(context, stdhttp.StatusNotFound, "table_policy_not_found", "Table Policy not found")
	case errors.Is(err, application.ErrTablePolicyExists):
		writeError(context, stdhttp.StatusConflict, "table_policy_exists", "Table Policy already exists")
	case errors.Is(err, application.ErrQueryPolicyNotFound), errors.Is(err, application.ErrQueryPolicyNotAssignable):
		writeError(context, stdhttp.StatusUnprocessableEntity, "query_policy_not_assignable", "Query Policy is not Active and assignable")
	case errors.Is(err, application.ErrMutationPolicyNotFound), errors.Is(err, application.ErrMutationPolicyNotAssignable):
		writeError(context, stdhttp.StatusUnprocessableEntity, "mutation_policy_not_assignable", "Mutation Policy is not Active and assignable")
	case errors.Is(err, application.ErrUnknownQueryPolicyType), errors.Is(err, application.ErrUnknownMutationPolicyType):
		writeError(context, stdhttp.StatusUnprocessableEntity, "unknown_policy_type", "Policy definition references an unknown Type")
	case errors.Is(err, application.ErrInvalidQueryPolicyRules), errors.Is(err, application.ErrInvalidMutationPolicyRules):
		writeError(context, stdhttp.StatusUnprocessableEntity, "incompatible_policy_definition", "Policy definition is incompatible with the live table Schema")
	case errors.Is(err, application.ErrIncompatibleTable):
		writeError(context, stdhttp.StatusUnprocessableEntity, "incompatible_table", "database table is incompatible with Table Policy management")
	default:
		writeError(context, stdhttp.StatusServiceUnavailable, "policy_catalog_unavailable", "Policy Catalog is unavailable")
	}
	return true
}

func writeQueryPolicyError(context *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, application.ErrInvalidPolicyCode):
		writeError(context, stdhttp.StatusBadRequest, "invalid_policy_code", "Policy Code must be lower-case, versioned, and technology-neutral")
	case errors.Is(err, application.ErrInvalidQueryPolicyDefinition):
		writeError(context, stdhttp.StatusBadRequest, "invalid_query_policy_definition", "Query Policy definition is incomplete")
	case errors.Is(err, application.ErrQueryPolicyNotFound):
		writeError(context, stdhttp.StatusNotFound, "query_policy_not_found", "Query Policy not found")
	case errors.Is(err, application.ErrQueryPolicyExists):
		writeError(context, stdhttp.StatusConflict, "query_policy_exists", "Query Policy already exists")
	case errors.Is(err, application.ErrInvalidPolicyTransition), errors.Is(err, application.ErrQueryPolicyStateConflict):
		writeError(context, stdhttp.StatusConflict, "invalid_policy_transition", "Query Policy lifecycle transition is not allowed")
	case errors.Is(err, application.ErrUnknownQueryPolicyType):
		writeError(context, stdhttp.StatusUnprocessableEntity, "unknown_policy_type", "Query Policy references an unknown Type")
	case errors.Is(err, application.ErrInvalidQueryPolicyRules):
		writeError(context, stdhttp.StatusUnprocessableEntity, "invalid_query_policy_rules", "Query Policy execution rules are invalid")
	default:
		writeError(context, stdhttp.StatusServiceUnavailable, "policy_catalog_unavailable", "Policy Catalog is unavailable")
	}
	return true
}

func writeMutationPolicyError(context *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, application.ErrInvalidPolicyCode):
		writeError(context, stdhttp.StatusBadRequest, "invalid_policy_code", "Policy Code must be lower-case, versioned, and technology-neutral")
	case errors.Is(err, application.ErrInvalidMutationPolicyDefinition):
		writeError(context, stdhttp.StatusBadRequest, "invalid_mutation_policy_definition", "Mutation Policy definition is incomplete")
	case errors.Is(err, application.ErrMutationPolicyNotFound):
		writeError(context, stdhttp.StatusNotFound, "mutation_policy_not_found", "Mutation Policy not found")
	case errors.Is(err, application.ErrMutationPolicyExists):
		writeError(context, stdhttp.StatusConflict, "mutation_policy_exists", "Mutation Policy already exists")
	case errors.Is(err, application.ErrInvalidPolicyTransition), errors.Is(err, application.ErrMutationPolicyStateConflict):
		writeError(context, stdhttp.StatusConflict, "invalid_policy_transition", "Mutation Policy lifecycle transition is not allowed")
	case errors.Is(err, application.ErrUnknownMutationPolicyType):
		writeError(context, stdhttp.StatusUnprocessableEntity, "unknown_policy_type", "Mutation Policy references an unknown Type")
	case errors.Is(err, application.ErrInvalidMutationPolicyRules):
		writeError(context, stdhttp.StatusUnprocessableEntity, "invalid_mutation_policy_rules", "Mutation Policy execution rules are invalid")
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
	case errors.Is(err, application.ErrTablePolicyNotFound):
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
	case errors.Is(err, application.ErrUnknownQueryPolicyType), errors.Is(err, application.ErrUnknownMutationPolicyType):
		writeError(context, stdhttp.StatusUnprocessableEntity, "unknown_policy_type", "Policy Snapshot references an unknown Type")
	case errors.Is(err, application.ErrInvalidQueryPolicyRules), errors.Is(err, application.ErrInvalidMutationPolicyRules), errors.Is(err, application.ErrInvalidPolicySnapshot):
		writeError(context, stdhttp.StatusUnprocessableEntity, "invalid_policy_snapshot", "Policy Snapshot is not executable")
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
	case errors.Is(err, application.ErrRecordVersionRequired):
		writeError(context, stdhttp.StatusUnprocessableEntity, "record_version_required", "read and supply the expected record version")
	case errors.Is(err, application.ErrRecordVersionInvalid):
		writeError(context, stdhttp.StatusUnprocessableEntity, "record_version_invalid", "expected_version must be an unsigned decimal JSON string")
	case errors.Is(err, application.ErrRecordVersionConflict):
		writeError(context, stdhttp.StatusConflict, "record_version_conflict", "record changed; inspect the latest data and explicitly reconfirm")
	case errors.Is(err, application.ErrOperatorFieldIncompatible):
		writeError(context, stdhttp.StatusUnprocessableEntity, "operator_field_incompatible", "Operator fields must be ordinary text columns that can store a complete 36-character Account ID")
	case errors.Is(err, application.ErrProtectedTable):
		writeError(context, stdhttp.StatusForbidden, "protected_table", "protected tables cannot be mutated")
	case errors.Is(err, application.ErrTablePolicyNotFound):
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
	case errors.Is(err, application.ErrUnknownQueryPolicyType), errors.Is(err, application.ErrUnknownMutationPolicyType):
		writeError(context, stdhttp.StatusUnprocessableEntity, "unknown_policy_type", "Policy Snapshot references an unknown Type")
	case errors.Is(err, application.ErrInvalidQueryPolicyRules), errors.Is(err, application.ErrInvalidMutationPolicyRules), errors.Is(err, application.ErrInvalidPolicySnapshot):
		writeError(context, stdhttp.StatusUnprocessableEntity, "invalid_policy_snapshot", "Policy Snapshot is not executable")
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
	TableName             string                             `json:"table_name"`
	TableComment          string                             `json:"table_comment"`
	PolicyExists          bool                               `json:"policy_exists"`
	PolicyEnabled         bool                               `json:"policy_enabled"`
	Compatible            bool                               `json:"compatible"`
	IncompatibilityReason *application.IncompatibilityReason `json:"incompatibility_reason"`
}

func tableResponse(table application.DatabaseTable) databaseTableResponse {
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
	detail := gin.H{"code": code, "message": message, "request_id": requestID(context)}
	if index, exists := context.Get("release_item_index"); exists {
		detail["item_index"] = index
	}
	context.JSON(status, gin.H{"error": detail})
}
