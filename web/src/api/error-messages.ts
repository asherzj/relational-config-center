import { ApiError } from "./client";

const errorMessages: Record<string, string> = {
  invalid_policy_code: "规则编码必须使用小写、包含版本号，并且不能包含技术实现名称。",
  invalid_query_policy_definition: "查询规则定义不完整，请检查必填项。",
  query_policy_not_found: "查询规则不存在或已被移除。",
  query_policy_exists: "该规则编码已存在。",
  invalid_policy_transition: "当前状态不允许执行此生命周期操作。",
  unknown_policy_type: "Admin 不支持该规则类型。",
  invalid_query_policy_rules: "查询规则的执行约束无效。",
  invalid_mutation_policy_definition: "变更规则定义不完整，请检查必填项。",
  mutation_policy_not_found: "变更规则不存在或已被移除。",
  mutation_policy_exists: "该变更规则编码已存在。",
  invalid_mutation_policy_rules: "变更规则授权或 Auto Fill 约束无效。",
  invalid_policy_definition: "表规则定义不完整，请检查三个必填项。",
  database_table_not_found: "真实数据库表不存在或已被移除。",
  table_policy_not_found: "表规则不存在或已被移除。",
  table_policy_exists: "该真实数据库表已经存在表规则。",
  query_policy_not_assignable: "请选择 Active 且可分配的查询规则。",
  mutation_policy_not_assignable: "请选择 Active 且可分配的变更规则。",
  incompatible_policy_definition: "规则定义与实时表结构不兼容，请检查字段与约束。",
  incompatible_table: "真实数据库表结构不符合 Managed Table 要求。",
  protected_table: "受保护的规则目录表不能分配表规则。",
  database_unavailable: "数据库表发现暂时不可用，请稍后重试。",
  policy_catalog_unavailable: "规则目录暂时不可用，请稍后重试。",
  table_policy_disabled: "该表规则已停用，当前表不再是 Managed Table。",
  invalid_query_condition: "查询条件不符合实时字段类型或查询规则约束。",
  invalid_query_order: "排序字段或方向不符合查询规则约束。",
  invalid_pagination: "分页参数超出查询规则或平台安全限制。",
  invalid_policy_snapshot: "当前规则快照无法执行，请检查规则分配和实时 Schema。",
  query_timeout: "Managed Table 查询超时，请缩小条件或分页范围后重试。",
  query_unavailable: "Managed Table 查询暂时不可用，请稍后重试。",
  mutation_not_allowed: "当前变更规则不允许该写入操作。",
  mutation_row_not_found: "目标记录不存在或已被删除。",
  invalid_mutation_content: "写入内容不符合实时字段 Schema。",
  missing_required_field: "新增内容缺少实时 Schema 要求的字段。",
  duplicate_key: "唯一键已存在，请修改字段值后重试。",
  mutation_timeout: "Managed Table 写入超时，请确认结果后再决定是否重试。",
  mutation_unavailable: "Managed Table 写入响应未能确认，请先核对当前结果。",
  request_body_too_large: "请求内容超过大小限制。",
  invalid_request: "请求内容不符合 Admin 契约。",
  network_error: "Admin 连接或响应传输中断。",
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
