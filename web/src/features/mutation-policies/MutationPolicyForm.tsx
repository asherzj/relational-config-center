import { Info } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { presentError } from "../../api/error-messages";
import type { PolicyFormMode } from "../policies/lifecycle";
import {
  formatTimestamp,
  policyStatusLabels,
  supportedMutationPolicyTypes,
  validateDraft,
  type MutationPolicy,
  type MutationPolicyDraft,
  type MutationPolicyMetadata,
} from "./model";

export type FormMode = PolicyFormMode;

const emptyDraft: MutationPolicyDraft = {
  code: "",
  name: "",
  description: "",
  typeCode: "single_table_mutation",
  allowAdd: false,
  allowModify: false,
  allowDelete: false,
  createOperatorField: null,
  createTimeField: null,
  modifyOperatorField: null,
  modifyTimeField: null,
};

function draftFor(policy?: MutationPolicy): MutationPolicyDraft {
  if (!policy) return emptyDraft;
  const { code, name, description, typeCode, allowAdd, allowModify, allowDelete, createOperatorField, createTimeField, modifyOperatorField, modifyTimeField } = policy;
  return { code, name, description, typeCode, allowAdd, allowModify, allowDelete, createOperatorField, createTimeField, modifyOperatorField, modifyTimeField };
}

type Props = {
  mode: FormMode;
  policy?: MutationPolicy;
  typeCodes: string[];
  serverError?: unknown;
  pending?: boolean;
  onSubmit: (value: MutationPolicyDraft | MutationPolicyMetadata) => void;
};

