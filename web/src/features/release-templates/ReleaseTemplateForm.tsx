import { useState, type FormEvent } from "react";
import { Input } from "../../components/shadcn/input";
import { Label } from "../../components/shadcn/label";
import { NativeSelect } from "../../components/shadcn/native-select";
import { Textarea } from "../../components/shadcn/textarea";
import { Badge } from "../../components/shadcn/badge";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { presentError } from "../../api/error-messages";
import { defaultNodes, nodeTypeLabels, releaseTypeLabels, roleLabels, validateReleaseTemplate, type ReleaseTemplate, type ReleaseTemplateDraft, type ReleaseType } from "./model";

const emptyDraft: ReleaseTemplateDraft = { code: "", name: "", description: "", type: "STANDARD", nodes: defaultNodes("STANDARD") };
const draftFor = (template?: ReleaseTemplate): ReleaseTemplateDraft => template ? { code: template.code, name: template.name, description: template.description, type: template.type, nodes: template.nodes.map(node => ({ ...node })) } : emptyDraft;

export function ReleaseTemplateForm({ template, mode, pending, serverError, uncertainWrite, onSubmit }: { template?: ReleaseTemplate; mode: "create" | "edit" | "view"; pending: boolean; serverError?: unknown; uncertainWrite?: boolean; onSubmit: (draft: ReleaseTemplateDraft, expectedVersion?: string) => void }) {
  const [baseline] = useState(() => draftFor(template));
  const [baselineVersion] = useState(template?.version);
  const [draft, setDraft] = useState(() => draftFor(template));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const readOnly = mode === "view";
  useDraftProtection(!readOnly && JSON.stringify(draft) !== JSON.stringify(baseline), pending);
  const updateType = (type: ReleaseType) => { setDraft(current => ({ ...current, type, nodes: defaultNodes(type) })); setErrors({}); };
  const updateNode = (index: number, key: "code" | "name", value: string) => {
    setDraft(current => ({ ...current, nodes: current.nodes.map((node, nodeIndex) => nodeIndex === index ? { ...node, [key]: value } : node) }));
    setErrors(current => ({ ...current, [`node-${index}-${key}`]: "", nodes: "" }));
  };
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (readOnly || pending) return;
    const next = { ...draft, code: draft.code.trim(), name: draft.name.trim(), description: draft.description.trim(), nodes: draft.nodes.map(node => ({ ...node, code: node.code.trim(), name: node.name.trim() })) };
    const nextErrors = validateReleaseTemplate(next);
    if (Object.keys(nextErrors).length) { setErrors(nextErrors); return; }
    onSubmit(next, baselineVersion);
  };
  const shownError = serverError ? presentError(serverError) : null;
  return <form id="release-template-form" className="policy-form" onSubmit={submit} noValidate>
    {shownError && <div className="inline-alert" role="alert"><strong>{uncertainWrite ? "保存结果未知；请保持当前页面，再次点击保存将原样重推同一请求。" : shownError.message}</strong>{shownError.requestId && <span>请求编号：{shownError.requestId}</span>}</div>}
    {template && <Badge variant="outline" className={`status-badge ${template.enabled ? "status-active" : "status-deprecated"}`}>{template.enabled ? "已启用" : "已停用"}</Badge>}
    <fieldset className="form-controls" disabled={pending || readOnly || uncertainWrite}>
      <section className="form-section" aria-labelledby="template-basics-heading">
        <div className="form-section-heading"><h3 id="template-basics-heading">模板信息</h3><p>编码与发布类型创建后固定；名称和节点名称会保存到后续实例。</p></div>
        <Label className="field field-wide"><span>模板编码 · 创建后不可变</span><Input autoFocus value={draft.code} disabled={mode !== "create"} onChange={event => setDraft(current => ({ ...current, code: event.target.value }))} aria-invalid={Boolean(errors.code)} />{errors.code && <small className="field-error">{errors.code}</small>}</Label>
        <Label className="field field-wide"><span>模板名称</span><Input value={draft.name} onChange={event => setDraft(current => ({ ...current, name: event.target.value }))} aria-invalid={Boolean(errors.name)} />{errors.name && <small className="field-error">{errors.name}</small>}</Label>
        <Label className="field field-wide"><span>发布类型</span><NativeSelect aria-label="发布类型" value={draft.type} disabled={mode !== "create"} onChange={event => updateType(event.target.value as ReleaseType)}><option value="STANDARD">常规</option><option value="EMERGENCY">应急</option></NativeSelect><small>{releaseTypeLabels[draft.type]}模板采用固定可执行节点顺序。</small></Label>
        <Label className="field field-wide"><span>描述</span><Textarea rows={3} value={draft.description} onChange={event => setDraft(current => ({ ...current, description: event.target.value }))} aria-invalid={Boolean(errors.description)} />{errors.description && <small className="field-error">{errors.description}</small>}</Label>
      </section>
      <section className="form-panel" aria-labelledby="template-nodes-heading">
        <div className="form-section-heading"><h3 id="template-nodes-heading">流程节点</h3><p>节点类型、顺序和权限来源由发布类型约束；审批复用正式按表审批能力。</p></div>
        {errors.nodes && <p className="field-error">{errors.nodes}</p>}
        <div className="template-node-list">{draft.nodes.map((node, index) => <section className="template-node" key={`${node.type}-${index}`} aria-label={`${index + 1}. ${nodeTypeLabels[node.type]}`}>
          <header><span className="template-node-index">{index + 1}</span><strong>{nodeTypeLabels[node.type]}</strong><Badge variant="outline">{roleLabels[node.requiredRole]}</Badge></header>
          <div className="form-grid">
            <Label className="field"><span>节点编码</span><Input value={node.code} onChange={event => updateNode(index, "code", event.target.value)} aria-invalid={Boolean(errors[`node-${index}-code`])} />{errors[`node-${index}-code`] && <small className="field-error">{errors[`node-${index}-code`]}</small>}</Label>
            <Label className="field"><span>节点名称</span><Input value={node.name} onChange={event => updateNode(index, "name", event.target.value)} aria-invalid={Boolean(errors[`node-${index}-name`])} />{errors[`node-${index}-name`] && <small className="field-error">{errors[`node-${index}-name`]}</small>}</Label>
          </div>
        </section>)}</div>
        <p className="form-note">监控节点未启用；模板的监控列表固定为空。</p>
      </section>
      {template && <dl className="audit-grid"><div><dt>版本</dt><dd>{template.version}</dd></div><div><dt>创建人</dt><dd>{template.creator}</dd></div><div><dt>修改人</dt><dd>{template.modifier}</dd></div><div><dt>最近更新</dt><dd>{new Date(template.updatedAt).toLocaleString("zh-CN")}</dd></div></dl>}
    </fieldset>
  </form>;
}
