import {useReleaseJournal} from "./useReleaseJournal";
import {ApiError,shouldRetryQuery} from "../../api/client";
import {presentError} from "../../api/error-messages";
import {ReleaseActionDialog,CopyDraftDialog,ReprepareDraftDialog} from "./ReleaseActionDialog";
import {ReleaseConflictReview} from "./ReleaseRequestReview";
import {ReleaseDraftEditor} from "./ReleaseDraftEditor";
import {useState} from "react";
import {releaseRequestOrder,releaseRequestSending} from "./release-journal";
import {useQuery} from "@tanstack/react-query";
import {Link,useParams} from "react-router-dom";
import {canReviewRelease,releaseOrders,releaseTables,type ReleaseStateAction} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {Button as PrimitiveButton} from "../../components/shadcn/button";
import {Input} from "../../components/shadcn/input";
import {NativeSelect} from "../../components/shadcn/native-select";
import {Table,TableHeader,TableBody,TableRow,TableHead,TableCell} from "../../components/shadcn/table";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {useAccountRole} from "../accounts/roles";
import {useToast} from "../../components/ui/Toast";
import {ReleaseTime} from "./ReleaseTime";
import {ReleaseProgress} from "./ReleaseProgress";
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
 return <main className="workspace"><ReleaseConflictReview/><div className="page-heading"><div><h1>发布单</h1></div></div>{id?<ReleaseDetail key={id} id={id}/>:<ReleaseList/>}</main>;
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
function ReleaseDetail({id}:{id:string}){
 const {showToast}=useToast();
 const [copyError,setCopyError]=useState(false);
 const {requests,accountID}=useReleaseJournal();
 const requestPending=requests.some(item=>releaseRequestSending(accountID,item.key)&&releaseRequestOrder(item)===id);
 const retained=(action:string)=>requests.some(item=>item.scope===`${action}:${id}`);
 const query=useQuery({queryKey:["release-order",id],queryFn:()=>releaseOrders.get(id),retry:shouldRetryQuery});
 const people=useQuery({queryKey:["release-order-people",id],queryFn:()=>releaseOrders.people(id),enabled:query.isSuccess,retry:shouldRetryQuery});
 const [action,setAction]=useState<ReleaseStateAction>();
 const [copy,setCopy]=useState(false);
 const [quickRollback,setQuickRollback]=useState(false);
 const [reprepare,setReprepare]=useState(false);
 const [editing,setEditing]=useState(false);
 const canEdit=useAccountRole("EDITOR");
 const canPublish=useAccountRole("PUBLISHER");

 if(query.isPending)return <LoadingState/>;
 if(!query.data)return <ErrorState error={query.error} onRetry={()=>void query.refetch()}/>;
 const order=query.data;
 const peopleFailure=people.isError?presentError(people.error):undefined;
 const peopleCode=people.error instanceof ApiError?people.error.code:"unknown_error";
 const names=people.isError?{}:people.data?.people??{};
 const publication=order.executions.find(execution=>execution.kind==="PUBLICATION");
 const approver=[...order.history].reverse().find(event=>event.action==="APPROVE");
 return <CurrentFieldDisplayProvider tableNames={releaseTables(order)}><div className="release-detail min-w-0"><nav aria-label="发布单位置" className="text-xs text-muted-foreground">配置管理 / 发布单 / <span aria-current="page">详情</span></nav><div><Link className="underline underline-offset-4" to="/configuration/release-orders">返回发布单列表</Link></div>
 {query.isError&&<ErrorState error={query.error} onRetry={()=>void query.refetch()}/>}
 {order.state==="ROLLED_BACK"?<ReleaseProgress order={order} people={names}/>:<ReleasePhase order={order} people={names}/>}
 {order.missing_flow_tables.length>0&&<section aria-label="流程配置未完成" className="release-panel min-w-0"><h2 className="text-lg font-semibold text-warning">流程配置未完成，暂不能提交审批</h2><p className="mt-2">以下表尚未保存常规流程。请管理员检查常规模板与表关联；配置修复后，再保存草稿补齐缺失流程。已有表流程保持不变。</p><ul className="my-3 grid gap-1 break-all font-mono">{order.missing_flow_tables.map(table=><li key={table}>{table}</li>)}</ul>{canEdit&&(order.allowed_actions.includes("edit")||retained("edit"))&&<Button disabled={requestPending} onClick={()=>setEditing(true)}>保存草稿以补齐流程</Button>}</section>}
 <div className="release-detail-overview">
  <section className="release-panel min-w-0" aria-label="基本信息"><h2 className="text-xl font-semibold break-all">{order.title}</h2><p className="mt-2 mb-6 text-muted-foreground break-all">{releaseTables(order).join("、")||"暂无明细表"} · {releaseStateLabels[order.state]}</p>
   <dl className="release-info"><div><dt>发布单号</dt><dd className="font-mono break-all">{order.id}<Button variant="ghost" className="ml-1" onClick={async()=>{try{await navigator.clipboard.writeText(order.id);setCopyError(false);showToast("已复制发布单号")}catch{setCopyError(true)}}}>复制发布单号</Button>{copyError&&<p role="alert">复制失败，请选择单号手动复制。</p>}</dd></div><div><dt>发布方式</dt><dd>{order.release_type==="EMERGENCY"?"应急发布":"常规发布"}</dd></div><div><dt>申请人</dt><dd><ReleasePerson id={order.applicant_id} name={names[order.applicant_id]}/></dd></div><div><dt>创建时间</dt><dd><ReleaseTime value={order.created_at}/></dd></div>{order.release_type==="STANDARD"&&<div><dt>最近审批人</dt><dd>{approver?<ReleasePerson id={approver.actor_id} name={names[approver.actor_id]}/>:"尚无批准记录"}</dd></div>}<div><dt>发布单版本</dt><dd>{order.version}</dd></div>{order.emergency_reason&&<div><dt>应急原因</dt><dd className="whitespace-pre-wrap break-all">{order.emergency_reason}</dd></div>}{publication&&<div><dt>发布人</dt><dd><ReleasePerson id={publication.actor_id} name={names[publication.actor_id]}/></dd></div>}</dl>
  </section>
  <section className="release-panel min-w-0" aria-label="发布操作"><h2 className="text-lg font-semibold mb-4">发布操作</h2><p className="font-medium">{releaseStateLabels[order.state]}</p><p className="my-3 text-muted-foreground">本单共 {order.item_count.toLocaleString("en-US")} 项变更。{order.release_type==="EMERGENCY"?"应急提交后仍由当前发布人员手动执行，发布与取消针对整单。":"审批按本人可审批表确认，发布与取消针对整单；分页与筛选仅用于审阅。"}</p>{requestPending&&<p role="status" className="mb-3">此单请求正在处理，请稍后再执行其他操作。</p>}<div className="release-action-list">{canEdit&&(order.allowed_actions.includes("edit")||retained("edit"))&&<><Button disabled={requestPending} onClick={()=>setEditing(true)}>编辑草稿</Button>{requestPending?<Button disabled>添加明细</Button>:<Link className="button" to={`/configuration/managed-data?table_name=${encodeURIComponent(releaseTables(order)[0]??"")}&draft=${order.id}`}>添加明细</Link>}</>}{canEdit&&(order.allowed_actions.includes("submit")||retained("submit"))&&<Button variant="primary" disabled={requestPending||order.item_count===0} onClick={()=>setAction("submit")}>{order.release_type==="EMERGENCY"?"提交应急发布":"提交审批"}</Button>}{(canReviewRelease(order,"approve")||retained("approve"))&&<Button variant="primary" disabled={requestPending} onClick={()=>setAction("approve")}>批准发布单</Button>}{(canReviewRelease(order,"reject")||retained("reject"))&&<Button disabled={requestPending} onClick={()=>setAction("reject")}>拒绝发布单</Button>}{canEdit&&(order.allowed_actions.includes("cancel")||retained("cancel"))&&<Button disabled={requestPending} onClick={()=>setAction("cancel")}>{order.state==="DRAFT"?"取消草稿":"取消发布单"}</Button>}{canPublish&&(order.allowed_actions.includes("execute")||retained("execute"))&&<Button variant="primary" disabled={requestPending} onClick={()=>setAction("execute")}>执行发布</Button>}{canPublish&&(order.allowed_actions.includes("quick-rollback")||retained("quick-rollback"))&&<Button variant="danger" disabled={requestPending} onClick={()=>setQuickRollback(true)}>快速回滚</Button>}{canPublish&&(order.allowed_actions.includes("complete")||retained("complete"))&&<Button variant="primary" disabled={requestPending} onClick={()=>setAction("complete")}>完结发布单</Button>}{canEdit&&(order.allowed_actions.includes("reprepare")||retained("reprepare"))&&<Button disabled={requestPending} onClick={()=>setReprepare(true)}>重新准备</Button>}{canEdit&&(order.allowed_actions.includes("copy")||retained("copy"))&&<Button disabled={requestPending} onClick={()=>setCopy(true)}>复制新草稿</Button>}</div>{order.allowed_actions.length===0&&<p className="text-muted-foreground">当前状态和权限下没有可执行操作。</p>}</section>
 </div>
 {peopleFailure&&<section className="inline-alert mb-4 min-w-0 flex-wrap" role="alert"><div><strong>人员姓名读取失败，当前仅显示永久账号 ID。</strong><span>{peopleFailure.message}</span><span>错误代码：{peopleCode}</span>{peopleFailure.requestId&&<span>请求编号：{peopleFailure.requestId}</span>}</div><Button variant="secondary" disabled={people.isFetching} onClick={()=>void people.refetch()}>{people.isFetching?"正在读取人员姓名…":"重新读取人员姓名"}</Button></section>}
 {order.frozen_digest&&<p className="mb-4">提交内容已冻结，审批和发布以这份差异为准。</p>}{order.copied_from_id&&<p className="mb-4">{order.history[0]?.action==="REPREPARE"?"重新准备自":"复制自"} <Link to={`/configuration/release-orders/${order.copied_from_id}`}>{order.copied_from_id}</Link></p>}

 <ReleaseFlows order={order} people={names}/>{order.release_type==="STANDARD"&&<ReleaseApprovals order={order} people={names}/>}<ReleaseReview order={order} people={names}/>{order.state==="ROLLED_BACK"&&<RollbackReason order={order} people={names}/>}<ReleaseHistory order={order} people={names}/>
 {editing&&<ReleaseDraftEditor order={order} onClose={()=>setEditing(false)}/>}
 {action&&<ReleaseActionDialog order={order} action={action} onClose={()=>setAction(undefined)}/>}
 {quickRollback&&<QuickRollbackDialog order={order} onClose={()=>setQuickRollback(false)}/>}
 {copy&&<CopyDraftDialog order={order} onClose={()=>setCopy(false)}/>}
 {reprepare&&<ReprepareDraftDialog order={order} onClose={()=>setReprepare(false)}/>}</div></CurrentFieldDisplayProvider>;
}
