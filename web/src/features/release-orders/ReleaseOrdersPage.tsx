import {ReleaseNotificationRead} from "../notifications/ReleaseNotificationRead";
import {useWorkspaceReady} from "../accounts/ProtectedWorkspace";
import {useReleaseJournal} from "./useReleaseJournal";
import {ApiError,shouldRetryQuery} from "../../api/client";
import {presentError} from "../../api/error-messages";
import {ReleaseActionDialog,CopyDraftDialog,ReprepareDraftDialog} from "./ReleaseActionDialog";
import {ReleaseConflictReview} from "./ReleaseRequestReview";
import {ReleaseDraftEditor} from "./ReleaseDraftEditor";
import {useRef,useState} from "react";
import {releaseRequestOrder,releaseRequestSending} from "./release-journal";
import {useQuery} from "@tanstack/react-query";
import {Link,useParams} from "react-router-dom";
import {canReviewRelease,releaseOrders,releaseTables,type ReleaseStateAction} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {Button as PrimitiveButton} from "../../components/shadcn/button";
import {Badge} from "../../components/shadcn/badge";
import {DropdownMenu,DropdownMenuTrigger,DropdownMenuContent,DropdownMenuItem} from "../../components/shadcn/dropdown-menu";
import {ArrowLeft,ChevronDown,Copy,MoreHorizontal,Plus} from "lucide-react";
import {Input} from "../../components/shadcn/input";
import {NativeSelect} from "../../components/shadcn/native-select";
import {Table,TableHeader,TableBody,TableRow,TableHead,TableCell} from "../../components/shadcn/table";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {useAccountRole} from "../accounts/roles";
import {useToast} from "../../components/ui/Toast";
import {ReleaseTime} from "./ReleaseTime";
import {ReleaseFlows,ReleasePhase} from "./ReleaseFlows";
import {ReleaseApprovals} from "./ReleaseApprovals";
import {ReleaseHistory} from "./ReleaseHistory";
import {RollbackReason} from "./RollbackReason";
import {ReleaseReview} from "./ReleaseReview";

import {QuickRollbackDialog} from "./QuickRollbackDialog";
import {NewDraftDialog} from "./NewDraftDialog";
import {ReleasePerson} from "./ReleasePerson";
import {CurrentFieldDisplayProvider} from "../field-display/CurrentFieldDisplay";

