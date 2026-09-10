import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { approvalRoles, type ApprovalRole, type ApprovalRoleInput, type ApprovalMember } from "../../api/approval-roles";
import { accountRoles } from "../../api/account-roles";
import { ApiError } from "../../api/client";
import { useAccountRole } from "../accounts/roles";
import { useWorkspaceIdentity } from "../accounts/ProtectedWorkspace";
import { Button } from "../../components/ui/Button";
import { Drawer } from "../../components/ui/Drawer";
import { ConfirmDialog } from "../../components/ui/ConfirmDialog";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { useToast } from "../../components/ui/Toast";
import { Input } from "../../components/shadcn/input";
import { Textarea } from "../../components/shadcn/textarea";
import { Checkbox } from "../../components/shadcn/checkbox";
import { Badge } from "../../components/shadcn/badge";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "../../components/shadcn/table";

export function ApprovalRolesPage() {
  const allowed = useAccountRole("ADMIN");
  const accountID = useWorkspaceIdentity()?.account.id;
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState("");
  const [selected, setSelected] = useState<ApprovalRole | "new" | null>(null);
  const client = useQueryClient();
  const { showToast } = useToast();
  const list = useQuery({ queryKey: ["approval-roles", accountID, query, cursor], queryFn: () => approvalRoles.list(query, cursor), enabled: allowed });
  return <main className="workspace">
    <div className="page-heading"><div><h1>角色管理</h1><p>维护审批角色及成员。账号可以加入多个审批角色，全局账号权限单独管理。</p></div>
      <Button variant="primary" disabled={!allowed} onClick={() => setSelected("new")}>新建审批角色</Button></div>
    {!allowed && <p role="alert">仅管理员可管理审批角色。已输入内容保留，保存已禁用。</p>}
    <section hidden={!allowed}>
      <form className="mb-5 flex flex-wrap items-end gap-3" onSubmit={event => { event.preventDefault(); setQuery(search.trim()); setCursor(""); }}>
        <label className="grid flex-1 gap-2 min-w-48">检索审批角色<Input value={search} maxLength={200} placeholder="角色名称或永久 ID" onChange={event => setSearch(event.target.value)} /></label>
        <Button type="submit">查询角色</Button><Button onClick={() => void list.refetch()}>刷新角色</Button>
      </form>
      {list.isPending ? <LoadingState label="正在读取审批角色…" /> : <>
        {list.isError && <ErrorState error={list.error} onRetry={() => void list.refetch()} />}
        {list.data && <>
          <Table className="min-w-[640px]" containerProps={{ role: "region", "aria-label": "审批角色列表", tabIndex: 0, className: "rounded-lg border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" }}><TableHeader><TableRow><TableHead>审批角色</TableHead><TableHead>成员</TableHead><TableHead>状态</TableHead><TableHead className="sticky right-0 bg-muted">操作</TableHead></TableRow></TableHeader><TableBody>
            {list.data.roles.map(role => <TableRow key={role.id}>
              <TableCell><strong className="break-all">{role.name}</strong><p className="mt-1 whitespace-pre-wrap break-all text-muted-foreground">{role.description || "未填写说明"}</p><small className="break-all">{role.id}</small></TableCell>
              <TableCell><span>{role.members.length} 人</span><p className="mt-1 break-all text-muted-foreground">{role.members.slice(0, 3).map(member => member.display_name || member.username).join("、")}{role.members.length > 3 ? "…" : ""}</p></TableCell>
              <TableCell><Badge variant="secondary" className={role.enabled ? "bg-success-soft text-success" : undefined}>{role.enabled ? "启用" : "停用"}</Badge></TableCell>
              <TableCell className="sticky right-0 bg-card"><Button variant="ghost" aria-label={`编辑审批角色 ${role.name}`} onClick={() => setSelected(role)}>编辑角色</Button></TableCell>
            </TableRow>)}
          </TableBody></Table>
          {list.data.roles.length === 0 && <p className="feedback-state">{query ? "没有匹配的审批角色。可修改检索条件。" : "尚无审批角色。新建角色后可选择成员。"}</p>}
          <footer className="catalog-footer"><span>当前页 {list.data.roles.length} 个角色</span><Button disabled={!cursor} onClick={() => setCursor("")}>回到首页</Button><Button disabled={!list.data.next_cursor} onClick={() => setCursor(list.data.next_cursor)}>下一页</Button></footer>
        </>}
      </>}
    </section>
    {selected && <ApprovalRoleEditor key={selected === "new" ? "new" : selected.id} role={selected === "new" ? undefined : selected} onClose={() => setSelected(null)} onSaved={deleted => {
      setSelected(null); showToast(deleted ? "审批角色已删除。" : "审批角色已保存。"); void client.invalidateQueries({ queryKey: ["approval-roles", accountID] });
    }} />}
  </main>;
}