export function MutationPolicyForm({ mode, policy, typeCodes, serverError, pending = false, onSubmit }: Props) {
  const [baseline] = useState(() => draftFor(policy));
  const [editableDraft, setDraft] = useState<MutationPolicyDraft>(baseline);
  const draft = mode === "view" ? draftFor(policy) : editableDraft;
  const [errors, setErrors] = useState<Partial<Record<keyof MutationPolicyDraft, string>>>({});
  const executionLocked = mode === "view" || mode === "metadata";
  const fullyLocked = mode === "view";

  // Keyed by resource and mode: refreshes cannot reset an editing session.
  const comparable = (value: typeof draft) => ({
    ...value, name: value.name.trim(), description: value.description.trim(),
    createOperatorField: value.createOperatorField?.trim() || null,
    createTimeField: value.createTimeField?.trim() || null,
    modifyOperatorField: value.modifyOperatorField?.trim() || null,
    modifyTimeField: value.modifyTimeField?.trim() || null,
  });
  useDraftProtection(mode !== "view" && JSON.stringify(comparable(draft)) !== JSON.stringify(comparable(baseline)), pending);

  const update = <K extends keyof MutationPolicyDraft>(key: K, value: MutationPolicyDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
    setErrors((current) => ({ ...current, [key]: undefined }));
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (pending || mode === "view") return;
    if (mode === "metadata") {
      const next = { name: draft.name.trim(), description: draft.description.trim() };
      const nextErrors: typeof errors = {};
      if (!next.name) nextErrors.name = "请输入显示名称。";
      if ([...next.name].length > 100) nextErrors.name = "显示名称不能超过 100 个字符。";
      if ([...next.description].length > 500) nextErrors.description = "描述不能超过 500 个字符。";
      if (Object.keys(nextErrors).length) { setErrors(nextErrors); return; }
      onSubmit(next);
      return;
    }
    const optional = (value: string | null) => value?.trim() || null;
    const next: MutationPolicyDraft = {
      ...draft,
      name: draft.name.trim(),
      description: draft.description.trim(),
      createOperatorField: optional(draft.createOperatorField),
      createTimeField: optional(draft.createTimeField),
      modifyOperatorField: optional(draft.modifyOperatorField),
      modifyTimeField: optional(draft.modifyTimeField),
    };
    const nextErrors = validateDraft(next);
    if (Object.keys(nextErrors).length) { setErrors(nextErrors); return; }
    onSubmit(next);
  };

  const inputProps = (key: keyof MutationPolicyDraft) => ({
    "aria-invalid": Boolean(errors[key]),
    "aria-describedby": errors[key] ? `${key}-error` : undefined,
  });
  const field = (key: "createOperatorField" | "createTimeField" | "modifyOperatorField" | "modifyTimeField", label: string, placeholder: string) => (
    <label className="field">
      <span>{label}</span>
      <input value={draft[key] ?? ""} onChange={(event) => update(key, event.target.value)} disabled={executionLocked} placeholder={placeholder} {...inputProps(key)} />
      {errors[key] && <small id={`${key}-error`} className="field-error">{errors[key]}</small>}
    </label>
  );
  const supportedTypes = typeCodes.filter((type) => supportedMutationPolicyTypes.has(type));
  const options = [...new Set([...supportedTypes, draft.typeCode])];
  const presentedError = serverError ? presentError(serverError) : null;

  return (
    <form id="mutation-policy-form" className="policy-form" onSubmit={submit} noValidate>
      {presentedError && <div className="inline-alert" role="alert"><strong>{presentedError.message}</strong>{presentedError.requestId && <span>请求编号：{presentedError.requestId}</span>}</div>}
      <fieldset className="form-controls" disabled={pending}>
      {policy && <span className={`status-badge status-${policy.status.toLowerCase()}`}>{policyStatusLabels[policy.status]}</span>}

      <label className="field field-wide"><span>规则编码 · 创建后不可变</span><input value={draft.code} onChange={(event) => update("code", event.target.value)} disabled={mode !== "create"} placeholder="standard_mutation_v2" {...inputProps("code")} />{errors.code && <small id="code-error" className="field-error">{errors.code}</small>}</label>
      <label className="field field-wide"><span>显示名称</span><input value={draft.name} onChange={(event) => update("name", event.target.value)} disabled={fullyLocked} {...inputProps("name")} />{errors.name && <small id="name-error" className="field-error">{errors.name}</small>}</label>
      <label className="field field-wide"><span>描述</span><textarea value={draft.description} onChange={(event) => update("description", event.target.value)} disabled={fullyLocked} rows={4} {...inputProps("description")} />{errors.description && <small id="description-error" className="field-error">{errors.description}</small>}</label>

      <div className="form-panel">
        <label className="field field-wide"><span>规则类型</span><select value={draft.typeCode} onChange={(event) => update("typeCode", event.target.value)} disabled={executionLocked} {...inputProps("typeCode")}>{options.map((type) => <option key={type} value={type}>{type}</option>)}</select>{errors.typeCode && <small id="typeCode-error" className="field-error">{errors.typeCode}</small>}</label>
        <section className="mutation-form-section" aria-labelledby="operation-heading">
          <h3 id="operation-heading">操作授权</h3>
          <div className="capability-grid">
            {(["allowAdd", "allowModify", "allowDelete"] as const).map((key) => (
              <label className="capability-toggle" key={key}>
                <span>{key === "allowAdd" ? "ADD" : key === "allowModify" ? "MODIFY" : "DELETE"}</span>
                <input type="checkbox" checked={draft[key]} onChange={(event) => update(key, event.target.checked)} disabled={executionLocked} />
                <strong>{draft[key] ? "允许" : "禁止"}</strong>
              </label>
            ))}
          </div>
        </section>
        <section className="mutation-form-section" aria-labelledby="auto-fill-heading">
          <h3 id="auto-fill-heading">标准 Auto Fill 目标字段</h3>
          <p>Operator 使用当前 Operator；Time 使用数据库时间。留空表示不填充。</p>
          <div className="form-grid">
            {field("createOperatorField", "Create Operator Field", "creator")}
            {field("createTimeField", "Create Time Field", "created_at")}
            {field("modifyOperatorField", "Modify Operator Field", "modifier")}
            {field("modifyTimeField", "Modify Time Field", "updated_at")}
          </div>
        </section>
      </div>

      {policy && policy.status !== "DRAFT" && <div className="form-note"><Info size={17} /><span>已激活或已弃用规则的授权与 Auto Fill 字段已锁定；只能更新名称和描述。</span></div>}
      {policy && <dl className="audit-grid"><div><dt>创建人</dt><dd>{policy.creator}</dd></div><div><dt>创建时间</dt><dd>{formatTimestamp(policy.createdAt)}</dd></div><div><dt>修改人</dt><dd>{policy.modifier}</dd></div><div><dt>修改时间</dt><dd>{formatTimestamp(policy.modifiedAt)}</dd></div></dl>}
      </fieldset>
    </form>
  );
}
