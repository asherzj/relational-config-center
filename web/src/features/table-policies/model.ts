export type IncompatibilityReason =
  | "missing_primary_key"
  | "composite_primary_key"
  | "primary_key_must_be_id";

export type DatabaseTable = {
  tableName: string;
  tableComment: string;
  policyExists: boolean;
  policyEnabled: boolean;
  compatible: boolean;
  incompatibilityReason: IncompatibilityReason | null;
};

export type TablePolicy = {
  version: string;
  concurrencyKey?: string[];
  tableName: string;
  queryPolicyCode: string;
  mutationPolicyCode: string;
  enabled: boolean;
  creator: string;
  modifier: string;
  createdAt: string;
  modifiedAt: string;
};

export type TablePolicyAssignment = Pick<TablePolicy, "tableName" | "queryPolicyCode" | "mutationPolicyCode" | "concurrencyKey">;

export const incompatibilityReasonLabels: Record<IncompatibilityReason, string> = {
  missing_primary_key: "缺少主键",
  composite_primary_key: "不支持复合主键",
  primary_key_must_be_id: "主键必须命名为 id",
};

export function formatTimestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "medium" }).format(date);
}
