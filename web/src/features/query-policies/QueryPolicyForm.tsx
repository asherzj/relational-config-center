import { Info } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { presentError } from "../../api/error-messages";
import type { PolicyFormMode } from "../policies/lifecycle";
import { QueryPolicyEffect, type RegistryState } from "../policies/PolicyEffect";
import {
  formatTimestamp,
  policyStatusLabels,
  supportedQueryPolicyTypes,
  validateDraft,
  type QueryPolicy,
  type QueryPolicyDraft,
  type QueryPolicyMetadata,
} from "./model";

export type FormMode = PolicyFormMode;

const emptyDraft: QueryPolicyDraft = {
  code: "",
  name: "",
  description: "",
  typeCode: "page_query",
  defaultOrderField: "id",
  defaultOrderDirection: "DESC",
  defaultPageSize: 20,
  maxPageSize: 200,
};

function draftFor(policy?: QueryPolicy): QueryPolicyDraft {
  if (!policy) return emptyDraft;
  return {
    code: policy.code,
    name: policy.name,
    description: policy.description,
    typeCode: policy.typeCode,
    defaultOrderField: policy.defaultOrderField,
    defaultOrderDirection: policy.defaultOrderDirection,
    defaultPageSize: policy.defaultPageSize,
    maxPageSize: policy.maxPageSize,
  };
}

type Props = {
  mode: FormMode;
  policy?: QueryPolicy;
  typeCodes: string[];
  registryState: RegistryState;
  serverError?: unknown;
  pending?: boolean;
  onSubmit: (value: QueryPolicyDraft | QueryPolicyMetadata) => void;
};

