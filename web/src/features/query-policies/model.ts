import type { PolicyStatus } from "../policies/model";

export { allowedActions, formatTimestamp, policyStatusLabels } from "../policies/model";
export type { PolicyAction, PolicyStatus } from "../policies/model";
export type OrderDirection = "ASC" | "DESC";

export type QueryPolicy = {
  code: string;
  name: string;
  description: string;
  typeCode: string;
  defaultOrderField: string;
  defaultOrderDirection: OrderDirection;
  defaultPageSize: number;
  maxPageSize: number;
  status: PolicyStatus;
  creator: string;
  modifier: string;
  createdAt: string;
  modifiedAt: string;
};

export type QueryPolicyDraft = Pick<
  QueryPolicy,
  | "code"
  | "name"
  | "description"
  | "typeCode"
  | "defaultOrderField"
  | "defaultOrderDirection"
  | "defaultPageSize"
  | "maxPageSize"
>;

export type QueryPolicyMetadata = Pick<QueryPolicy, "name" | "description">;

export const supportedQueryPolicyTypes = new Set(["page_query"]);

export function validateDraft(draft: QueryPolicyDraft): Partial<Record<keyof QueryPolicyDraft, string>> {
  const errors: Partial<Record<keyof QueryPolicyDraft, string>> = {};
  if (!/^[a-z][a-z0-9_]*_v[1-9][0-9]*$/.test(draft.code)) {
    errors.code = "使用小写字母、数字和下划线，并以 _v1 这类版本号结尾。";
  } else if (/^(mysql|mariadb|postgres|postgresql|sqlite|oracle|sqlserver|mongodb|gorm|sql)_/.test(draft.code)) {
    errors.code = "规则编码不能包含技术实现名称。";
  }
  if (!draft.name.trim()) errors.name = "请输入显示名称。";
  if (!supportedQueryPolicyTypes.has(draft.typeCode)) errors.typeCode = "Web 尚不支持编辑该规则类型。";
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(draft.defaultOrderField)) {
    errors.defaultOrderField = "请输入安全的字段名。";
  }
  if (!Number.isInteger(draft.defaultPageSize) || draft.defaultPageSize < 1) {
    errors.defaultPageSize = "默认页大小必须为正整数。";
  }
  if (!Number.isInteger(draft.maxPageSize) || draft.maxPageSize < 1 || draft.maxPageSize > 200) {
    errors.maxPageSize = "最大页大小必须为 1 到 200 的整数。";
  } else if (draft.defaultPageSize > draft.maxPageSize) {
    errors.defaultPageSize = "默认页大小不能超过最大页大小。";
  }
  return errors;
}
