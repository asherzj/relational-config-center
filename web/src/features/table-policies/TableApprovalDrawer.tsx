import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { tableApprovals, type TableApprovalAssignment, type TableApprovalInput, type ApprovalRoleIdentity } from "../../api/table-approval";
import { approvalRoles } from "../../api/approval-roles";
import { ApiError } from "../../api/client";
import { Button } from "../../components/ui/Button";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { useToast } from "../../components/ui/Toast";
import { Checkbox } from "../../components/shadcn/checkbox";
import { Input } from "../../components/shadcn/input";
import { useAccountRole } from "../accounts/roles";
import { useWorkspaceIdentity } from "../accounts/ProtectedWorkspace";

const defaultRule = "未选择角色时，由已启用且不是申请人的 ADMIN 默认审批。无人符合时不能提交。";
export function TableApprovalDrawer({ tableName }: { tableName: string }) {
  const allowed = useAccountRole("ADMIN"), accountID = useWorkspaceIdentity()?.account.id, navigate = useNavigate();
  const assignment = useQuery({ queryKey: ["table-approval", accountID, tableName], queryFn: () => tableApprovals.get(tableName), enabled: allowed });
  const close = () => navigate("/platform/table-policies");
  if (assignment.data) return <TableApprovalEditor key={`${accountID}:${tableName}`} initial={assignment.data} onClose={close} />;
  return <Drawer open title="表审批角色" eyebrow={tableName} onClose={close} footer={<Button onClick={close}>关闭</Button>}>
    {!allowed ? <p role="alert">仅管理员可配置表审批角色。</p> : assignment.isError ? <ErrorState error={assignment.error} onRetry={() => void assignment.refetch()} /> : <LoadingState label="正在读取表审批角色…" />}
  </Drawer>;
}
function TableApprovalEditor({ initial, onClose }: { initial: TableApprovalAssignment; onClose: () => void }) {
  const allowed = useAccountRole("ADMIN"), accountID = useWorkspaceIdentity()?.account.id, client = useQueryClient(), { showToast } = useToast();
  const [baseline, setBaseline] = useState(initial), [selected, setSelected] = useState(initial.roles);
  const [error, setError] = useState<unknown>(), [readError, setReadError] = useState<unknown>();
  const [pending, setPending] = useState(false), [uncertain, setUncertain] = useState(false), [reading, setReading] = useState(false), [reviewed, setReviewed] = useState(false);
  const original = useRef<{ input: TableApprovalInput; key: string } | null>(null), busy = useRef(false);
  const conflict = error instanceof ApiError && error.code === "table_approval_conflict";
  const dirty = selected.map(role => role.id).sort().join() !== [...baseline.role_ids].sort().join();
  const protection = useDraftProtection(dirty || uncertain, pending);
  const close = () => { if (!busy.current) protection.requestLeave(onClose); };
  const save = async () => {
    if (!allowed || busy.current || reading || conflict) return;
    busy.current = true; setPending(true); setError(undefined);
    original.current ??= { input: { expected_version: baseline.version, role_ids: selected.map(role => role.id) }, key: crypto.randomUUID() };
    try {
      await tableApprovals.save(initial.table_name, original.current.input, original.current.key);
      void client.invalidateQueries({ queryKey: ["table-approval", accountID, initial.table_name] });
      void client.invalidateQueries({ queryKey: ["approval-roles", accountID] });
      protection.afterSave(() => { showToast("表审批角色已保存。"); onClose(); });
    } catch (cause) {
      setError(cause);
      const definite = cause instanceof ApiError && ((cause.status >= 400 && cause.status < 500) || cause.code === "approval_role_not_saved");
      const keep = !definite || (uncertain && cause instanceof ApiError && (cause.status === 401 || cause.status === 403 || cause.code === "approval_role_not_saved"));
      setUncertain(keep); if (!keep) original.current = null;
    } finally { busy.current = false; setPending(false); }
  };
  const inspect = async () => {
    if (!allowed || reading || uncertain) return;
    setReading(true); setReadError(undefined);
    try { setBaseline(await tableApprovals.get(initial.table_name)); setReviewed(true); setError(undefined); original.current = null; }
    catch (cause) { setReadError(cause); } finally { setReading(false); }
  };
  return <Drawer open title="表审批角色" eyebrow={initial.table_name} onClose={close} footer={<><Button disabled={pending} onClick={close}>关闭</Button><Button variant="primary" disabled={!allowed || pending || reading || conflict || (!dirty && !uncertain)} onClick={() => void save()}>{pending ? "正在保存…" : uncertain ? "使用原请求重试" : "保存表审批角色"}</Button></>}>
    <p className="mb-4 break-all">配置表：<code>{initial.table_name}</code></p>
    <p className="mb-4 text-muted-foreground">为此表选择零个或多个审批角色，任一合格角色成员即可通过此表。保存只影响之后提交的申请。</p>
    {!allowed && <p role="alert" className="inline-alert">管理员权限已撤销，选择保留，暂不能保存。</p>}
    <p className="mb-5">{defaultRule}</p>
    <ApprovalRolePicker selected={selected} onChange={value => { setSelected(value); if (!conflict) setError(undefined); }} disabled={!allowed || pending || uncertain || reading} />
    {Boolean(error) && <ErrorState error={error} />}
    {uncertain && <p role="alert" className="inline-alert">保存结果待确认。选择与原请求已保留，请使用原请求重试。</p>}
    {conflict && <div className="inline-alert"><p>其他管理员已修改表审批角色。你的选择已保留，请读取最新分配后重新审阅。</p><Button disabled={!allowed || reading} onClick={() => void inspect()}>{reading ? "正在读取…" : "查看最新审批分配"}</Button></div>}
    {Boolean(readError) && <ErrorState error={readError} />}
    {reviewed && <section aria-label="服务器最新审批分配" className="mt-5 rounded-lg border p-4 break-all"><h2 className="font-semibold">服务器最新分配 · 版本 {baseline.version}</h2><p>{baseline.roles.map(role => role.name).join("、") || "无角色 · 默认 ADMIN 审批"}</p><p className="mt-2 text-muted-foreground">上方保留你的选择，保存将基于此版本提交。</p></section>}
  </Drawer>;
}
function ApprovalRolePicker({ selected, onChange, disabled }: { selected: ApprovalRoleIdentity[]; onChange: (roles: ApprovalRoleIdentity[]) => void; disabled: boolean }) {
  const accountID = useWorkspaceIdentity()?.account.id;
  const [input, setInput] = useState(""), [query, setQuery] = useState(""), [cursor, setCursor] = useState("");
  const list = useQuery({ queryKey: ["table-approval-role-options", accountID, query, cursor], queryFn: () => approvalRoles.list(query, cursor), enabled: !disabled });
  const chosen = new Set(selected.map(role => role.id));
  return <div className="grid min-w-0 gap-5">
    <section aria-label="已选择审批角色"><h2 className="font-semibold">已选择 {selected.length} 个审批角色</h2>{selected.length ? <ul className="mt-3 grid gap-2">{selected.map(role => <li key={role.id} className="flex min-w-0 items-start justify-between gap-3 rounded-lg border p-3"><div className="min-w-0 break-all"><strong>{role.name}</strong><p className="text-xs text-muted-foreground">{role.id}</p></div><Button variant="ghost" disabled={disabled} aria-label={`移除审批角色 ${role.name}`} onClick={() => onChange(selected.filter(item => item.id !== role.id))}>移除</Button></li>)}</ul> : <p className="mt-2 text-muted-foreground">无角色 · 默认 ADMIN 审批</p>}</section>
    <div className="flex flex-wrap items-end gap-3"><label className="grid min-w-0 flex-1 gap-2">检索审批角色<Input value={input} disabled={disabled} maxLength={200} onChange={event => setInput(event.target.value)} onKeyDown={event => { if (event.key === "Enter") { event.preventDefault(); setQuery(input.trim()); setCursor(""); } }} /></label><Button disabled={disabled} onClick={() => { setQuery(input.trim()); setCursor(""); }}>查询角色</Button></div>
    {list.isPending ? <LoadingState label="正在读取审批角色…" /> : <>{list.isError && <ErrorState error={list.error} onRetry={() => void list.refetch()} />}{list.data && <>
      <ul aria-label="可选审批角色" className="max-h-72 overflow-y-auto rounded-lg border divide-y">{list.data.roles.map(role => <li key={role.id}><label className="flex items-start gap-3 p-3"><Checkbox className="mt-1" aria-label={`选择审批角色 ${role.name}`} checked={chosen.has(role.id)} disabled={disabled} onCheckedChange={checked => onChange(checked ? [...selected, { id: role.id, name: role.name }] : selected.filter(item => item.id !== role.id))} /><span className="min-w-0 break-all"><strong>{role.name}</strong><span className="block text-muted-foreground">{role.enabled ? "启用" : "停用"} · {role.members.length} 名成员</span><small>{role.id}</small></span></label></li>)}</ul>
      {list.data.roles.length === 0 && <p>没有匹配的审批角色。可调整检索条件或在角色管理中创建。</p>}
      <div className="flex flex-wrap gap-2"><Button disabled={disabled || !cursor} onClick={() => setCursor("")}>审批角色首页</Button><Button disabled={disabled || !list.data.next_cursor} onClick={() => setCursor(list.data.next_cursor)}>更多审批角色</Button></div>
    </>}</>}
  </div>;
}
