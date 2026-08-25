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
  policy_catalog_unavailable: "策略目录暂时不可用，请稍后重试。",
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
