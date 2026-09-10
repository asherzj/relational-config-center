import { useToast } from "../../components/ui/Toast";
import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { accountRoles, accountRoleSchema, type AccountRole, type RoleAccount } from "../../api/account-roles";
import { isUncertainWriteError, ApiError } from "../../api/client";
import { accountRolesChanged, roleLabels, useAccountRole } from "../accounts/roles";
import { Button } from "../../components/ui/Button";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { Input } from "../../components/shadcn/input";
import { Checkbox } from "../../components/shadcn/checkbox";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "../../components/shadcn/table";
import { useDraftProtection } from "../../components/ui/LeaveProtection";

const roleDescriptions: Record<AccountRole,string> = {
 VIEWER:"查看当前部署的全部受管表和发布历史。",
 EDITOR:"编辑配置；发布单接入后可创建和提交草稿。",
 APPROVER:"旧审批权限，不独立授予按表审批资格。",
 PUBLISHER:"发布单接入后可手动执行已批准的变更。",
 ADMIN:"管理账号角色、规则目录及配置，仍不可审批自己的发布单。",
};

export function AccountRolesPage() {
 const allowed=useAccountRole("ADMIN");
 const [input,setInput]=useState("");
 const [query,setQuery]=useState("");
 const [cursor,setCursor]=useState("");
 const [selected,setSelected]=useState<RoleAccount|null>(null);
 const {showToast}=useToast();
 const client=useQueryClient();
 const list=useQuery({queryKey:["account-roles",query,cursor],queryFn:()=>accountRoles.list(query,cursor),enabled:allowed});

 return <main className="workspace">
  <div className="page-heading"><div><h1>账号角色</h1><p>角色在当前部署全局生效，可组合分配。新注册账号默认只读。</p></div></div>
  {!allowed && <p role="alert">仅管理员可查看和分配账号角色。已输入的选择保留，保存已禁用。</p>}
  <section hidden={!allowed}>
  <form className="flex flex-wrap items-end gap-3 mb-5" onSubmit={event=>{event.preventDefault();setQuery(input.trim());setCursor("");}}>
   <label className="grid gap-2 flex-1 min-w-48">检索账号<Input value={input} maxLength={64} onChange={event=>setInput(event.target.value)} placeholder="用户名、显示名称或账号 ID" /></label>
   <Button type="submit">查询</Button><Button onClick={()=>void list.refetch()}>刷新</Button>
  </form>

  {list.isPending?<LoadingState label="正在读取账号…" />:list.isError?<ErrorState error={list.error} onRetry={()=>void list.refetch()}/>:<>
   <div className="table-scroll"><Table><TableHeader><TableRow><TableHead>账号</TableHead><TableHead>状态</TableHead><TableHead>全局角色</TableHead><TableHead>操作</TableHead></TableRow></TableHeader><TableBody>
    {list.data.accounts.map(account=><TableRow key={account.id}><TableCell><strong>{account.display_name}</strong><div>{account.username}</div><small className="break-all">{account.id}</small></TableCell><TableCell>{account.enabled?"启用":"停用"}</TableCell><TableCell>{account.roles.map(role=>roleLabels[role]).join("、")}</TableCell><TableCell><Button variant="ghost" aria-label={`管理 ${account.username} 的角色`} onClick={()=>setSelected(account)}>管理角色</Button></TableCell></TableRow>)}
   </TableBody></Table></div>
   {list.data.accounts.length===0&&<p className="feedback-state">没有匹配的账号。</p>}
   <footer className="catalog-footer"><span>当前页 {list.data.accounts.length} 个账号</span><Button disabled={!cursor} onClick={()=>setCursor("")}>回到首页</Button><Button disabled={!list.data.next_cursor} onClick={()=>setCursor(list.data.next_cursor)}>下一页</Button></footer>
  </>}
  </section>
  {selected&&<RoleEditor key={selected.id} account={selected} onClose={()=>setSelected(null)} onSaved={()=>{setSelected(null);showToast("角色已保存，后续请求立即生效。");void client.invalidateQueries({queryKey:["account-roles"]});window.dispatchEvent(new Event(accountRolesChanged));}}/>}
 </main>;
}

