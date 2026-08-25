import { AlertCircle } from "lucide-react";
import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "../../components/ui/Button";
import { ConfirmDialog } from "../../components/ui/ConfirmDialog";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useToast } from "../../components/ui/Toast";
import { supportsMutationPolicyType } from "../mutation-policies/model";
import { useMutationPolicies, useMutationPolicyTypes } from "../mutation-policies/queries";
import { supportedQueryPolicyTypes } from "../query-policies/model";
import { useQueryPolicies, useQueryPolicyTypes } from "../query-policies/queries";
import type { TablePolicyAssignment } from "./model";
import { useCreateTablePolicy, useDatabaseTables, useDisableTablePolicy, useEnableTablePolicy, useReplaceTablePolicy, useTablePolicy } from "./queries";

type Props = { tableName?: string };
const emptyAssignment: TablePolicyAssignment = { tableName: "", queryPolicyCode: "", mutationPolicyCode: "" };

export function TablePolicyDrawer({ tableName }: Props) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { showToast } = useToast();
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
  const loadError = (selectingAssignment && (discovery.error || queryPolicies.error || queryPolicyTypes.error || mutationPolicies.error || mutationPolicyTypes.error)) || detail.error;
  const valid = Boolean(assignment.tableName && assignment.queryPolicyCode && assignment.mutationPolicyCode);

  const executeReplace = () => {
    if (!tableName || !valid) return;
    replace.mutate({ tableName, assignment }, {
      onSuccess() {
        setConfirmReplace(false);
        showToast("Table Policy 已原子替换");
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
        showToast(command === "enable" ? "Table Policy 已启用" : "Table Policy 已停用");
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
        showToast("Table Policy 已创建并保持未启用");
        navigate(`/platform/table-policies/${encodeURIComponent(policy.tableName)}`);
      },
    });
    else if (detail.data?.enabled) setConfirmReplace(true);
    else executeReplace();
  };

  let content: ReactNode;
  if (loading) content = <LoadingState label="正在读取真实表与 Active Policy…" />;
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
      <label className="field"><span>Query Policy Code</span><input value={detail.data.queryPolicyCode} disabled readOnly /></label>
      <label className="field"><span>Mutation Policy Code</span><input value={detail.data.mutationPolicyCode} disabled readOnly /></label>
      <dl className="audit-grid"><div><dt>创建人</dt><dd>{detail.data.creator}</dd></div><div><dt>修改人</dt><dd>{detail.data.modifier}</dd></div><div><dt>创建时间</dt><dd>{detail.data.createdAt}</dd></div><div><dt>修改时间</dt><dd>{detail.data.modifiedAt}</dd></div></dl>
      {(enable.error || disable.error) && <ErrorState error={enable.error || disable.error} />}
    </div>
  );
  else content = (
    <form id="table-policy-form" className="policy-form" onSubmit={submit}>
      <div className="form-note"><AlertCircle size={17} /><span>{creating ? "新分配始终创建为未启用；启用前 Admin 会再次校验实时 Schema 与两个 Policy 引用。" : "两个 Policy Code 将作为一个候选整体校验并原子替换；失败时当前分配保持不变。"}</span></div>
      {creating ? <label className="field"><span>真实数据库表</span><select aria-label="真实数据库表" value={assignment.tableName} onChange={(event) => setAssignment((current) => ({ ...current, tableName: event.target.value }))}>
          <option value="">请选择兼容且未分配的表</option>
          {candidates.map((table) => <option key={table.tableName} value={table.tableName}>{table.tableName}{table.tableComment ? ` · ${table.tableComment}` : ""}</option>)}
        </select></label> : <label className="field"><span>真实数据库表</span><input value={assignment.tableName} disabled readOnly /></label>}
      <label className="field"><span>Active Query Policy</span><select aria-label="Active Query Policy" value={assignment.queryPolicyCode} onChange={(event) => setAssignment((current) => ({ ...current, queryPolicyCode: event.target.value }))}>
        <option value="">请选择 Active Query Policy</option>
        {activeQueryPolicies.map((policy) => <option key={policy.code} value={policy.code}>{policy.code} · {policy.name}</option>)}
      </select></label>
      <label className="field"><span>Active Mutation Policy</span><select aria-label="Active Mutation Policy" value={assignment.mutationPolicyCode} onChange={(event) => setAssignment((current) => ({ ...current, mutationPolicyCode: event.target.value }))}>
        <option value="">请选择 Active Mutation Policy</option>
        {activeMutationPolicies.map((policy) => <option key={policy.code} value={policy.code}>{policy.code} · {policy.name}</option>)}
      </select></label>
      {creating && !candidates.length && <div className="inline-alert"><AlertCircle size={17} /><span>没有兼容且未分配的真实数据库表。</span></div>}
      {(create.error || replace.error) && <ErrorState error={create.error || replace.error} />}
    </form>
  );

  let footer: ReactNode;
  if (creating || replacing) footer = <><Button type="submit" form="table-policy-form" variant="primary" disabled={!valid || create.isPending || replace.isPending}>{create.isPending || replace.isPending ? "正在保存…" : creating ? "创建未启用分配" : "检查并替换"}</Button><Button onClick={() => replacing && tableName ? navigate(`/platform/table-policies/${encodeURIComponent(tableName)}`) : close()} disabled={create.isPending || replace.isPending}>取消</Button></>;
  else footer = <><Button variant="primary" onClick={() => navigate("?mode=replace")}>原子替换</Button>{detail.data && <Button variant={detail.data.enabled ? "danger" : "primary"} onClick={() => setPendingStateCommand(detail.data.enabled ? "disable" : "enable")}>{detail.data.enabled ? "停用" : "启用"}</Button>}<Button className="drawer-close-action" onClick={close}>关闭</Button></>;
  const title = creating ? "新建 Table Policy 分配" : replacing ? "原子替换 Table Policy" : "Table Policy 详情";
  const stateConfirm = pendingStateCommand === "enable" ? { title: "启用 Table Policy？", description: "Admin 将根据实时 Schema 和两个 Policy 引用重新校验；成功后该表成为 Managed Table。", label: "确认启用" } : { title: "停用 Table Policy？", description: "停用后该表立即失去 Managed Table 身份，后续数据 API 请求将被拒绝。", label: "确认停用" };
  return <><Drawer open title={title} eyebrow="表策略" onClose={close} footer={footer}>{content}</Drawer>{detail.data?.enabled && <ConfirmDialog open={confirmReplace} title="替换已启用的 Table Policy？" description="替换提交成功后，下一次请求立即生效，并使用新的完整 Policy Snapshot。" confirmLabel="确认原子替换" pending={replace.isPending} onCancel={() => setConfirmReplace(false)} onConfirm={executeReplace} />}{pendingStateCommand && <ConfirmDialog open title={stateConfirm.title} description={stateConfirm.description} confirmLabel={stateConfirm.label} destructive={pendingStateCommand === "disable"} pending={enable.isPending || disable.isPending} onCancel={() => setPendingStateCommand(null)} onConfirm={executeStateCommand} />}</>;
}
