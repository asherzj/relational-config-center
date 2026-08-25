export type PolicyStatus = "DRAFT" | "ACTIVE" | "DEPRECATED";
export type PolicyAction = "replace" | "activate" | "delete" | "metadata" | "deprecate";

export const allowedActions: Record<PolicyStatus, readonly PolicyAction[]> = {
  DRAFT: ["replace", "activate", "delete"],
  ACTIVE: ["metadata", "deprecate"],
  DEPRECATED: ["metadata"],
};

export const policyStatusLabels: Record<PolicyStatus, string> = {
  DRAFT: "草稿",
  ACTIVE: "已激活",
  DEPRECATED: "已弃用",
};

export function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  })
    .format(date)
    .replaceAll("/", "-");
}