export function QueryPolicyForm({ mode, policy, typeCodes, registryState, serverError, pending = false, onSubmit }: Props) {
  const [baseline] = useState(() => draftFor(policy));
  const [editableDraft, setDraft] = useState<QueryPolicyDraft>(baseline);
  const draft = mode === "view" ? draftFor(policy) : editableDraft;
  const [errors, setErrors] = useState<Partial<Record<keyof QueryPolicyDraft, string>>>({});
  const executionLocked = mode === "view" || mode === "metadata";
  const fullyLocked = mode === "view";

  // Keyed by resource and mode: refreshes cannot reset an editing session.
  const comparable = (value: typeof draft) => ({
    ...value, name: value.name.trim(), description: value.description.trim(), defaultOrderField: value.defaultOrderField.trim(),
  });
  useDraftProtection(mode !== "view" && JSON.stringify(comparable(draft)) !== JSON.stringify(comparable(baseline)), pending);

  const update = <K extends keyof QueryPolicyDraft>(key: K, value: QueryPolicyDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
    setErrors((current) => ({ ...current, [key]: undefined }));
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (pending || mode === "view") return;
    if (mode === "metadata") {
      const next: QueryPolicyMetadata = { name: draft.name.trim(), description: draft.description.trim() };
      if (!next.name) {
        setErrors({ name: "请输入显示名称。" });
        return;
      }
      onSubmit(next);
      return;
    }
    const next = { ...draft, name: draft.name.trim(), description: draft.description.trim(), defaultOrderField: draft.defaultOrderField.trim() };
    const nextErrors = validateDraft(next);
    if (Object.keys(nextErrors).length) {
      setErrors(nextErrors);
      return;
    }
    onSubmit(next);
  };

  const inputProps = (key: keyof QueryPolicyDraft) => ({
    "aria-invalid": Boolean(errors[key]),
    "aria-describedby": errors[key] ? `${key}-error` : undefined,
  });

  const supportedTypes = typeCodes.filter((code) => supportedQueryPolicyTypes.has(code));
  const options = supportedTypes.length ? supportedTypes : [draft.typeCode];
  const presentedError = serverError ? presentError(serverError) : null;

  return (
    <form id="query-policy-form" className="policy-form" onSubmit={submit} noValidate>
      {presentedError && (
        <div className="inline-alert" role="alert">
          <strong>{presentedError.message}</strong>
          {presentedError.requestId && <span>请求编号：{presentedError.requestId}</span>}
        </div>
      )}

      <fieldset className="form-controls" disabled={pending}>
      {policy && <span className={`status-badge status-${policy.status.toLowerCase()}`}>{policyStatusLabels[policy.status]}</span>}

      <section className="form-section" aria-labelledby="query-display-heading">
        <div className="form-section-heading">
          <h3 id="query-display-heading">名称和描述</h3>
          <p>用于在规则目录中识别这条规则，不改变查询的执行内容。</p>
        </div>

      <label className="field field-wide">
        <span>规则编码 · 创建后不可变</span>
        <input
          value={draft.code}
          onChange={(event) => update("code", event.target.value)}
          disabled={mode !== "create"}
          placeholder="standard_page_query_v2"
          {...inputProps("code")}
        />
        {errors.code && <small id="code-error" className="field-error">{errors.code}</small>}
      </label>

      <label className="field field-wide">
        <span>显示名称</span>
        <input value={draft.name} onChange={(event) => update("name", event.target.value)} disabled={fullyLocked} {...inputProps("name")} />
        {errors.name && <small id="name-error" className="field-error">{errors.name}</small>}
      </label>

      <label className="field field-wide">
        <span>描述</span>
        <textarea value={draft.description} onChange={(event) => update("description", event.target.value)} disabled={fullyLocked} rows={4} />
      </label>
      </section>

      <section className="form-panel" aria-labelledby="query-execution-heading">
        <div className="form-section-heading">
          <h3 id="query-execution-heading">执行规则</h3>
          <p>{mode === "metadata" ? "当前状态不能修改执行规则。" : "草稿激活后，这些内容将锁定。"}</p>
        </div>
        <label className="field field-wide">
          <span>规则类型</span>
          <select value={draft.typeCode} onChange={(event) => update("typeCode", event.target.value)} disabled={executionLocked} {...inputProps("typeCode")}>
            {options.map((code) => <option key={code} value={code}>{code}</option>)}
          </select>
          {errors.typeCode && <small id="typeCode-error" className="field-error">{errors.typeCode}</small>}
        </label>
        <div className="form-grid">
          <label className="field">
            <span>默认排序字段</span>
            <input value={draft.defaultOrderField} onChange={(event) => update("defaultOrderField", event.target.value)} disabled={executionLocked} {...inputProps("defaultOrderField")} />
            {errors.defaultOrderField && <small id="defaultOrderField-error" className="field-error">{errors.defaultOrderField}</small>}
          </label>
          <label className="field">
            <span>默认排序方向</span>
            <select value={draft.defaultOrderDirection} onChange={(event) => update("defaultOrderDirection", event.target.value as "ASC" | "DESC")} disabled={executionLocked}>
              <option value="ASC">ASC</option>
              <option value="DESC">DESC</option>
            </select>
          </label>
          <label className="field">
            <span>默认页大小</span>
            <input type="number" min="1" value={draft.defaultPageSize} onChange={(event) => update("defaultPageSize", Number(event.target.value))} disabled={executionLocked} {...inputProps("defaultPageSize")} />
            {errors.defaultPageSize && <small id="defaultPageSize-error" className="field-error">{errors.defaultPageSize}</small>}
          </label>
          <label className="field">
            <span>最大页大小 · 上限 200</span>
            <input type="number" min="1" max="200" value={draft.maxPageSize} onChange={(event) => update("maxPageSize", Number(event.target.value))} disabled={executionLocked} {...inputProps("maxPageSize")} />
            {errors.maxPageSize && <small id="maxPageSize-error" className="field-error">{errors.maxPageSize}</small>}
          </label>
        </div>
        <QueryPolicyEffect policy={draft} registeredTypes={typeCodes} registryState={registryState} heading={mode === "create" || mode === "replace" || policy?.status === "DRAFT" ? "执行效果预览" : "实际查询效果"} />
      </section>

      {policy && policy.status !== "DRAFT" && (
        <div className="form-note"><Info size={17} /><span>当前状态只能修改名称和描述，执行内容不会改变。若要改变执行规则，请新建使用新版本编码的草稿，激活后再替换表分配。{policy.status === "DEPRECATED" ? "这条已弃用规则对已有分配仍然有效，但不能用于新分配。" : ""}</span></div>
      )}

      {policy && (
        <dl className="audit-grid">
          <div><dt>创建人</dt><dd>{policy.creator}</dd></div>
          <div><dt>创建时间</dt><dd>{formatTimestamp(policy.createdAt)}</dd></div>
          <div><dt>修改人</dt><dd>{policy.modifier}</dd></div>
          <div><dt>修改时间</dt><dd>{formatTimestamp(policy.modifiedAt)}</dd></div>
        </dl>
      )}
      </fieldset>
    </form>
  );
}
