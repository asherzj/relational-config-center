import {shouldRetryQuery} from "../../api/client";
import {ReleaseRecovery} from "./ReleaseRecovery";
import {ReleaseDraftEditor} from "./ReleaseDraftEditor";
import {useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {Link,useParams} from "react-router-dom";
import {releaseOrders,releaseRequests,type ReleaseOrder} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {Input} from "../../components/shadcn/input";
import {NativeSelect} from "../../components/shadcn/native-select";
import {Table,TableHeader,TableBody,TableRow,TableHead,TableCell} from "../../components/shadcn/table";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {Drawer} from "../../components/ui/Drawer";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";
import {ReleaseDiff} from "./ReleaseDiff";

export const releaseStateLabels={DRAFT:"草稿",PENDING_APPROVAL:"待审批",APPROVED:"已批准",SUCCEEDED:"已发布",REJECTED:"已拒绝",CANCELLED:"已取消",ROLLED_BACK:"已回滚"};
export function ReleaseOrdersPage(){
 const {id}=useParams();
 return <main className="workspace"><ReleaseRecovery/><div className="page-heading"><div><h1>发布单</h1><p>草稿仅保存变更意图，实际配置尚未改变。</p></div><Link to="/configuration/managed-data" className="button">编辑配置</Link></div>{id?<ReleaseDetail key={id} id={id}/>:<ReleaseList/>}</main>;
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
 <div className="table-scroll"><Table className="min-w-[760px]"><TableHeader><TableRow><TableHead>单号 / 表</TableHead><TableHead>申请人</TableHead><TableHead>状态</TableHead><TableHead>变更</TableHead></TableRow></TableHeader><TableBody>{list.data.orders.map(order=><TableRow key={order.id}><TableCell><Link to={`/configuration/release-orders/${order.id}`}>{order.id}</Link><p>{order.table_name}</p></TableCell><TableCell className="whitespace-nowrap">{order.applicant_id}</TableCell><TableCell>{releaseStateLabels[order.state]}</TableCell><TableCell>{order.items.map(item=>item.operation).join("、")}</TableCell></TableRow>)}</TableBody></Table></div>
 {list.data.orders.length===0&&<p className="feedback-state">没有符合条件的发布单。</p>}
 <footer className="catalog-footer"><Button disabled={!filters.after} onClick={()=>setFilters({...filters,after:""})}>回到首页</Button><Button disabled={!list.data.next_cursor} onClick={()=>setFilters({...filters,after:list.data.next_cursor})}>下一页</Button></footer>
 </>}</>;
}
function ReleaseDetail({id}:{id:string}){
 const query=useQuery({queryKey:["release-order",id],queryFn:()=>releaseOrders.get(id),retry:shouldRetryQuery});
 const [cancel,setCancel]=useState(false);
 const [editing,setEditing]=useState(false);
 const canEdit=useAccountRole("EDITOR");
 if(query.isPending)return <LoadingState/>;
 if(query.isError)return <ErrorState error={query.error} onRetry={()=>void query.refetch()}/>;
 const order=query.data;
 return <><Link to="/configuration/release-orders">返回发布单列表</Link><section className="my-6 break-all"><h2 className="text-xl font-semibold">{order.table_name} · {releaseStateLabels[order.state]}</h2><p>单号：{order.id}</p><p>申请人：{order.applicant_id}</p><p>发布单版本：{order.version}</p><div className="flex gap-3 mt-4"><Button onClick={()=>void query.refetch()}>重新读取发布单</Button>{canEdit&&order.allowed_actions.includes("edit")&&<Button onClick={()=>setEditing(true)}>编辑草稿</Button>}{canEdit&&order.allowed_actions.includes("cancel")&&<Button onClick={()=>setCancel(true)}>取消草稿</Button>}</div></section>
 <ReleaseDiff order={order}/><h2 className="text-lg font-semibold mb-3">操作历史</h2><ol className="grid gap-3">{order.history.map(event=><li key={event.version} className="border-b pb-3 break-all">{event.action} · 版本 {event.version}<p>账号：{event.actor_id}</p><time>{event.at}</time>{event.reason&&<p>{event.reason}</p>}</li>)}</ol>
 {editing&&<ReleaseDraftEditor order={order} onClose={()=>setEditing(false)}/>}
 {cancel&&<CancelDraft order={order} onClose={()=>setCancel(false)}/>}</>;
}
function CancelDraft({order,onClose}:{order:ReleaseOrder;onClose:()=>void}){
 const [reason,setReason]=useState("");const write=useReleaseWrite(`cancel:${order.id}`);const allowed=useAccountRole("EDITOR");
 const protection=useDraftProtection(Boolean(reason)||write.unresolved,write.pending);
 return <Drawer open eyebrow="发布单" title="取消草稿" onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>关闭</Button><Button variant="danger" disabled={!allowed||write.pending||!reason.trim()&&!write.unresolved} onClick={async()=>{
  const result=await write.send({...releaseRequests.cancel(order.id,order.version,reason),label:`取消 ${order.id}`});
  if(result){protection.afterSave(onClose)}
 }}>{write.pending?"正在取消…":write.unresolved?"使用原请求重试":"确认取消草稿"}</Button></>}>
 <label>取消原因<Input value={reason} maxLength={2000} disabled={write.pending||write.unresolved} onChange={e=>setReason(e.target.value)}/></label>
 {Boolean(write.error)&&<ErrorState error={write.error}/>} {write.unresolved&&<p role="alert">结果待确认。原请求与原因已保留，请使用原请求重试。</p>}
 </Drawer>;
}
