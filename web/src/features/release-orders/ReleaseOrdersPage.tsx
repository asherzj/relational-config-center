import {ApiError,shouldRetryQuery} from "../../api/client";
import {presentError} from "../../api/error-messages";
import {ReleaseActionDialog,CopyDraftDialog} from "./ReleaseActionDialog";
import {ReleaseRecovery} from "./ReleaseRecovery";
import {ReleaseDraftEditor} from "./ReleaseDraftEditor";
import {useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {Link,useParams} from "react-router-dom";
import {releaseOrders,type ReleaseStateAction} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {Input} from "../../components/shadcn/input";
import {NativeSelect} from "../../components/shadcn/native-select";
import {Table,TableHeader,TableBody,TableRow,TableHead,TableCell} from "../../components/shadcn/table";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {useAccountRole} from "../accounts/roles";
import {PublicationResult} from "./PublicationResult";
import {ReleaseDiff} from "./ReleaseDiff";
import {ReleasePerson} from "./ReleasePerson";

export const releaseStateLabels={DRAFT:"草稿",PENDING_APPROVAL:"待审批",APPROVED:"已批准",SUCCEEDED:"已发布",REJECTED:"已拒绝",CANCELLED:"已取消",ROLLED_BACK:"已回滚"};
export function ReleaseOrdersPage(){
 const {id}=useParams();
 return <main className="workspace"><ReleaseRecovery/><div className="page-heading"><div><h1>发布单</h1><p>配置经独立审批后发布；发布成功表示数据库已提交，分发尚未接入。</p></div><Link to="/configuration/managed-data" className="button">编辑配置</Link></div>{id?<ReleaseDetail key={id} id={id}/>:<ReleaseList/>}</main>;
}
function ReleaseList(){
 const [input,setInput]=useState({table_name:"",applicant_id:"",state:"",id:""});
 const [filters,setFilters]=useState<Record<string,string>>({});
 const list=useQuery({queryKey:["release-orders",filters],queryFn:()=>releaseOrders.list(filters),retry:shouldRetryQuery});
 return <><form className="flex flex-wrap items-end gap-3 mb-6" onSubmit={event=>{event.preventDefault();setFilters({...input,after:""})}}>
  <label>表名<Input value={input.table_name} onChange={e=>setInput({...input,table_name:e.target.value})}/></label>
  <label>申请人账号 ID<Input value={input.applicant_id} onChange={e=>setInput({...input,applicant_id:e.target.value})}/></label>
  <label>单号<Input value={input.id} onChange={e=>setInput({...input,id:e.target.value})}/></label>
  <label>状态<NativeSelect aria-label="状态" value={input.state} onChange={e=>setInput({...input,state:e.target.value})}><option value="">全部状态</option>{Object.entries(releaseStateLabels).map(([value,label])=><option key={value} value={value}>{label}</option>)}</NativeSelect></label>
  <Button type="submit">查询发布单</Button><Button onClick={()=>void list.refetch()}>刷新列表</Button>
 </form>{list.isPending?<LoadingState/>:list.isError?<ErrorState error={list.error} onRetry={()=>void list.refetch()}/>:<>
 <div className="table-scroll"><Table className="min-w-[760px]"><TableHeader><TableRow><TableHead>标题 / 单号 / 表</TableHead><TableHead>申请人</TableHead><TableHead>状态</TableHead><TableHead>变更</TableHead></TableRow></TableHeader><TableBody>{list.data.orders.map(order=><TableRow key={order.id}><TableCell><Link className="font-medium" to={`/configuration/release-orders/${order.id}`}>{order.title}</Link><p className="break-all text-xs text-muted-foreground">{order.id} · {order.table_name}</p></TableCell><TableCell className="whitespace-nowrap">{order.applicant_id}</TableCell><TableCell>{releaseStateLabels[order.state]}</TableCell><TableCell>{order.item_count} 项 · {Object.entries(order.operation_counts).map(([operation,count])=>`${operation} ${count}`).join("、")}</TableCell></TableRow>)}</TableBody></Table></div>
 {list.data.orders.length===0&&<p className="feedback-state">没有符合条件的发布单。</p>}
 <footer className="catalog-footer"><Button disabled={!filters.after} onClick={()=>setFilters({...filters,after:""})}>回到首页</Button><Button disabled={!list.data.next_cursor} onClick={()=>setFilters({...filters,after:list.data.next_cursor})}>下一页</Button></footer>
 </>}</>;
}
function ReleaseDetail({id}:{id:string}){
 const query=useQuery({queryKey:["release-order",id],queryFn:()=>releaseOrders.get(id),retry:shouldRetryQuery});
 const people=useQuery({queryKey:["release-order-people",id],queryFn:()=>releaseOrders.people(id),enabled:query.isSuccess,retry:shouldRetryQuery});
 const [action,setAction]=useState<ReleaseStateAction>();
 const [copy,setCopy]=useState(false);
 const [editing,setEditing]=useState(false);
 const canEdit=useAccountRole("EDITOR");
 const canPublish=useAccountRole("PUBLISHER");
 const canApprove=useAccountRole("APPROVER");
 if(query.isPending)return <LoadingState/>;
 if(query.isError)return <ErrorState error={query.error} onRetry={()=>void query.refetch()}/>;
 const order=query.data;
 const peopleFailure=people.isError?presentError(people.error):undefined;
 const peopleCode=people.error instanceof ApiError?people.error.code:"unknown_error";
 const names=people.isError?{}:people.data?.people??{};
 const reverse=Boolean(order.rollback_of_id);
 return <><Link to="/configuration/release-orders">返回发布单列表</Link><section className="my-6 break-all"><h2 className="text-xl font-semibold">{order.title}</h2><p>{order.table_name} · {releaseStateLabels[order.state]}</p><p>单号：{order.id}</p><div className="my-2 flex items-center gap-2"><span>申请人：</span><ReleasePerson id={order.applicant_id} name={names[order.applicant_id]}/></div><p>发布单版本：{order.version}</p><div className="flex flex-wrap gap-3 mt-4"><Button onClick={()=>{void query.refetch();void people.refetch()}}>重新读取发布单</Button>{!reverse&&canEdit&&order.allowed_actions.includes("edit")&&<><Button onClick={()=>setEditing(true)}>编辑草稿</Button><Link className="button" to={`/configuration/managed-data?table_name=${encodeURIComponent(order.table_name)}&draft=${order.id}`}>添加明细</Link></>}{canEdit&&order.allowed_actions.includes("submit")&&<Button variant="primary" onClick={()=>setAction("submit")}>提交审批</Button>}{canApprove&&order.allowed_actions.includes("approve")&&<Button variant="primary" onClick={()=>setAction("approve")}>批准发布单</Button>}{canApprove&&order.allowed_actions.includes("reject")&&<Button onClick={()=>setAction("reject")}>拒绝发布单</Button>}{canEdit&&order.allowed_actions.includes("cancel")&&<Button onClick={()=>setAction("cancel")}>{order.state==="DRAFT"?"取消草稿":"取消发布单"}</Button>}{canPublish&&order.allowed_actions.includes("execute")&&<Button variant="primary" onClick={()=>setAction("execute")}>执行发布</Button>}{!reverse&&canEdit&&order.allowed_actions.includes("copy")&&<Button onClick={()=>setCopy(true)}>复制新草稿</Button>}{canEdit&&!order.rollback_pending&&order.allowed_actions.includes("rollback")&&<Button variant="primary" onClick={()=>setAction("rollback")}>申请回滚</Button>}</div></section>
 {peopleFailure&&<section className="inline-alert mb-4" role="alert"><div><strong>人员姓名读取失败，当前仅显示永久账号 ID。</strong><span>{peopleFailure.message}</span><span>错误代码：{peopleCode}</span>{peopleFailure.requestId&&<span>请求编号：{peopleFailure.requestId}</span>}</div><Button variant="secondary" disabled={people.isFetching} onClick={()=>void people.refetch()}>{people.isFetching?"正在读取人员姓名…":"重新读取人员姓名"}</Button></section>}
 {order.rollback_pending&&<p className="inline-alert mb-4">这张已发布单已有回滚申请处理中。</p>}{order.frozen_digest&&<p className="mb-4">{reverse?"回滚意图":"提交内容"}已冻结，审批和发布以这份差异为准。</p>}{order.copied_from_id&&<p className="mb-4">复制自 <Link to={`/configuration/release-orders/${order.copied_from_id}`}>{order.copied_from_id}</Link></p>}{order.rollback_of_id&&<p className="mb-4">回滚原发布单 <Link to={`/configuration/release-orders/${order.rollback_of_id}`}>{order.rollback_of_id}</Link>；明细来自原发布的实际结果，不可编辑或复制。</p>}{order.rollback_order_id&&<p className="mb-4">最新回滚发布单 <Link to={`/configuration/release-orders/${order.rollback_order_id}`}>{order.rollback_order_id}</Link></p>}
 <ReleaseDiff order={order}/>{order.publication&&<PublicationResult result={order.publication} people={names}/>}<h2 className="text-lg font-semibold mb-3">操作历史</h2><ol className="grid gap-3">{order.history.map(event=><li key={event.version} className="border-b pb-3 break-all"><p>{event.action} · 版本 {event.version}</p><ReleasePerson id={event.actor_id} name={names[event.actor_id]}/><time className="block">{event.at}</time>{event.reason&&<p>{event.reason}</p>}{event.related_order_id&&<p>关联发布单：<Link to={`/configuration/release-orders/${event.related_order_id}`}>{event.related_order_id}</Link></p>}</li>)}</ol>
 {editing&&<ReleaseDraftEditor order={order} onClose={()=>setEditing(false)}/>}
 {action&&<ReleaseActionDialog order={order} action={action} onClose={()=>setAction(undefined)}/>}
 {copy&&<CopyDraftDialog order={order} onClose={()=>setCopy(false)}/>}</>;
}