export const releaseStateLabels={DRAFT:"草稿",PENDING_APPROVAL:"待审批",PENDING_PUBLICATION:"待发布",APPROVED:"已批准",SUCCEEDED:"已发布待完结",COMPLETED:"已完结",REJECTED:"已拒绝",CANCELLED:"已取消",ROLLED_BACK:"已回滚"};
export function ReleaseOrdersPage(){
 const {id}=useParams();
 return <main className="workspace"><ReleaseConflictReview/>{id?<ReleaseDetail key={id} id={id}/>:<><div className="page-heading"><div><h1>发布单</h1></div></div><ReleaseList/></>}</main>;
}
function ReleaseList(){
 const journal=useReleaseJournal();
 const [creating,setCreating]=useState(false);const canCreate=useAccountRole("EDITOR");
 const [input,setInput]=useState({table_name:"",applicant_id:"",state:"",id:""});
 const [filters,setFilters]=useState<Record<string,string>>({});
 const list=useQuery({queryKey:["release-orders",filters],queryFn:()=>releaseOrders.list(filters),retry:shouldRetryQuery});
 return <>{canCreate&&<Button className="mb-4" variant="primary" disabled={journal.pending} onClick={()=>setCreating(true)}>新建草稿</Button>}{creating&&<NewDraftDialog onClose={()=>setCreating(false)}/>}<form className="flex flex-wrap items-end gap-3 mb-6" onSubmit={event=>{event.preventDefault();setFilters({...input,after:""})}}>
  <label>表名<Input value={input.table_name} onChange={e=>setInput({...input,table_name:e.target.value})}/></label>
  <label>申请人账号 ID<Input value={input.applicant_id} onChange={e=>setInput({...input,applicant_id:e.target.value})}/></label>
  <label>单号<Input value={input.id} onChange={e=>setInput({...input,id:e.target.value})}/></label>
  <label>状态<NativeSelect aria-label="状态" value={input.state} onChange={e=>setInput({...input,state:e.target.value})}><option value="">全部状态</option>{Object.entries(releaseStateLabels).map(([value,label])=><option key={value} value={value}>{label}</option>)}</NativeSelect></label>
  <Button type="submit">查询发布单</Button><Button onClick={()=>void list.refetch()}>刷新列表</Button>
 </form>{list.isPending?<LoadingState/>:list.isError?<ErrorState error={list.error} onRetry={()=>void list.refetch()}/>:<>
 <div className="table-scroll"><Table className="min-w-[760px]"><TableHeader><TableRow><TableHead>标题 / 单号 / 表</TableHead><TableHead>申请人</TableHead><TableHead>状态</TableHead><TableHead>变更</TableHead><TableHead className="sticky right-0 z-10 w-32 bg-background">操作</TableHead></TableRow></TableHeader><TableBody>{list.data.orders.map(order=><TableRow key={order.id}><TableCell><Link className="font-medium" to={`/configuration/release-orders/${order.id}`}>{order.title}</Link><p className="break-all text-xs text-muted-foreground">{order.id} · {releaseTables(order).join("、")||"暂无明细表"}</p></TableCell><TableCell className="whitespace-nowrap">{order.applicant_id}</TableCell><TableCell>{releaseStateLabels[order.state]}{order.approvals.length>0&&order.state!=="DRAFT"&&<p className="mt-1 text-xs text-muted-foreground">已通过 {order.approvals.filter(table=>table.state==="APPROVED").length} / {order.approvals.length} 表</p>}</TableCell><TableCell>{order.item_count} 项 · {Object.entries(order.operation_counts).map(([operation,count])=>`${operation} ${count}`).join("、")}</TableCell><TableCell className="sticky right-0 z-10 bg-background whitespace-nowrap"><PrimitiveButton asChild variant="ghost" size="sm"><Link to={`/configuration/release-orders/${order.id}`} aria-label={`查看详情：${order.title}`}>查看详情</Link></PrimitiveButton></TableCell></TableRow>)}</TableBody></Table></div>
 {list.data.orders.length===0&&<p className="feedback-state">没有符合条件的发布单。</p>}
 <footer className="catalog-footer"><Button disabled={!filters.after} onClick={()=>setFilters({...filters,after:""})}>回到首页</Button><Button disabled={!list.data.next_cursor} onClick={()=>setFilters({...filters,after:list.data.next_cursor})}>下一页</Button></footer>
 </>}</>;
}
export function ReleaseDetail({id,listPath="/configuration/release-orders",listLabel="发布单"}:{id:string;listPath?:string;listLabel?:string}){
 const ready=useWorkspaceReady();
 const {showToast}=useToast();
 const [copyError,setCopyError]=useState(false);
 const {requests,accountID}=useReleaseJournal();
 const requestPending=requests.some(item=>releaseRequestSending(accountID,item.key)&&releaseRequestOrder(item)===id);
 const retained=(action:string)=>requests.some(item=>item.scope===`${action}:${id}`);
 const query=useQuery({queryKey:["release-order",id],queryFn:()=>releaseOrders.get(id),enabled:ready,retry:shouldRetryQuery});
 const people=useQuery({queryKey:["release-order-people",id],queryFn:()=>releaseOrders.people(id),enabled:ready&&query.isSuccess,retry:shouldRetryQuery});
 const [action,setAction]=useState<ReleaseStateAction>();
 const [copy,setCopy]=useState(false);
 const [quickRollback,setQuickRollback]=useState(false);
 const [reprepare,setReprepare]=useState(false);
 const [editing,setEditing]=useState(false);
 // The drawer must restore focus to the persistent trigger, not an unmounted menu item.
 const moreActionsRef=useRef<HTMLButtonElement>(null);
 const canEdit=useAccountRole("EDITOR");
 const canPublish=useAccountRole("PUBLISHER");

 const navigation=<Link className="mb-2 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground" to={listPath}><ArrowLeft className="size-3.5" aria-hidden="true"/>返回{listLabel}列表</Link>;
 if(query.isPending)return <div className="release-detail min-w-0">{navigation}<LoadingState/></div>;
 if(!query.data)return <div className="release-detail min-w-0">{navigation}<ErrorState error={query.error} onRetry={()=>void query.refetch()}/></div>;
 const order=query.data;
 const peopleFailure=people.isError?presentError(people.error):undefined;
 const peopleCode=people.error instanceof ApiError?people.error.code:"unknown_error";
 const names=people.isError?{}:people.data?.people??{};
 const publication=order.executions.find(execution=>execution.kind==="PUBLICATION");
 const approver=[...order.history].reverse().find(event=>event.action==="APPROVE");
 const canApprove=canReviewRelease(order,"approve")||retained("approve");
 const canReject=canReviewRelease(order,"reject")||retained("reject");
 const available=(name:string,role:boolean)=>role&&(order.allowed_actions.includes(name)||retained(name));
 const primaryCandidates=[["submit",canEdit],["approve",canApprove],["execute",canPublish],["complete",canPublish]] as const;
 const primaryAction=primaryCandidates.find(([name,role])=>role&&order.allowed_actions.includes(name))?.[0]??primaryCandidates.find(([name,role])=>available(name,role))?.[0];
 const actionVariant=(name:string)=>primaryAction===name?"primary":"secondary";
 const canCancel=available("cancel",canEdit);
 const statusClass=order.state==="DRAFT"||order.state==="PENDING_APPROVAL"?"status-draft":["REJECTED","CANCELLED","ROLLED_BACK"].includes(order.state)?"bg-danger-soft text-destructive":"status-active";
 return <CurrentFieldDisplayProvider tableNames={releaseTables(order)}><div className="release-detail min-w-0">
 {query.isError&&<ErrorState error={query.error} onRetry={()=>void query.refetch()}/>}
 <ReleaseNotificationRead key={`${order.id}:${order.notification.sequence}`} order={order} canAcknowledge={query.isFetchedAfterMount&&query.isSuccess&&!query.isFetching} onRefresh={()=>void query.refetch()}/>
 <header className="min-w-0" aria-label="发布单页头">
  <div className="flex min-w-0 flex-wrap items-start justify-between gap-4">
   <div className="min-w-0 flex-1 basis-72">
    {navigation}
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2"><h1 className="min-w-0 break-all text-2xl font-semibold">{order.title}</h1><Badge variant="outline" aria-label="发布单状态" className={`status-badge ${statusClass}`}>{releaseStateLabels[order.state]}</Badge></div>
   </div>
   <div className="flex max-w-full flex-wrap items-center gap-2 sm:pt-6" role="group" aria-label="发布操作">
    {available("edit",canEdit)&&<Button disabled={requestPending} onClick={()=>setEditing(true)}>编辑草稿</Button>}
    {available("submit",canEdit)&&<Button variant={actionVariant("submit")} disabled={requestPending||order.item_count===0} onClick={()=>setAction("submit")}>{order.release_type==="EMERGENCY"?"提交应急发布":"提交审批"}</Button>}
    {available("approve",canApprove)&&<Button variant={actionVariant("approve")} disabled={requestPending} onClick={()=>setAction("approve")}>批准发布单</Button>}
    {available("reject",canReject)&&<Button disabled={requestPending} className="text-destructive" onClick={()=>setAction("reject")}>拒绝发布单</Button>}
    {available("execute",canPublish)&&<Button variant={actionVariant("execute")} disabled={requestPending} onClick={()=>setAction("execute")}>执行发布</Button>}
    {(available("quick-rollback",canPublish)||available("quick-rollback-preview",canPublish))&&<Button variant="secondary" className="text-destructive" disabled={requestPending} onClick={()=>setQuickRollback(true)}>快速回滚</Button>}
    {available("complete",canPublish)&&<Button variant={actionVariant("complete")} disabled={requestPending} onClick={()=>setAction("complete")}>完结发布单</Button>}
    {available("reprepare",canEdit)&&<Button disabled={requestPending} onClick={()=>setReprepare(true)}>重新准备</Button>}
    {available("copy",canEdit)&&<Button disabled={requestPending} onClick={()=>setCopy(true)}>复制新草稿</Button>}
    {canCancel&&<DropdownMenu modal={false}><DropdownMenuTrigger asChild><Button ref={moreActionsRef} disabled={requestPending} icon={<MoreHorizontal aria-hidden="true"/>}>更多操作</Button></DropdownMenuTrigger><DropdownMenuContent inline align="end" className="release-actions-menu"><DropdownMenuItem variant="destructive" disabled={requestPending} onSelect={()=>{moreActionsRef.current?.focus();setAction("cancel")}}>{order.state==="DRAFT"?"取消草稿":"取消发布单"}</DropdownMenuItem></DropdownMenuContent></DropdownMenu>}
   </div>
  </div>
  <dl className="mt-4 flex min-w-0 flex-wrap items-start gap-x-6 gap-y-3 text-sm">
   <div className="flex min-w-0 items-center gap-2"><dt className="shrink-0 text-xs text-muted-foreground">申请人</dt><dd className="min-w-0"><ReleasePerson id={order.applicant_id} name={names[order.applicant_id]}/></dd></div>
   <div className="flex flex-wrap items-center gap-2"><dt className="text-xs text-muted-foreground">创建时间</dt><dd><ReleaseTime value={order.created_at}/></dd></div>
   <div className="flex items-center gap-2"><dt className="text-xs text-muted-foreground">变更数量</dt><dd>{order.item_count.toLocaleString("en-US")} 项</dd></div>
  </dl>
  <details className="group mt-4 border-t border-border pt-3" aria-label="基本信息">
   <summary className="flex w-fit cursor-pointer list-none items-center gap-1 rounded-sm text-xs text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">基本信息<ChevronDown className="size-3.5 transition-transform group-open:rotate-180" aria-hidden="true"/></summary>
   <dl className="mt-3 grid min-w-0 gap-x-6 gap-y-4 text-sm sm:grid-cols-2 [&_dt]:mb-1 [&_dt]:text-xs [&_dt]:text-muted-foreground">
    <div className="min-w-0"><dt>发布单号</dt><dd className="flex min-w-0 flex-wrap items-center gap-1"><span className="break-all font-mono">{order.id}</span><Button variant="ghost" className="icon-button size-7" aria-label="复制发布单号" icon={<Copy aria-hidden="true"/>} onClick={async()=>{try{await navigator.clipboard.writeText(order.id);setCopyError(false);showToast("已复制发布单号")}catch{setCopyError(true)}}}/></dd>{copyError&&<p role="alert">复制失败，请选择单号手动复制。</p>}</div>
    <div><dt>发布方式</dt><dd>{order.release_type==="EMERGENCY"?"应急发布":"常规发布"}</dd></div>
    <div><dt>发布单版本</dt><dd>{order.version}</dd></div>
    <div className="min-w-0"><dt>涉及表</dt><dd className="break-all">{releaseTables(order).join("、")||"暂无明细表"}</dd></div>
    {order.copied_from_id&&<div className="min-w-0"><dt>{order.history[0]?.action==="REPREPARE"?"重新准备自":"复制自"}</dt><dd><Link className="break-all font-mono underline underline-offset-4" to={`/configuration/release-orders/${order.copied_from_id}`}>{order.copied_from_id}</Link></dd></div>}
    {order.release_type==="STANDARD"&&<div className="min-w-0"><dt>审批人</dt><dd>{approver?<ReleasePerson id={approver.actor_id} name={names[approver.actor_id]}/>:"尚无批准记录"}</dd></div>}
    {order.emergency_reason&&<div className="min-w-0 sm:col-span-2"><dt>应急原因</dt><dd className="whitespace-pre-wrap break-all">{order.emergency_reason}</dd></div>}
    {publication&&<div className="min-w-0"><dt>发布人</dt><dd><ReleasePerson id={publication.actor_id} name={names[publication.actor_id]}/></dd></div>}
   </dl>
  </details>
  {requestPending&&<p role="status" className="mt-3 text-sm">此单请求正在处理，请稍后再执行其他操作。</p>}
  {order.allowed_actions.length===0&&!requests.some(item=>releaseRequestOrder(item)===id)&&<p className="mt-3 text-xs text-muted-foreground">当前状态和权限下没有可执行操作。</p>}
 </header>
 {order.missing_flow_tables.length>0&&<section aria-label="流程配置未完成" className="release-panel min-w-0"><h2 className="text-lg font-semibold text-warning">流程配置未完成，暂不能提交</h2><p className="mt-2">以下表尚未保存当前发布方式的流程。请管理员检查对应模板与表关联；配置修复后，再保存草稿补齐缺失流程。已有表流程保持不变。</p><ul className="my-3 grid gap-1 break-all font-mono">{order.missing_flow_tables.map(table=><li key={table}>{table}</li>)}</ul>{canEdit&&(order.allowed_actions.includes("edit")||retained("edit"))&&<Button disabled={requestPending} onClick={()=>setEditing(true)}>保存草稿以补齐流程</Button>}</section>}
 <ReleasePhase order={order} people={names}/>
 {peopleFailure&&<section className="inline-alert mb-4 min-w-0 flex-wrap" role="alert"><div><strong>人员姓名读取失败，当前仅显示永久账号 ID。</strong><span>{peopleFailure.message}</span><span>错误代码：{peopleCode}</span>{peopleFailure.requestId&&<span>请求编号：{peopleFailure.requestId}</span>}</div><Button variant="secondary" disabled={people.isFetching} onClick={()=>void people.refetch()}>{people.isFetching?"正在读取人员姓名…":"重新读取人员姓名"}</Button></section>}
 {order.frozen_digest&&<p className="text-sm text-muted-foreground">提交内容已冻结，后续发布以这份差异为准。</p>}

 <ReleaseFlows order={order} people={names}/>{order.rollback_table_flows.length>0&&<ReleaseFlows restoration order={{release_type:"EMERGENCY",table_flows:order.rollback_table_flows,missing_flow_tables:[]}} people={names}/>}{order.release_type==="STANDARD"&&<ReleaseApprovals order={order} people={names}/>}<ReleaseReview order={order} people={names} toolbarAction={available("edit",canEdit)&&(requestPending?<Button disabled icon={<Plus aria-hidden="true"/>}>添加变更</Button>:<PrimitiveButton asChild variant="outline"><Link to={`/configuration/managed-data?table_name=${encodeURIComponent(releaseTables(order)[0]??"")}&draft=${order.id}`}><Plus aria-hidden="true"/>添加变更</Link></PrimitiveButton>)}/>{order.state==="ROLLED_BACK"&&<RollbackReason order={order} people={names}/>}<ReleaseHistory order={order} people={names}/>
 {editing&&<ReleaseDraftEditor order={order} onClose={()=>setEditing(false)}/>}
 {action&&<ReleaseActionDialog order={order} action={action} onClose={()=>setAction(undefined)}/>}
 {quickRollback&&<QuickRollbackDialog order={order} onClose={()=>setQuickRollback(false)}/>}
 {copy&&<CopyDraftDialog order={order} onClose={()=>setCopy(false)}/>}
 {reprepare&&<ReprepareDraftDialog order={order} onClose={()=>setReprepare(false)}/>}</div></CurrentFieldDisplayProvider>;
}
