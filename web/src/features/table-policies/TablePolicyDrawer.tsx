import { useAccountRole } from "../accounts/roles";
import { Input } from "../../components/shadcn/input";
import { NativeSelect } from "../../components/shadcn/native-select";
import { Label } from "../../components/shadcn/label";
import { Badge } from "../../components/shadcn/badge";
import { AlertCircle } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { Button } from "../../components/ui/Button";
import { isUncertainWriteError, prioritizeUncertainWriteError } from "../../api/client";
import { ConfirmDialog } from "../../components/ui/ConfirmDialog";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useToast } from "../../components/ui/Toast";
import { supportsMutationPolicyType } from "../mutation-policies/model";
import { useMutationPolicies, useMutationPolicyTypes } from "../mutation-policies/queries";
import { MutationPolicyEffect, QueryPolicyEffect } from "../policies/PolicyEffect";
import { supportedQueryPolicyTypes } from "../query-policies/model";
import { useQueryPolicies, useQueryPolicyTypes } from "../query-policies/queries";
import type { TablePolicyAssignment } from "./model";
import { tablePolicyKeys, useCreateTablePolicy, useDatabaseTables, useDisableTablePolicy, useEnableTablePolicy, useReplaceTablePolicy, useTablePolicy } from "./queries";

type Props = { tableName?: string };
const emptyAssignment: TablePolicyAssignment = { tableName: "", queryPolicyCode: "", mutationPolicyCode: "" };

export function TablePolicyDrawer({ tableName }: Props) {
  const [searchParams] = useSearchParams();
  const create = useCreateTablePolicy();
  const replace = useReplaceTablePolicy();
  const enable = useEnableTablePolicy();
  const disable = useDisableTablePolicy();
  // Editing selections reset on navigation; an unconfirmed write outcome must
  // survive closing and reopening the drawer until its state is read back.
  return <TablePolicySession key={`${tableName}:${searchParams.get("mode")}`} tableName={tableName} commands={{ create, replace, enable, disable }} />;
}

type Commands = {
  create: ReturnType<typeof useCreateTablePolicy>;
  replace: ReturnType<typeof useReplaceTablePolicy>;
  enable: ReturnType<typeof useEnableTablePolicy>;
  disable: ReturnType<typeof useDisableTablePolicy>;
};

