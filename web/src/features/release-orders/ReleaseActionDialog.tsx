import {useState} from "react";
import {useNavigate} from "react-router-dom";
import {draftFromOrder,releaseOrders,releaseRequests,releaseActionRole,type ReleaseOrder,type ReleaseStateAction,type DraftItem} from "../../api/release-orders";
import {Drawer} from "../../components/ui/Drawer";
import {Button} from "../../components/ui/Button";
import {Input} from "../../components/shadcn/input";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";
import {ReleaseDiff} from "./ReleaseDiff";

export function ReleaseActionDialog({order,action,onClose}:{order:ReleaseOrder;action:ReleaseStateAction;onClose:()=>void}){
 const [reason,setReason]=useState("");const write=useReleaseWrite(`${action}:${order.id}`);
 const navigate=useNavigate();
 const allowed=useAccountRole(releaseActionRole(action))&&order.allowed_actions.includes(action);
 const protection=useDraftProtection(Boolean(reason)||write.unresolved,write.pending);
 const labels={execute:["执行发布","确认发布到数据库"],submit:["提交审批","确认提交审批"],approve:["批准发布单","确认批准"],reject:["拒绝发布单","确认拒绝"],rollback:["申请回滚","创建回滚草稿"],cancel:order.state==="DRAFT"?["取消草稿","确认取消草稿"]:["取消发布单","确认取消发布单"]};
 const reasonBytes=new TextEncoder().encode(reason).length;
 const reasonLabel=action==="cancel"?"取消原因":action==="rollback"?"回滚原因":"审批意见";
 return <Drawer open eyebrow="发布单" title={labels[action][0]!} onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>关闭</Button><Button variant={action==="cancel"||action==="reject"?"danger":"primary"} disabled={!allowed||write.pending||(action!=="submit"&&action!=="execute"&&(!reason.trim()||action==="rollback"&&reasonBytes>2000)&&!write.unresolved)} onClick={async()=>{
  const result=await write.send({...releaseRequests.action(action,order.id,order.version,reason),label:`${labels[action][0]} ${order.id}`});if(result)protection.afterSave(()=>{onClose();if(action==="rollback")navigate(`/configuration/release-orders/${result.id}`)});
 }}>{write.pending?"正在处理…":write.unresolved?"使用原请求重试":labels[action][1]}</Button></>}>
 {action==="execute"?<p>发布将在一个事务中提交全部配置、版本和历史。成功仅表示数据库生效，分发尚未接入。</p>:action==="submit"?<p>请核对以下差异。提交后内容将被冻结，已知记录会被此单占用，等待其他审批人确认。</p>:<label>{reasonLabel}<Input aria-label={reasonLabel} value={reason} maxLength={2000} disabled={write.pending||write.unresolved} onChange={event=>setReason(event.target.value)}/>{action==="rollback"&&<small>{reasonBytes} / 2000 bytes</small>}</label>}
 <p>全部 {order.items.length.toLocaleString("en-US")} 项将一起{action==="execute"?"发布":action==="approve"?"批准":action==="submit"?"提交":action==="rollback"?"生成反向草稿":"处理"}，预览分页不改变操作范围。</p>
 <ReleaseDiff order={order}/>
 {Boolean(write.error)&&<ErrorState error={write.error}/>} {write.unresolved&&<p role="alert">结果待确认。原请求与意见已保留，请使用原请求重试。</p>}
 </Drawer>;
}
export function CopyDraftDialog({order,onClose}:{order:ReleaseOrder;onClose:()=>void}){
 const [snapshot,setSnapshot]=useState<Awaited<ReturnType<typeof releaseOrders.preview>>>();
 const [reading,setReading]=useState(false),[error,setError]=useState<unknown>();
 const write=useReleaseWrite(`copy:${order.id}`),allowed=useAccountRole("EDITOR"),navigate=useNavigate();
 const protection=useDraftProtection(write.unresolved,write.pending);
 const inspect=async()=>{setReading(true);setError(undefined);try{setSnapshot(await releaseOrders.preview(draftFromOrder(order)))}catch(cause){setError(cause)}finally{setReading(false)}};
 return <Drawer open eyebrow="发布单" title="复制新草稿" onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>关闭</Button><Button variant="primary" disabled={!allowed||reading||write.pending||!snapshot&&!write.unresolved} onClick={async()=>{
  const original=draftFromOrder(order);const items:DraftItem[]=original.items.map((item,index)=>({...item,expected_record_version:snapshot?.items[index]?.expected_record_version??item.expected_record_version}));
  const result=await write.send({...releaseRequests.copy(order.id,order.version,items),label:`复制 ${order.id}`});if(result)protection.afterSave(()=>{onClose();navigate(`/configuration/release-orders/${result.id}`)})
 }}>{write.pending?"正在保存…":write.unresolved?"使用原请求重试":"确认最新基线并复制"}</Button></>}>
 <p>原单和意见永久保留。新草稿使用你核对的当前记录基线，需要重新提交审批。</p><Button disabled={!allowed||reading||write.pending||write.unresolved} onClick={()=>void inspect()}>{reading?"正在读取…":"读取最新配置"}</Button>{reading&&<LoadingState label="正在读取最新配置…"/>}
 {snapshot&&<ReleaseDiff order={{items:snapshot.items}}/>}{Boolean(error)&&<ErrorState error={error}/>} {Boolean(write.error)&&<ErrorState error={write.error}/>} {write.unresolved&&<p role="alert">结果待确认。原复制请求已保留。</p>}
 </Drawer>;
}
