import { useEffect, useRef, useState } from "react";
import { GitBranchPlus, Plus, RefreshCw } from "lucide-react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { isUncertainWriteError } from "../../api/client";
import { presentError } from "../../api/error-messages";
import { Badge } from "../../components/shadcn/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../../components/shadcn/table";
import { Button } from "../../components/ui/Button";
import { Drawer } from "../../components/ui/Drawer";
import { ConfirmDialog } from "../../components/ui/ConfirmDialog";
import { EmptyState, ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useLeaveProtection } from "../../components/ui/LeaveProtection";
import { useToast } from "../../components/ui/Toast";
import { useAccountRole } from "../accounts/roles";
import { nodeTypeLabels, releaseTypeLabels, type ReleaseTemplateDraft } from "./model";
import { useCreateReleaseTemplate, useDeleteReleaseTemplate, useReleaseTemplate, useReleaseTemplates, useReplaceReleaseTemplate, useSetReleaseTemplateEnabled } from "./queries";
import { ReleaseTemplateForm } from "./ReleaseTemplateForm";

export function ReleaseTemplatesPage() {
  const canManage = useAccountRole("ADMIN");
  const navigate = useNavigate();
  const protection = useLeaveProtection();
  const { showToast } = useToast();
  const { code } = useParams<{ code?: string }>();
  const [search] = useSearchParams();
  const creating = code === "new";
  const editing = search.get("mode") === "edit";
  const list = useReleaseTemplates();
  const detail = useReleaseTemplate(creating ? undefined : code);
  const create = useCreateReleaseTemplate();
  const replace = useReplaceReleaseTemplate();
  const state = useSetReleaseTemplateEnabled();
  const remove = useDeleteReleaseTemplate();
  const [commandError, setCommandError] = useState<unknown>(null);
  const [confirmation, setConfirmation] = useState<"enable" | "disable" | "delete" | null>(null);
  type FrozenWrite =
    | { kind: "create"; draft: ReleaseTemplateDraft; key: string }
    | { kind: "replace"; code: string; draft: ReleaseTemplateDraft; version: string; key: string }
    | { kind: "state"; code: string; enabled: boolean; version: string; key: string }
    | { kind: "delete"; code: string; version: string; key: string };
  const unresolvedWrite = useRef<FrozenWrite | null>(null);
  const [uncertainWrite, setUncertainWrite] = useState(false);
  const routeCode = useRef(code);
  useEffect(() => {
    if (routeCode.current === code) return;
    routeCode.current = code;
    unresolvedWrite.current = null;
    setUncertainWrite(false);
    setCommandError(null);
    setConfirmation(null);
    create.reset();
    replace.reset();
    state.reset();
    remove.reset();
  }, [code]);
  const close = () => navigate("/platform/release-templates");
  const mutationError = create.error ?? replace.error;
  const pending = create.isPending || replace.isPending || state.isPending || remove.isPending;
  const finish = (message: string) => {
    unresolvedWrite.current = null;
    setUncertainWrite(false);
    setConfirmation(null);
    setCommandError(null);
    protection.afterSave(() => { showToast(message); close(); });
  };
  const failed = (write: FrozenWrite, error: unknown, commandFailure = false) => {
    const uncertain = isUncertainWriteError(error);
    unresolvedWrite.current = uncertain ? write : null;
    setUncertainWrite(uncertain);
    if (commandFailure) setCommandError(error);
  };
  const execute = (write: FrozenWrite) => {
    if (write.kind === "create") create.mutate(write, { onSuccess: () => finish("发布流程模板已创建"), onError: error => failed(write, error) });
    else if (write.kind === "replace") replace.mutate(write, { onSuccess: () => finish("发布流程模板已更新"), onError: error => failed(write, error) });
    else if (write.kind === "delete") remove.mutate(write, { onSuccess: () => finish("发布流程模板已删除"), onError: error => failed(write, error, true) });
    else state.mutate(write, { onSuccess: () => finish(write.enabled ? "发布流程模板已启用" : "发布流程模板已停用"), onError: error => failed(write, error, true) });
  };
  const submit = (draft: ReleaseTemplateDraft, expectedVersion?: string) => {
    const retry = unresolvedWrite.current;
    if (retry && (retry.kind === "create" || retry.kind === "replace")) { execute(retry); return; }
    const frozen = { ...draft, nodes: draft.nodes.map(node => ({ ...node })) };
    if (creating) execute({ kind: "create", draft: frozen, key: `release-template-${crypto.randomUUID()}` });
    else if (detail.data && expectedVersion) execute({ kind: "replace", code: detail.data.code, draft: frozen, version: expectedVersion, key: `release-template-${crypto.randomUUID()}` });
  };
  const command = () => {
    if (!detail.data || !confirmation) return;
    const retry = unresolvedWrite.current;
    if (retry?.kind === "delete" && confirmation === "delete") { execute(retry); return; }
    if (retry?.kind === "state" && confirmation === (retry.enabled ? "enable" : "disable")) { execute(retry); return; }
    const template = detail.data;
    setCommandError(null);
    setUncertainWrite(false);
    if (confirmation === "delete") execute({ kind: "delete", code: template.code, version: template.version, key: `release-template-${crypto.randomUUID()}` });
    else execute({ kind: "state", code: template.code, enabled: confirmation === "enable", version: template.version, key: `release-template-${crypto.randomUUID()}` });
  };
  const lifecycleRecovery = uncertainWrite && (unresolvedWrite.current?.kind === "state" || unresolvedWrite.current?.kind === "delete") ? unresolvedWrite.current : null;
  const commandPresentation = commandError ? presentError(commandError) : null;
  const stateAction = lifecycleRecovery?.kind === "state" ? lifecycleRecovery.enabled ? "enable" : "disable" : detail.data?.enabled ? "disable" : "enable";
  return <main className="workspace">
    <div className="page-heading"><div><h1>发布流程模板</h1><p>维护常规与应急发布的可复用节点定义。应急模板始终保持可用。</p></div><Button variant="primary" icon={<Plus size={17} />} disabled={!canManage} onClick={() => navigate("/platform/release-templates/new")}>新建模板</Button></div>
    <section className="type-registry" aria-label="发布类型说明"><span><GitBranchPlus size={18} />可用发布类型</span><span className="mutation-type"><code>STANDARD</code><small>审批 → 发布 → 完结</small></span><span className="mutation-type"><code>EMERGENCY</code><small>发布 → 完结</small></span></section>
    <section className="catalog" aria-label="发布流程模板目录">{list.isPending ? <LoadingState label="正在读取模板目录…" /> : list.isError && !list.data ? <ErrorState error={list.error} onRetry={() => void list.refetch()} /> : !list.data?.length ? <EmptyState entity="发布流程模板" /> : <div className="table-scroll"><Table className="policy-table release-template-table"><TableHeader><TableRow><TableHead>模板</TableHead><TableHead>类型</TableHead><TableHead>节点</TableHead><TableHead>状态</TableHead><TableHead>版本 / 最近更新</TableHead><TableHead>操作</TableHead></TableRow></TableHeader><TableBody>{list.data.map(template => <TableRow key={template.code} className={code === template.code ? "selected-row" : ""}><TableCell className="rule-identity"><strong title={template.description}>{template.name}</strong><code>{template.code}</code></TableCell><TableCell><Badge variant="outline" className={template.type === "EMERGENCY" ? "status-badge status-draft" : "status-badge"}>{releaseTypeLabels[template.type]}</Badge></TableCell><TableCell><div className="template-node-summary">{template.nodes.map(node => <span key={node.code}>{nodeTypeLabels[node.type]}</span>)}</div></TableCell><TableCell><Badge variant="outline" className={`status-badge ${template.enabled ? "status-active" : "status-deprecated"}`}>{template.enabled ? "已启用" : "已停用"}</Badge></TableCell><TableCell className="timestamp"><strong>v{template.version}</strong><small>{new Date(template.updatedAt).toLocaleString("zh-CN")}</small></TableCell><TableCell><div className="row-actions"><Button variant="ghost" onClick={() => navigate(`/platform/release-templates/${encodeURIComponent(template.code)}`)}>查看</Button><Button variant="ghost" disabled={!canManage} onClick={() => navigate(`/platform/release-templates/${encodeURIComponent(template.code)}?mode=edit`)}>编辑</Button></div></TableCell></TableRow>)}</TableBody></Table></div>}
      <footer className="catalog-footer"><span>共 {list.data?.length ?? 0} 个模板</span><span>编码与类型稳定 · 节点受约束</span><Button className="catalog-refresh" variant="ghost" icon={<RefreshCw size={15} />} onClick={() => void list.refetch()} disabled={list.isFetching}>刷新</Button></footer>
    </section>
    {code && <Drawer open title={creating ? "新建发布流程模板" : editing ? "编辑发布流程模板" : "发布流程模板详情"} eyebrow="发布流程模板" onClose={() => protection.requestLeave(close)} footer={<>{(creating || editing) && <Button variant="primary" type="submit" form="release-template-form" disabled={!canManage || pending}>{pending ? "正在保存…" : uncertainWrite ? "重推原请求" : "保存模板"}</Button>}{!creating && !editing && detail.data && <><Button variant="primary" disabled={Boolean(lifecycleRecovery)} onClick={() => navigate("?mode=edit")}>编辑</Button><Button disabled={!canManage || pending || detail.data.type === "EMERGENCY" || Boolean(lifecycleRecovery && lifecycleRecovery.kind !== "state")} onClick={() => setConfirmation(stateAction)}>{lifecycleRecovery?.kind === "state" ? `重推${lifecycleRecovery.enabled ? "启用" : "停用"}请求` : detail.data.enabled ? "停用" : "启用"}</Button><Button variant="danger" disabled={!canManage || pending || detail.data.type === "EMERGENCY" || Boolean(lifecycleRecovery && lifecycleRecovery.kind !== "delete")} onClick={() => setConfirmation("delete")}>{lifecycleRecovery?.kind === "delete" ? "重推删除请求" : "删除"}</Button></>}<Button onClick={() => protection.requestLeave(close)} disabled={pending}>关闭</Button></>}>
      {lifecycleRecovery && !confirmation && <div className="inline-alert" role="alert"><strong>{lifecycleRecovery.kind === "delete" ? "删除" : lifecycleRecovery.enabled ? "启用" : "停用"}结果未知；只能针对模板 {lifecycleRecovery.code} 重推原请求，或关闭后放弃本页恢复入口。</strong>{commandPresentation?.requestId && <span>请求编号：{commandPresentation.requestId}</span>}</div>}
      {!creating && detail.isPending ? <LoadingState label="正在读取模板…" /> : !creating && detail.isError && !detail.data ? <ErrorState error={detail.error} onRetry={() => void detail.refetch()} /> : <ReleaseTemplateForm key={`${code}:${editing ? "edit" : creating ? "create" : "view"}`} template={detail.data} mode={creating ? "create" : editing ? "edit" : "view"} pending={pending} serverError={mutationError} uncertainWrite={uncertainWrite && !lifecycleRecovery} onSubmit={submit} />}
    </Drawer>}
    {confirmation && detail.data && <ConfirmDialog open title={confirmation === "delete" ? `删除发布流程模板 ${detail.data.code}？` : `${confirmation === "disable" ? "停用" : "启用"}发布流程模板 ${detail.data.code}？`} description={confirmation === "delete" ? "删除后无法恢复。" : confirmation === "disable" ? "停用后不能用于新的流程实例；已有实例保持不变。" : "启用后可以重新用于新的流程实例。"} confirmLabel={uncertainWrite ? "重推原请求" : confirmation === "delete" ? "确认删除" : confirmation === "disable" ? "确认停用" : "确认启用"} destructive={confirmation === "delete" || confirmation === "disable"} pending={pending} onCancel={() => { if (!pending) { setConfirmation(null); if (!uncertainWrite) setCommandError(null); } }} onConfirm={command}>{commandError ? uncertainWrite ? <div className="inline-alert" role="alert"><strong>操作结果未知；再次确认将原样重推同一请求。</strong>{commandPresentation?.requestId && <span>请求编号：{commandPresentation.requestId}</span>}</div> : <ErrorState error={commandError} /> : null}</ConfirmDialog>}
  </main>;
}