function RoleEditor({account,onClose,onSaved}:{account:RoleAccount;onClose:()=>void;onSaved:()=>void}) {
 const canManage=useAccountRole("ADMIN");
 const [baseline,setBaseline]=useState(account);
 const [roles,setRoles]=useState<AccountRole[]>(account.roles);
 const [error,setError]=useState<unknown>();
 const [pending,setPending]=useState(false);
 const [unresolved,setUnresolved]=useState(false);
 const [latestReadError,setLatestReadError]=useState<unknown>();
 const [readingLatest,setReadingLatest]=useState(false);
 const [before,setBefore]=useState("");
 const locked=useRef(false);
 const request=useRef<{roles:AccountRole[];version:string;key:string}|null>(null);
 const history=useQuery({queryKey:["account-role-history",account.id,before],queryFn:()=>accountRoles.history(account.id,before),enabled:canManage});
 const dirty=roles.join()!==baseline.roles.join();
 const uncertain=unresolved;
 const conflict=error instanceof ApiError&&error.code==="account_roles_conflict";
 const protection=useDraftProtection(dirty||uncertain,pending);
 const close=()=>{if(!locked.current)protection.requestLeave(onClose);};
 const save=async()=>{
  if(!canManage||locked.current||readingLatest||roles.length===0||conflict)return;
  locked.current=true;setPending(true);
  request.current??={roles:[...roles],version:baseline.version,key:crypto.randomUUID()};
  try{await accountRoles.change(account.id,request.current.roles,request.current.version,request.current.key);protection.afterSave(onSaved);}
  catch(cause){
   setError(cause);
   // A rejected retry does not establish the outcome of an earlier uncertain write.
   const keepRequest=isUncertainRoleChange(cause)||(unresolved&&cause instanceof ApiError&&(cause.status===401||cause.status===403));
   setUnresolved(keepRequest);
   if(!keepRequest)request.current=null;
  }
  finally{locked.current=false;setPending(false);}
 };
 const loadLatest=async()=>{
  if(readingLatest||!canManage)return;
  setReadingLatest(true);setLatestReadError(undefined);
  try{
   const result=await accountRoles.list(account.id);
   const latest=result.accounts.find(item=>item.id===account.id);
   if(!latest)throw new ApiError("account_not_found","account not found",404);
   setBaseline(latest);setError(undefined);setUnresolved(false);request.current=null;
  }catch(cause){setLatestReadError(cause);}
  finally{setReadingLatest(false);}
 };
 return <Drawer open title={`管理 ${account.username} 的角色`} eyebrow="账号角色" onClose={close} footer={<><Button variant="primary" disabled={!canManage||readingLatest||pending||roles.length===0||conflict||(!dirty&&!uncertain)} onClick={()=>void save()}>{pending?"正在保存…":uncertain?"使用原请求重试":"保存角色"}</Button><Button disabled={pending} onClick={close}>关闭</Button></>}>
  {!canManage&&<p className="inline-alert" role="alert">管理员角色已撤销，已输入内容保留，暂不能保存。</p>}
  <p className="mb-4 break-all">账号 ID：{account.id}</p>
  <fieldset disabled={!canManage||pending||uncertain} className="grid gap-4">
   {accountRoleSchema.options.map(role=><label key={role} className="flex items-start gap-3"><Checkbox className="mt-1" checked={roles.includes(role)} onCheckedChange={checked=>{setRoles(current=>accountRoleSchema.options.filter(item=>item===role?Boolean(checked):current.includes(item)));setError((current:unknown)=>current instanceof ApiError&&current.code==="account_roles_conflict"?current:undefined);request.current=null;}}/><span><strong>{roleLabels[role]} {role}</strong><small className="block mt-1 text-muted-foreground">{roleDescriptions[role]}</small></span></label>)}
  </fieldset>
  {Boolean(error)&&<ErrorState error={error}/>}
  {uncertain&&<p role="alert">保存结果待确认。保留当前内容，使用原请求重试可找回结果。</p>}
  {conflict&&<div className="inline-alert"><p>其他管理员已调整此账号。你的选择已保留，请先查看最新角色，再确认是否保存。</p><Button disabled={!canManage||readingLatest} onClick={()=>void loadLatest()}>{readingLatest?"正在读取…":"查看最新角色"}</Button></div>}
  {Boolean(latestReadError)&&<ErrorState error={latestReadError}/>}
  <p className="mt-4">服务器当前角色：{baseline.roles.map(role=>roleLabels[role]).join("、")}（版本 {baseline.version}）</p>
  <h2 className="mt-8 mb-3 text-lg font-semibold">角色变更历史</h2>
  {history.isPending?<LoadingState/>:history.isError?<ErrorState error={history.error}/>:<>
   {history.data.events.length===0?<p>暂无角色变更记录。</p>:<ol className="grid gap-4">{history.data.events.map(event=><li key={event.id} className="border-b pb-3 break-all"><strong>{event.before_roles.join("、")} → {event.after_roles.join("、")}</strong><p>{event.actor_kind==="maintenance"?"部署维护命令":`操作者：${event.actor_id}`}</p><time dateTime={event.created_at}>{new Date(event.created_at).toLocaleString()}</time></li>)}</ol>}
   <div className="flex gap-2 mt-3"><Button disabled={!before} onClick={()=>setBefore("")}>最新记录</Button><Button disabled={!history.data.next_cursor} onClick={()=>setBefore(history.data.next_cursor)}>更早记录</Button></div>
  </>}
 </Drawer>;
}

// A connection loss during COMMIT can be reported as 503 even after persistence.
// Role writes have durable request results, so retry them with their original key.
function isUncertainRoleChange(error:unknown){
 return isUncertainWriteError(error)||(error instanceof ApiError&&error.status===503);
}
