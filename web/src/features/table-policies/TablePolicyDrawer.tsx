import { AlertCircle } from "lucide-react";
import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "../../components/ui/Button";
import { isUncertainWriteError } from "../../api/client";
import { ConfirmDialog } from "../../components/ui/ConfirmDialog";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useToast } from "../../components/ui/Toast";
import { supportsMutationPolicyType } from "../mutation-policies/model";
import { useMutationPolicies, useMutationPolicyTypes } from "../mutation-policies/queries";
import { supportedQueryPolicyTypes } from "../query-policies/model";
import { useQueryPolicies, useQueryPolicyTypes } from "../query-policies/queries";
import type { TablePolicyAssignment } from "./model";
import { tablePolicyKeys, useCreateTablePolicy, useDatabaseTables, useDisableTablePolicy, useEnableTablePolicy, useReplaceTablePolicy, useTablePolicy } from "./queries";

type Props = { tableName?: string };
const emptyAssignment: TablePolicyAssignment = { tableName: "", queryPolicyCode: "", mutationPolicyCode: "" };

export function TablePolicyDrawer({ tableName }: Props) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { showToast } = useToast();
  const queryClient = useQueryClient();
  const creating = !tableName && searchParams.get("mode") === "create";
  const replacing = !creating && searchParams.get("mode") === "replace";
  const selectingAssignment = creating || replacing;
  const discovery = useDatabaseTables(selectingAssignment);
  const queryPolicies = useQueryPolicies(selectingAssignment);
  const queryPolicyTypes = useQueryPolicyTypes(selectingAssignment);
  const mutationPolicies = useMutationPolicies(selectingAssignment);
  const mutationPolicyTypes = useMutationPolicyTypes(selectingAssignment);
  const create = useCreateTablePolicy();
  const replace = useReplaceTablePolicy();
  const enable = useEnableTablePolicy();
  const disable = useDisableTablePolicy();
  const detail = useTablePolicy(creating ? undefined : tableName);
  const [assignment, setAssignment] = useState<TablePolicyAssignment>(emptyAssignment);
  const [confirmReplace, setConfirmReplace] = useState(false);
  const [pendingStateCommand, setPendingStateCommand] = useState<"enable" | "disable" | null>(null);
  const close = () => navigate("/platform/table-policies");
  const serverError = create.error || replace.error || enable.error || disable.error;
  const uncertain = isUncertainWriteError(serverError);
  const verifyCurrentState = () => {
    create.reset(); replace.reset(); enable.reset(); disable.reset();
    void Promise.all([
      queryClient.refetchQueries({ queryKey: tablePolicyKeys.list }),
      queryClient.refetchQueries({ queryKey: tablePolicyKeys.discovery }),
      ...(tableName ? [queryClient.refetchQueries({ queryKey: tablePolicyKeys.detail(tableName) })] : []),
    ]).finally(() => navigate(tableName ? `/platform/table-policies/${encodeURIComponent(tableName)}` : "/platform/table-policies"));
  };
  const writeError = serverError && (uncertain
    ? <div className="inline-alert" role="alert"><strong>提交结果尚未确认。系统不会自动重复此写入。</strong>{selectingAssignment && <Button type="button" variant="secondary" onClick={verifyCurrentState}>只读查询当前状态</Button>}</div>
    : <ErrorState error={serverError} />);

  useEffect(() => {
    if (creating) setAssignment(emptyAssignment);
  }, [creating]);

  useEffect(() => {
    if (detail.data && (!replacing || assignment.tableName !== detail.data.tableName)) {
      setAssignment({ tableName: detail.data.tableName, queryPolicyCode: detail.data.queryPolicyCode, mutationPolicyCode: detail.data.mutationPolicyCode });
    }
  }, [assignment.tableName, detail.data, replacing]);

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

  const executeReplace = () => {
    if (!tableName || !valid) return;
    replace.mutate({ tableName, assignment }, {
      onSuccess() {
        setConfirmReplace(false);
        showToast("表规则已原子替换");
        navigate(`/platform/table-policies/${encodeURIComponent(tableName)}`);
      },
      onError() { setConfirmReplace(false); },
    });
  };

  const executeStateCommand = () => {
    if (!tableName || !pendingStateCommand) return;
    const command = pendingStateCommand;
    const mutation = command === "enable" ? enable : disable;
    mutation.mutate(tableName, {
      onSuccess() {
        showToast(command === "enable" ? "表规则已启用" : "表规则已停用");
        setPendingStateCommand(null);
      },
      onError() { setPendingStateCommand(null); },
    });
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!valid) return;
    if (creating) create.mutate(assignment, {
      onSuccess(policy) {
        showToast("表规则已创建并保持未启用");
        navigate(`/platform/table-policies/${encodeURIComponent(policy.tableName)}`);
      },
    });
    else if (detail.data?.enabled) setConfirmReplace(true);
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
      <span className={`status-badge ${detail.data.enabled ? "status-active" : "status-draft"}`}>{detail.data.enabled ? "已启用" : "未启用"}</span>
      <label className="field"><span>真实数据库表</span><input value={detail.data.tableName} disabled readOnly /></label>
      <label className="field"><span>查询规则编码</span><input value={detail.data.queryPolicyCode} disabled readOnly /></label>
      <label className="field"><span>变更规则编码</span><input value={detail.data.mutationPolicyCode} disabled readOnly /></label>
      <dl className="audit-grid"><div><dt>创建人</dt><dd>{detail.data.creator}</dd></div><div><dt>修改人</dt><dd>{detail.data.modifier}</dd></div><div><dt>创建时间</dt><dd>{detail.data.createdAt}</dd></div><div><dt>修改时间</dt><dd>{detail.data.modifiedAt}</dd></div></dl>
      {writeError}
    </div>
  );
  else content = (
    <form id="table-policy-form" className="policy-form" onSubmit={submit}>
      <div className="form-note"><AlertCircle size={17} /><span>{creating ? "新分配始终创建为未启用；启用前 Admin 会再次校验实时 Schema 与两条规则引用。" : "两个规则编码将作为一个候选整体校验并原子替换；失败时当前分配保持不变。"}</span></div>
      {creating ? <label className="field"><span>真实数据库表</span><select aria-label="真实数据库表" value={assignment.tableName} onChange={(event) => setAssignment((current) => ({ ...current, tableName: event.target.value }))}>
          <option value="">请选择兼容且未分配的表</option>
          {candidates.map((table) => <option key={table.tableName} value={table.tableName}>{table.tableName}{table.tableComment ? ` · ${table.tableComment}` : ""}</option>)}
        </select></label> : <label className="field"><span>真实数据库表</span><input value={assignment.tableName} disabled readOnly /></label>}
      <label className="field"><span>Active 查询规则</span><select aria-label="Active 查询规则" value={assignment.queryPolicyCode} onChange={(event) => setAssignment((current) => ({ ...current, queryPolicyCode: event.target.value }))}>
        <option value="">请选择 Active 查询规则</option>
        {activeQueryPolicies.map((policy) => <option key={policy.code} value={policy.code}>{policy.code} · {policy.name}</option>)}
      </select></label>
      <label className="field"><span>Active 变更规则</span><select aria-label="Active 变更规则" value={assignment.mutationPolicyCode} onChange={(event) => setAssignment((current) => ({ ...current, mutationPolicyCode: event.target.value }))}>
        <option value="">请选择 Active 变更规则</option>
        {activeMutationPolicies.map((policy) => <option key={policy.code} value={policy.code}>{policy.code} · {policy.name}</option>)}
      </select></label>
      {creating && !candidates.length && <div className="inline-alert"><AlertCircle size={17} /><span>没有兼容且未分配的真实数据库表。</span></div>}
      {writeError}
    </form>
  );

  let footer: ReactNode;
  if (creating || replacing) footer = <><Button type="submit" form="table-policy-form" variant="primary" disabled={!valid || uncertain || create.isPending || replace.isPending}>{create.isPending || replace.isPending ? "正在保存…" : creating ? "创建未启用分配" : "检查并替换"}</Button><Button onClick={() => replacing && tableName ? navigate(`/platform/table-policies/${encodeURIComponent(tableName)}`) : close()} disabled={create.isPending || replace.isPending}>取消</Button></>;
  else footer = uncertain
    ? <><Button variant="primary" onClick={verifyCurrentState}>只读查询当前状态</Button><Button className="drawer-close-action" onClick={close}>关闭</Button></>
    : <><Button variant="primary" onClick={() => navigate("?mode=replace")}>原子替换</Button>{detail.data && <Button variant={detail.data.enabled ? "danger" : "primary"} onClick={() => setPendingStateCommand(detail.data.enabled ? "disable" : "enable")}>{detail.data.enabled ? "停用" : "启用"}</Button>}<Button className="drawer-close-action" onClick={close}>关闭</Button></>;
  const title = creating ? "新建表规则分配" : replacing ? "原子替换表规则" : "表规则详情";
  const stateConfirm = pendingStateCommand === "enable" ? { title: "启用表规则？", description: "Admin 将根据实时 Schema 和两条规则引用重新校验；成功后该表成为 Managed Table。", label: "确认启用" } : { title: "停用表规则？", description: "停用后该表立即失去 Managed Table 身份，后续数据 API 请求将被拒绝。", label: "确认停用" };
  return <><Drawer open title={title} eyebrow="表规则" onClose={close} footer={footer}>{content}</Drawer>{detail.data?.enabled && <ConfirmDialog open={confirmReplace} title="替换已启用的表规则？" description="替换提交成功后，下一次请求立即生效，并使用新的完整规则快照。" confirmLabel="确认原子替换" pending={replace.isPending} onCancel={() => setConfirmReplace(false)} onConfirm={executeReplace} />}{pendingStateCommand && <ConfirmDialog open title={stateConfirm.title} description={stateConfirm.description} confirmLabel={stateConfirm.label} destructive={pendingStateCommand === "disable"} pending={enable.isPending || disable.isPending} onCancel={() => setPendingStateCommand(null)} onConfirm={executeStateCommand} />}</>;
}