function TablePolicySession({ tableName, commands: { create, replace, enable, disable } }: Props & { commands: Commands }) {
  const navigate = useNavigate();
  const canManage = useAccountRole("ADMIN");
  const editingAllowedAtOpen = useRef(canManage).current;
  const [searchParams] = useSearchParams();
  const { showToast } = useToast();
  const queryClient = useQueryClient();
  const creating = editingAllowedAtOpen && !tableName && searchParams.get("mode") === "create";
  const replacing = editingAllowedAtOpen && !creating && searchParams.get("mode") === "replace";
  const selectingAssignment = creating || replacing;
  const discovery = useDatabaseTables(selectingAssignment);
  const queryPolicies = useQueryPolicies();
  const queryPolicyTypes = useQueryPolicyTypes();
  const mutationPolicies = useMutationPolicies();
  const mutationPolicyTypes = useMutationPolicyTypes();
  const detail = useTablePolicy(creating ? undefined : tableName);
  const [assignment, setAssignment] = useState<TablePolicyAssignment>(emptyAssignment);
  const [baseline, setBaseline] = useState<TablePolicyAssignment | null>(creating ? emptyAssignment : null);
  const inFlight = useRef(false);
  const pending = create.isPending || replace.isPending || enable.isPending || disable.isPending;
  const protection = useDraftProtection(selectingAssignment && baseline !== null && JSON.stringify(assignment) !== JSON.stringify(baseline), pending);
  const [confirmReplace, setConfirmReplace] = useState(false);
  const [pendingStateCommand, setPendingStateCommand] = useState<"enable" | "disable" | null>(null);
  const close = () => navigate("/platform/table-policies");
  const serverError = prioritizeUncertainWriteError([create.error, replace.error, enable.error, disable.error]);
  const uncertain = isUncertainWriteError(serverError);
  const verifyCurrentState = async () => {
    try {
      await Promise.all([
        queryClient.refetchQueries({ queryKey: tablePolicyKeys.list }, { throwOnError: true }),
        queryClient.refetchQueries({ queryKey: tablePolicyKeys.discovery }, { throwOnError: true }),
        ...(tableName ? [queryClient.refetchQueries({ queryKey: tablePolicyKeys.detail(tableName) }, { throwOnError: true })] : []),
      ]);
    } catch {
      // A failed read cannot establish the outcome of the previous write.
      return;
    }
    create.reset(); replace.reset(); enable.reset(); disable.reset();
    protection.afterSave(() => navigate(tableName ? `/platform/table-policies/${encodeURIComponent(tableName)}` : "/platform/table-policies"));
  };
  const writeError = serverError && (uncertain
    ? <div className="inline-alert" role="alert"><strong>提交结果尚未确认。系统不会自动重复此写入。</strong>{selectingAssignment && <Button type="button" variant="secondary" onClick={verifyCurrentState}>只读查询当前状态</Button>}</div>
    : <ErrorState error={serverError} />);

  useEffect(() => {
    if (!baseline && detail.data) {
      const initial = { tableName: detail.data.tableName, queryPolicyCode: detail.data.queryPolicyCode, mutationPolicyCode: detail.data.mutationPolicyCode };
      setAssignment(initial);
      setBaseline(initial);
    }
  }, [baseline, detail.data]);

  if (!tableName && !creating) return null;

  const candidates = (discovery.data ?? []).filter((table) => table.compatible && !table.policyExists);
  const activeQueryPolicies = (queryPolicies.data ?? []).filter((policy) => policy.status === "ACTIVE" && supportedQueryPolicyTypes.has(policy.typeCode) && queryPolicyTypes.data?.includes(policy.typeCode));
  const activeMutationPolicies = (mutationPolicies.data ?? []).filter((policy) => policy.status === "ACTIVE" && supportsMutationPolicyType(mutationPolicyTypes.data, policy.typeCode));
  const loading = (selectingAssignment && (discovery.isPending || queryPolicies.isPending || queryPolicyTypes.isPending || mutationPolicies.isPending || mutationPolicyTypes.isPending)) || (!creating && detail.isPending);
  const loadError = (selectingAssignment && (
    (discovery.isError && !discovery.data && discovery.error)
    || (queryPolicies.isError && !queryPolicies.data && queryPolicies.error)
    || (queryPolicyTypes.isError && !queryPolicyTypes.data && queryPolicyTypes.error)
    || (mutationPolicies.isError && !mutationPolicies.data && mutationPolicies.error)
    || (mutationPolicyTypes.isError && !mutationPolicyTypes.data && mutationPolicyTypes.error)
  )) || (detail.isError && !detail.data && detail.error);
  const valid = Boolean(
    assignment.tableName
    && assignment.queryPolicyCode
    && assignment.mutationPolicyCode
    && (!creating || candidates.some((table) => table.tableName === assignment.tableName))
    && activeQueryPolicies.some((policy) => policy.code === assignment.queryPolicyCode)
    && activeMutationPolicies.some((policy) => policy.code === assignment.mutationPolicyCode),
  );

  const queryRegistryState = queryPolicyTypes.isPending ? "loading" : queryPolicyTypes.isError ? "error" : "ready";
  const mutationRegistryState = mutationPolicyTypes.isPending ? "loading" : mutationPolicyTypes.isError ? "error" : "ready";
  const effects = (target: TablePolicyAssignment, preview: boolean, enabled = false) => {
    const selectedQueryPolicy = queryPolicies.data?.find((policy) => policy.code === target.queryPolicyCode);
    const selectedMutationPolicy = mutationPolicies.data?.find((policy) => policy.code === target.mutationPolicyCode);
    const intro = preview
      ? creating
        ? "这里只预览候选分配；创建后保持未启用，启用成功后才能查询或变更配置内容。"
        : detail.data?.enabled
          ? "当前选择尚未提交；替换成功后，下一次数据请求立即按所选规则执行。"
          : "当前选择尚未提交；替换成功后仍保持未启用，启用后才能查询或变更配置内容。"
      : enabled
        ? "该表的后续数据请求按这两条规则执行。"
        : "当前分配尚未启用，数据请求会被拒绝；以下是启用后才会生效的效果预览。";
    return (
    <section className="assignment-effects" aria-label={preview ? "所选规则效果预览" : "当前已选规则效果"}>
      <div className="form-section-heading">
        <h3>{preview ? "所选规则效果预览" : "当前已选规则效果"}</h3>
        <p>{intro}</p>
      </div>
      {queryPolicies.isError ? <div className="policy-effect policy-effect-unconfirmed"><strong>无法确认查询效果</strong><p>查询规则目录加载或刷新失败，无法用最新数据确认 <code>{target.queryPolicyCode}</code> 的效果。</p></div> : queryPolicies.isPending ? <div className="policy-effect policy-effect-unconfirmed"><strong>正在确认查询效果</strong><p>正在读取查询规则目录。</p></div> : selectedQueryPolicy ? <QueryPolicyEffect policy={selectedQueryPolicy} registeredTypes={queryPolicyTypes.data} registryState={queryRegistryState} heading="查询效果" /> : target.queryPolicyCode ? <div className="policy-effect policy-effect-unconfirmed"><strong>无法确认查询效果</strong><p>最新规则目录中没有 <code>{target.queryPolicyCode}</code>，当前界面不会猜测其能力。</p></div> : null}
      {mutationPolicies.isError ? <div className="policy-effect policy-effect-unconfirmed"><strong>无法确认变更效果</strong><p>变更规则目录加载或刷新失败，无法用最新数据确认 <code>{target.mutationPolicyCode}</code> 的效果。</p></div> : mutationPolicies.isPending ? <div className="policy-effect policy-effect-unconfirmed"><strong>正在确认变更效果</strong><p>正在读取变更规则目录。</p></div> : selectedMutationPolicy ? <MutationPolicyEffect policy={selectedMutationPolicy} registeredTypes={mutationPolicyTypes.data} registryState={mutationRegistryState} heading="变更效果" /> : target.mutationPolicyCode ? <div className="policy-effect policy-effect-unconfirmed"><strong>无法确认变更效果</strong><p>最新规则目录中没有 <code>{target.mutationPolicyCode}</code>，当前界面不会猜测其能力。</p></div> : null}
    </section>
    );
  };

  const executeReplace = () => {
    if (!canManage) return;
    if (!tableName || !valid || uncertain || inFlight.current || pending) return;
    inFlight.current = true;
    replace.mutate({ tableName, assignment }, {
      onSuccess() {
        setConfirmReplace(false);
        showToast("表规则已替换");
        protection.afterSave(() => navigate(`/platform/table-policies/${encodeURIComponent(tableName)}`));
      },
      onError() { setConfirmReplace(false); },
      onSettled() { inFlight.current = false; },
    });
  };

  const executeStateCommand = () => {
    if (!canManage || !tableName || !pendingStateCommand || uncertain || inFlight.current || pending) return;
    inFlight.current = true;
    const command = pendingStateCommand;
    const mutation = command === "enable" ? enable : disable;
    mutation.mutate(tableName, {
      onSuccess() {
        showToast(command === "enable" ? "表规则已启用" : "表规则已停用");
        setPendingStateCommand(null);
      },
      onError() { setPendingStateCommand(null); },
      onSettled() { inFlight.current = false; },
    });
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!canManage) return;
    if (!valid || uncertain || inFlight.current || pending) return;
    if (creating) {
      inFlight.current = true;
      create.mutate(assignment, {
        onSuccess(policy) {
          showToast("表规则已创建并保持未启用");
          protection.afterSave(() => navigate(`/platform/table-policies/${encodeURIComponent(policy.tableName)}`));
        },
        onSettled() { inFlight.current = false; },
      });
    } else if (detail.data?.enabled) setConfirmReplace(true);
    else executeReplace();
  };

  let content: ReactNode;
  if (loading) content = <LoadingState label="正在读取真实表与 Active 规则…" />;
  else if (loadError) content = <ErrorState error={loadError} onRetry={() => {
    if (selectingAssignment) {
      void discovery.refetch();
      void queryPolicies.refetch();
      void queryPolicyTypes.refetch();
      void mutationPolicies.refetch();
      void mutationPolicyTypes.refetch();
    }
    if (!creating) void detail.refetch();
  }} />;
  else if (!creating && !replacing && detail.data) content = (
    <div className="policy-form">
      <Badge variant="outline" className={`status-badge ${detail.data.enabled ? "status-active" : "status-draft"}`}>{detail.data.enabled ? "已启用" : "未启用"}</Badge>
      <Label className="field"><span>真实数据库表</span><Input value={detail.data.tableName} disabled readOnly /></Label>
      <Label className="field"><span>查询规则编码</span><Input value={detail.data.queryPolicyCode} disabled readOnly /></Label>
      <Label className="field"><span>变更规则编码</span><Input value={detail.data.mutationPolicyCode} disabled readOnly /></Label>
      {effects({ tableName: detail.data.tableName, queryPolicyCode: detail.data.queryPolicyCode, mutationPolicyCode: detail.data.mutationPolicyCode }, false, detail.data.enabled)}
      <dl className="audit-grid"><div><dt>创建人</dt><dd>{detail.data.creator}</dd></div><div><dt>修改人</dt><dd>{detail.data.modifier}</dd></div><div><dt>创建时间</dt><dd>{detail.data.createdAt}</dd></div><div><dt>修改时间</dt><dd>{detail.data.modifiedAt}</dd></div></dl>
      {writeError}
    </div>
  );
  else content = (
    <form id="table-policy-form" className="policy-form" onSubmit={submit}>
      <fieldset className="form-controls" disabled={pending || !canManage}>
      <div className="form-note"><AlertCircle size={17} /><span>{creating ? "新分配始终创建为未启用；启用前 Admin 会再次校验实时 Schema 与两条规则引用。" : "查询规则和变更规则会一起校验、一起替换；任何一项失败，当前分配都保持不变。"}</span></div>
      {creating ? <Label className="field"><span>真实数据库表</span><NativeSelect aria-label="真实数据库表" value={assignment.tableName} onChange={(event) => setAssignment((current) => ({ ...current, tableName: event.target.value }))}>
          <option value="">请选择兼容且未分配的表</option>
          {assignment.tableName && !candidates.some((table) => table.tableName === assignment.tableName) && <option value={assignment.tableName} disabled>{assignment.tableName} · 当前选择已不可新分配</option>}
          {candidates.map((table) => <option key={table.tableName} value={table.tableName}>{table.tableName}{table.tableComment ? ` · ${table.tableComment}` : ""}</option>)}
        </NativeSelect></Label> : <Label className="field"><span>真实数据库表</span><Input value={assignment.tableName} disabled readOnly /></Label>}
      <Label className="field"><span>Active 查询规则</span><NativeSelect aria-label="Active 查询规则" value={assignment.queryPolicyCode} onChange={(event) => setAssignment((current) => ({ ...current, queryPolicyCode: event.target.value }))}>
        <option value="">请选择 Active 查询规则</option>
        {assignment.queryPolicyCode && !activeQueryPolicies.some((policy) => policy.code === assignment.queryPolicyCode) && <option value={assignment.queryPolicyCode} disabled>{assignment.queryPolicyCode} · 当前引用或选择</option>}
        {activeQueryPolicies.map((policy) => <option key={policy.code} value={policy.code}>{policy.code} · {policy.name}</option>)}
      </NativeSelect></Label>
      <Label className="field"><span>Active 变更规则</span><NativeSelect aria-label="Active 变更规则" value={assignment.mutationPolicyCode} onChange={(event) => setAssignment((current) => ({ ...current, mutationPolicyCode: event.target.value }))}>
        <option value="">请选择 Active 变更规则</option>
        {assignment.mutationPolicyCode && !activeMutationPolicies.some((policy) => policy.code === assignment.mutationPolicyCode) && <option value={assignment.mutationPolicyCode} disabled>{assignment.mutationPolicyCode} · 当前引用或选择</option>}
        {activeMutationPolicies.map((policy) => <option key={policy.code} value={policy.code}>{policy.code} · {policy.name}</option>)}
      </NativeSelect></Label>
      {(assignment.queryPolicyCode || assignment.mutationPolicyCode) && effects(assignment, true)}
      {creating && !candidates.length && <div className="inline-alert"><AlertCircle size={17} /><span>没有兼容且未分配的真实数据库表。</span></div>}
      {writeError}
      </fieldset>
    </form>
  );

  let footer: ReactNode;
  if (creating || replacing) footer = <><Button type="submit" form="table-policy-form" variant="primary" disabled={!valid || uncertain || create.isPending || replace.isPending}>{create.isPending || replace.isPending ? "正在保存…" : creating ? "创建未启用分配" : "检查并替换"}</Button><Button onClick={() => replacing && tableName ? navigate(`/platform/table-policies/${encodeURIComponent(tableName)}`) : close()} disabled={create.isPending || replace.isPending}>取消</Button></>;
  else footer = uncertain
    ? <><Button variant="primary" onClick={verifyCurrentState}>只读查询当前状态</Button><Button className="drawer-close-action" onClick={close}>关闭</Button></>
    : <><Button variant="primary" onClick={() => navigate("?mode=replace")}>替换所选规则</Button>{detail.data && <Button variant={detail.data.enabled ? "danger" : "primary"} onClick={() => setPendingStateCommand(detail.data.enabled ? "disable" : "enable")}>{detail.data.enabled ? "停用" : "启用"}</Button>}<Button className="drawer-close-action" onClick={close}>关闭</Button></>;
  if (!canManage) footer = <Button onClick={close}>关闭</Button>;
  const title = creating ? "新建表规则分配" : replacing ? "替换表规则" : "表规则详情";
  const stateConfirm = pendingStateCommand === "enable" ? { title: "启用表规则？", description: "Admin 将根据实时 Schema 和两条规则引用重新校验；成功后该表成为 Managed Table。", label: "确认启用" } : { title: "停用表规则？", description: "停用后该表立即失去 Managed Table 身份，后续数据 API 请求将被拒绝。", label: "确认停用" };
  return <><Drawer open title={title} eyebrow="表规则" onClose={close} footer={footer}>{content}</Drawer>{detail.data?.enabled && <ConfirmDialog open={confirmReplace} title="替换已启用的表规则？" description="查询规则和变更规则校验成功后会一起替换，下一次请求立即生效。" confirmLabel="确认替换" pending={replace.isPending} onCancel={() => setConfirmReplace(false)} onConfirm={executeReplace} />}{pendingStateCommand && <ConfirmDialog open title={stateConfirm.title} description={stateConfirm.description} confirmLabel={stateConfirm.label} destructive={pendingStateCommand === "disable"} pending={enable.isPending || disable.isPending} onCancel={() => setPendingStateCommand(null)} onConfirm={executeStateCommand} />}</>;
}