type PendingRequest = { kind: "save"; input: ApprovalRoleInput; key: string } | { kind: "delete"; version: string; key: string };
function ApprovalRoleEditor({ role, onClose, onSaved }: { role?: ApprovalRole; onClose: () => void; onSaved: (deleted: boolean) => void }) {
  const allowed = useAccountRole("ADMIN");
  const [baseline, setBaseline] = useState(role);
  const [name, setName] = useState(role?.name ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [enabled, setEnabled] = useState(role?.enabled ?? true);
  const [members, setMembers] = useState<ApprovalMember[]>(role?.members ?? []);
  const [error, setError] = useState<unknown>();
  const [readError, setReadError] = useState<unknown>();
  const [pending, setPending] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [reading, setReading] = useState(false);
  const [reviewed, setReviewed] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const request = useRef<PendingRequest | null>(null);
  const locked = useRef(false);
  const conflict = error instanceof ApiError && error.code === "approval_role_conflict";
  const dirty = name !== (baseline?.name ?? "") || description !== (baseline?.description ?? "") || enabled !== (baseline?.enabled ?? true)
    || members.map(member => member.id).sort().join() !== (baseline?.members ?? []).map(member => member.id).sort().join();
  const protection = useDraftProtection(dirty || uncertain, pending);
  const close = () => { if (!locked.current) protection.requestLeave(onClose); };
  const clearError = () => { if (!conflict) setError(undefined); };
  const submit = async (kind: "save" | "delete") => {
    if (!allowed || locked.current || reading || conflict) return;
    locked.current = true; setPending(true); setError(undefined);
    request.current ??= kind === "save" ? { kind, input: { name, description, enabled, member_ids: members.map(member => member.id), ...(baseline ? { expected_version: baseline.version } : {}) }, key: crypto.randomUUID() }
      : { kind, version: baseline!.version, key: crypto.randomUUID() };
    const original = request.current;
    try {
      const result = original.kind === "save" ? await approvalRoles.save(role?.id, original.input, original.key) : await approvalRoles.remove(role!.id, original.version, original.key);
      protection.afterSave(() => onSaved(result.deleted));
    } catch (cause) {
      setError(cause);
      // A definite version rejection resolves an undelivered request; authentication
      // rejection alone cannot resolve a previous uncertain write.
      const keep = !isDefiniteRoleRejection(cause) || (uncertain && cause instanceof ApiError && (cause.status === 401 || cause.status === 403 || cause.code === "approval_role_not_saved"));
      setUncertain(keep);
      if (!keep) request.current = null;
    } finally { locked.current = false; setPending(false); }
  };
  const loadLatest = async () => {
    if (!allowed || !role || reading || uncertain) return;
    setReading(true); setReadError(undefined);
    try { setBaseline(await approvalRoles.get(role.id)); setReviewed(true); setError(undefined); request.current = null; }
    catch (cause) { setReadError(cause); }
    finally { setReading(false); }
  };
  return <Drawer open title={role ? "编辑审批角色" : "新建审批角色"} eyebrow="角色管理" onClose={close} footer={<>
    <Button variant="primary" disabled={!allowed || pending || reading || conflict || (!uncertain && !name.trim()) || (Boolean(role) && !dirty && !uncertain)} onClick={() => void submit("save")}>{pending ? "正在保存…" : uncertain ? "使用原请求重试" : "保存审批角色"}</Button>
    <Button disabled={pending} onClick={close}>关闭</Button>
  </>}>
    {!allowed && <p className="inline-alert" role="alert">管理员权限已撤销，已输入内容保留，暂不能保存。</p>}
    <fieldset disabled={!allowed || pending || uncertain || reading} className="grid min-w-0 gap-5">
      <label className="grid gap-2">角色名称<Input value={name} maxLength={200} onChange={event => { setName(event.target.value); clearError(); }} /></label>
      <label className="grid gap-2">角色说明<Textarea aria-label="角色说明" value={description} maxLength={2000} rows={3} onChange={event => { setDescription(event.target.value); clearError(); }} /></label>
      <label className="flex items-center gap-3"><Checkbox checked={enabled} onCheckedChange={value => { setEnabled(Boolean(value)); clearError(); }} />启用此审批角色</label>
      <MemberPicker members={members} onChange={value => { setMembers(value); clearError(); }} disabled={!allowed || pending || uncertain || reading} />
    </fieldset>
    {Boolean(error) && <ErrorState error={error} />}
    {uncertain && <p className="inline-alert" role="alert">保存结果待确认。当前内容与原请求已保留，请使用原请求重试。</p>}
    {conflict && <div className="inline-alert"><p>其他管理员已修改此角色。你的输入已保留，请读取最新内容后重新审阅。</p><Button disabled={!allowed || reading} onClick={() => void loadLatest()}>{reading ? "正在读取…" : "查看最新角色内容"}</Button></div>}
    {Boolean(readError) && <ErrorState error={readError} />}
    {reviewed && baseline && <section className="mt-5 rounded-lg border p-4 break-all" aria-label="服务器最新角色"><h2 className="font-semibold">服务器最新内容 · 版本 {baseline.version}</h2><p>{baseline.name} · {baseline.enabled ? "启用" : "停用"}</p><p className="whitespace-pre-wrap">{baseline.description || "未填写说明"}</p><p>成员：{baseline.members.map(member => member.username).join("、") || "无成员"}</p><p className="mt-2 text-muted-foreground">表单保留你的输入，保存将基于此版本提交。</p></section>}
    {baseline && <section className="mt-7 border-t pt-5"><p className="break-all text-xs text-muted-foreground">永久 ID：{baseline.id} · 版本 {baseline.version}</p><p className="mt-3 text-muted-foreground">{baseline.referenced ? "此角色曾被引用，不能删除；可调整成员或停用。" : "从未被表或审批引用的角色可以删除。"}</p><Button className="mt-3" variant="danger" disabled={!allowed || pending || uncertain || reading || conflict || baseline.referenced || dirty} onClick={() => setConfirmDelete(true)}>删除审批角色</Button>{dirty && <p className="mt-2 text-xs text-muted-foreground">删除前请先保存或放弃当前修改。</p>}</section>}
    <ConfirmDialog open={confirmDelete} title="删除审批角色？" description={`将删除“${baseline?.name ?? ""}”及其成员关联。仅从未引用的角色可删除。`} confirmLabel="确认删除角色" destructive pending={pending} confirmDisabled={!allowed} onCancel={() => { if (!pending) setConfirmDelete(false); }} onConfirm={() => { setConfirmDelete(false); void submit("delete"); }} />
  </Drawer>;
}

function MemberPicker({ members, onChange, disabled }: { members: ApprovalMember[]; onChange: (members: ApprovalMember[]) => void; disabled: boolean }) {
  const [input, setInput] = useState("");
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState("");
  const accountID = useWorkspaceIdentity()?.account.id;
  const list = useQuery({ queryKey: ["approval-role-people", accountID, query, cursor], queryFn: () => accountRoles.list(query, cursor), enabled: !disabled });
  const chosen = new Set(members.map(member => member.id));
  return <section className="min-w-0 border-t pt-5" aria-labelledby="role-members-heading">
    <h2 id="role-members-heading" className="font-semibold">角色成员 · 已选择 {members.length} 人</h2>
    <p className="mt-2 text-muted-foreground">按用户名、显示名称或账号 ID 检索。加入角色不会增加编辑、发布或管理员权限。</p>
    {members.length > 0 && <ul className="mt-4 grid gap-2" aria-label="已选择成员">{members.map(member => <li key={member.id} className="flex items-start justify-between gap-3 rounded-lg border p-3"><div className="min-w-0 break-all"><strong>{member.display_name || member.username}</strong><p>{member.username} · {member.enabled ? "启用" : "停用"}</p><small>{member.id}</small></div><Button variant="ghost" aria-label={`移除成员 ${member.username}`} disabled={disabled} onClick={() => onChange(members.filter(item => item.id !== member.id))}>移除</Button></li>)}</ul>}
    <div className="mt-5 flex flex-wrap items-end gap-3"><label className="grid min-w-0 flex-1 gap-2">检索人员<Input value={input} maxLength={64} onChange={event => setInput(event.target.value)} onKeyDown={event => { if (event.key === "Enter") { event.preventDefault(); setQuery(input.trim()); setCursor(""); } }} /></label><Button disabled={disabled} onClick={() => { setQuery(input.trim()); setCursor(""); }}>查询人员</Button></div>
    {list.isPending ? <LoadingState label="正在读取人员…" /> : <>
      {list.isError && <ErrorState error={list.error} onRetry={() => void list.refetch()} />}
      {list.data && <>
        <ul className="mt-4 max-h-64 overflow-y-auto rounded-lg border divide-y" aria-label="人员检索结果">{list.data.accounts.map(member => <li key={member.id}><label className="flex items-start gap-3 p-3"><Checkbox className="mt-1" checked={chosen.has(member.id)} disabled={disabled || (!chosen.has(member.id) && members.length >= 1000)} aria-label={`选择成员 ${member.username}`} onCheckedChange={checked => onChange(checked ? [...members, member] : members.filter(item => item.id !== member.id))} /><span className="min-w-0 break-all"><strong>{member.display_name || member.username}</strong><span className="block">{member.username} · {member.enabled ? "启用" : "停用"}</span><small>{member.id}</small></span></label></li>)}</ul>
        {list.data.accounts.length === 0 && <p className="mt-3">没有匹配的本地账号。</p>}
        <div className="mt-3 flex flex-wrap gap-2"><Button disabled={disabled || !cursor} onClick={() => setCursor("")}>人员首页</Button><Button disabled={disabled || !list.data.next_cursor} onClick={() => setCursor(list.data.next_cursor)}>更多人员</Button></div>
      </>}
    </>}
  </section>;
}

function isDefiniteRoleRejection(error: unknown) {
  if (error instanceof ApiError && error.status === 503 && error.code === "approval_role_not_saved") return true;
  return error instanceof ApiError && error.status >= 400 && error.status < 500 && [
    "invalid_approval_role", "approval_role_not_found", "approval_role_conflict", "approval_role_referenced", "account_not_found", "idempotency_conflict",
    "session_invalid", "account_disabled", "csrf_invalid", "permission_denied", "invalid_request", "request_body_too_large",
  ].includes(error.code);
}
