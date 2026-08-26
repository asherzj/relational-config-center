import { ApiError } from "./client";

const errorMessages: Record<string, string> = {
  invalid_policy_code: "策略编码必须使用小写、包含版本号，并且不能包含技术实现名称。",
  invalid_query_policy_definition: "查询策略定义不完整，请检查必填项。",
  query_policy_not_found: "查询策略不存在或已被移除。",
  query_policy_exists: "该策略编码已存在。",
  invalid_policy_transition: "当前状态不允许执行此生命周期操作。",
  unknown_policy_type: "Admin 不支持该策略类型。",
  invalid_query_policy_rules: "查询策略执行规则无效。",
  invalid_mutation_policy_definition: "变更策略定义不完整，请检查必填项。",
  mutation_policy_not_found: "变更策略不存在或已被移除。",
  mutation_policy_exists: "该变更策略编码已存在。",
  invalid_mutation_policy_rules: "变更策略授权或 Auto Fill 规则无效。",
  invalid_policy_definition: "Table Policy 定义不完整，请检查三个必填项。",
  database_table_not_found: "真实数据库表不存在或已被移除。",
  table_policy_not_found: "Table Policy 不存在或已被移除。",
  table_policy_exists: "该真实数据库表已经存在 Table Policy。",
  query_policy_not_assignable: "请选择 Active 且可分配的 Query Policy。",
  mutation_policy_not_assignable: "请选择 Active 且可分配的 Mutation Policy。",
  incompatible_policy_definition: "Policy 定义与实时表结构不兼容，请检查字段与规则。",
  incompatible_table: "真实数据库表结构不符合 Managed Table 要求。",
  protected_table: "受保护的策略目录表不能分配 Table Policy。",
  database_unavailable: "数据库表发现暂时不可用，请稍后重试。",
  policy_catalog_unavailable: "策略目录暂时不可用，请稍后重试。",
  table_policy_disabled: "该 Table Policy 已停用，当前表不再是 Managed Table。",
  invalid_query_condition: "查询条件不符合实时字段类型或 Query Policy 约束。",
  invalid_query_order: "排序字段或方向不符合 Query Policy 约束。",
  invalid_pagination: "分页参数超出 Query Policy 或平台安全限制。",
  invalid_policy_snapshot: "当前 Policy Snapshot 无法执行，请检查策略分配和实时 Schema。",
  query_timeout: "Managed Table 查询超时，请缩小条件或分页范围后重试。",
  query_unavailable: "Managed Table 查询暂时不可用，请稍后重试。",
  request_body_too_large: "请求内容超过大小限制。",
  invalid_request: "请求内容不符合 Admin 契约。",
  network_error: "无法连接 Admin，请检查服务状态后重试。",
  contract_mismatch: "Admin 响应与 Web 契约不一致。",
  unexpected_response: "Admin 返回了未识别的响应。",
};

export function presentError(error: unknown): { message: string; requestId?: string } {
  if (!(error instanceof ApiError)) return { message: "发生未知错误，请重试。" };
  return {
    message: errorMessages[error.code] ?? error.message,
    requestId: error.requestId,
  };
}
