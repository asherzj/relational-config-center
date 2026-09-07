import type { PolicyStatus } from "../policies/model";

export { allowedActions, formatTimestamp, policyStatusLabels } from "../policies/model";
export type { PolicyAction, PolicyStatus } from "../policies/model";
export type MutationOperation = "ADD" | "MODIFY" | "DELETE";

export type MutationPolicyType = {
  code: string;
  operations: MutationOperation[];
};

export type MutationPolicy = {
  code: string;
  name: string;
  description: string;
  typeCode: string;
  allowAdd: boolean;
  allowModify: boolean;
  allowDelete: boolean;
  createOperatorField: string | null;
  createTimeField: string | null;
  modifyOperatorField: string | null;
  modifyTimeField: string | null;
  status: PolicyStatus;
  creator: string;
  modifier: string;
  createdAt: string;
  modifiedAt: string;
};

export type MutationPolicyDraft = Pick<
  MutationPolicy,
  | "code"
  | "name"
  | "description"
  | "typeCode"
  | "allowAdd"
  | "allowModify"
  | "allowDelete"
  | "createOperatorField"
  | "createTimeField"
  | "modifyOperatorField"
  | "modifyTimeField"
>;

export type MutationPolicyMetadata = Pick<MutationPolicy, "name" | "description">;

export const supportedMutationPolicyTypes = new Set(["single_table_mutation"]);

const requiredMutationOperations: readonly MutationOperation[] = ["ADD", "MODIFY", "DELETE"];

export function supportsMutationPolicyType(types: readonly MutationPolicyType[] | undefined, code: string): boolean {
  if (!supportedMutationPolicyTypes.has(code)) return false;
  const registered = types?.find((type) => type.code === code);
  return Boolean(
    registered
    && registered.operations.length === requiredMutationOperations.length
    && requiredMutationOperations.every((operation) => registered.operations.includes(operation)),
  );
}

type DraftErrors = Partial<Record<keyof MutationPolicyDraft, string>>;

export function validateDraft(draft: MutationPolicyDraft): DraftErrors {
  const errors: DraftErrors = {};
  if (!/^[a-z][a-z0-9_]*_v[1-9][0-9]*$/.test(draft.code)) {
    errors.code = "使用小写字母、数字和下划线，并以 _v1 这类版本号结尾。";
  } else if (/^(mysql|mariadb|postgres|postgresql|sqlite|oracle|sqlserver|mongodb|gorm|sql)_/.test(draft.code)) {
    errors.code = "规则编码不能包含技术实现名称。";
  }
  if (!draft.name.trim()) errors.name = "请输入显示名称。";
  else if ([...draft.name.trim()].length > 100) errors.name = "显示名称不能超过 100 个字符。";
  if ([...draft.description.trim()].length > 500) errors.description = "描述不能超过 500 个字符。";
  if (!supportedMutationPolicyTypes.has(draft.typeCode)) errors.typeCode = "Web 尚不支持编辑该规则类型。";

  const targetKeys = ["createOperatorField", "createTimeField", "modifyOperatorField", "modifyTimeField"] as const;
  const seen = new Map<string, typeof targetKeys[number]>();
  for (const key of targetKeys) {
    const value = draft[key]?.trim();
    if (!value) continue;
    if (value.toLowerCase() === "id") errors[key] = "id 是主键，不能作为 Auto Fill 目标。";
    else if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(value)) errors[key] = "请输入安全的字段名。";
    else if (value.length > 64) errors[key] = "字段名不能超过 64 个字符。";
    const normalizedValue = value.toLowerCase();
    const prior = seen.get(normalizedValue);
    if (prior) {
      errors[key] = "Auto Fill 目标不能重复。";
      errors[prior] = "Auto Fill 目标不能重复。";
    } else {
      seen.set(normalizedValue, key);
    }
  }
  if (!draft.allowAdd && (draft.createOperatorField || draft.createTimeField)) {
    if (draft.createOperatorField && !errors.createOperatorField) errors.createOperatorField = "Create Auto Fill 需要允许 ADD。";
    if (draft.createTimeField && !errors.createTimeField) errors.createTimeField = "Create Auto Fill 需要允许 ADD。";
  }
  if (!draft.allowAdd && !draft.allowModify && (draft.modifyOperatorField || draft.modifyTimeField)) {
    if (draft.modifyOperatorField && !errors.modifyOperatorField) errors.modifyOperatorField = "Modify Auto Fill 需要允许 ADD 或 MODIFY。";
    if (draft.modifyTimeField && !errors.modifyTimeField) errors.modifyTimeField = "Modify Auto Fill 需要允许 ADD 或 MODIFY。";
  }
  return errors;
}
