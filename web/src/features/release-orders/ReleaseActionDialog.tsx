import {useState} from "react";
import {useNavigate} from "react-router-dom";
import {canReviewRelease,decodeReleaseRequest,draftFromOrder,releaseOrders,releaseRequests,releaseActionRole,releaseActionRequiresReason,loadReleaseForEdit,type ReleaseOrder,type ReleaseHeader,type ReleaseStateAction,type DraftItem} from "../../api/release-orders";
import {Drawer} from "../../components/ui/Drawer";
import {Button} from "../../components/ui/Button";
import {ConfirmDialog} from "../../components/ui/ConfirmDialog";
import {Input} from "../../components/shadcn/input";
import {Textarea} from "../../components/shadcn/textarea";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useToast} from "../../components/ui/Toast";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";
import {PagedReleaseDetails} from "./PagedReleaseDetails";
import {ReleaseDiff} from "./ReleaseDiff";
import {ApprovalScope,ReleaseApprovals} from "./ReleaseApprovals";
import {PendingIntent,pendingRequestReason} from "./ReleaseRequestReview";

export function ReleaseActionDialog({order,action,onClose}:{order:ReleaseHeader;action:ReleaseStateAction;onClose:()=>void}){
 const write=useReleaseWrite(`${action}:${order.id}`);
 const {showToast}=useToast();
 const [reason,setReason]=useState<string>(()=>pendingRequestReason(write.storedRequest));
 const [confirmedOrder]=useState(order);
 const approval=action==="approve"||action==="reject";
 const emergencySubmit=action==="submit"&&order.release_type==="EMERGENCY";
 const reasonLength=Array.from(reason).length;
 const reasonError=emergencySubmit?(!reason.trim()?"应急原因必填。":reasonLength>2000?"应急原因不能超过 2,000 个字符。":undefined):undefined;
 const globalAllowed=useAccountRole(releaseActionRole(action));
 const allowed=approval?(canReviewRelease(order,action)||write.unresolved):globalAllowed&&(order.allowed_actions.includes(action)||Boolean(write.storedRequest));
 let original:ReturnType<typeof decodeReleaseRequest>|undefined;
 try{original=write.storedRequest?decodeReleaseRequest(write.storedRequest):undefined}catch{/* Preserve an unreadable original envelope without deriving another request. */}
 const scope=original&&(original.action==="approve"||original.action==="reject")?{approvable_tables:original.input.confirmed_tables}:confirmedOrder.approval_context;
 const protection=useDraftProtection(Boolean(reason)||write.unresolved,write.pending);
 const labels={complete:["完结发布单","确认完结"],execute:["执行发布","确认发布到数据库"],submit:emergencySubmit?["提交应急发布","确认提交待发布"]:["提交审批","确认提交审批"],approve:["批准发布单","确认批准"],reject:["拒绝发布单","确认拒绝"],cancel:order.state==="DRAFT"?["取消草稿","确认取消草稿"]:["取消发布单","确认取消发布单"]};
 const reasonLabel=action==="cancel"?"取消原因":"审批意见";
 return <Drawer open eyebrow="发布单" title={labels[action][0]!} onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>关闭</Button><Button variant={action==="cancel"||action==="reject"?"danger":"primary"} disabled={write.blocked||Boolean(write.storedRequest?.rejection)||!allowed||write.pending||(!write.unresolved&&(releaseActionRequiresReason(action)&&!reason.trim()||Boolean(reasonError)))} onClick={async()=>{
  const result=write.unresolved?await write.retry():await write.send({...releaseRequests.action(action,order.id,confirmedOrder.version,reason,confirmedOrder.approval_context),label:`${labels[action][0]} ${order.id}`});if(result){if(emergencySubmit)showToast("应急发布已提交");protection.afterSave(()=>{onClose();});}
 }}>{write.pending?"正在处理…":labels[action][1]}</Button></>}>
 {action==="complete"?<p>完结将释放全部目标记录的占用，并关闭快速回滚。配置内容保持不变，完结后不能再回滚此单。分发尚未接入。</p>:action==="execute"?<p>发布将在一个事务中提交全部配置、版本和历史。成功仅表示数据库生效，分发尚未接入。</p>:action==="submit"?<p>{emergencySubmit?"请填写应急原因并核对差异。提交后内容将被冻结并进入待发布，仍需当前发布人员手动执行；提交本身不会写入业务数据。":"请核对以下差异。提交后内容将被冻结，已知记录会被此单占用，等待其他审批人确认。"}</p>:<label>{reasonLabel}<Input aria-label={reasonLabel} value={reason} maxLength={2000} disabled={write.pending||write.unresolved} onChange={event=>setReason(event.target.value)}/></label>}
 {emergencySubmit&&<div className="grid gap-2 my-5"><label htmlFor="release-emergency-reason">应急原因</label><Textarea id="release-emergency-reason" aria-label="应急原因" value={reason} required aria-invalid={Boolean(reasonError)} aria-describedby={`release-emergency-reason-count${reasonError?" release-emergency-reason-error":""}`} disabled={write.pending||write.unresolved} onChange={event=>setReason(event.target.value)}/><span id="release-emergency-reason-count" className="text-xs text-muted-foreground">{reasonLength.toLocaleString("en-US")} / 2,000 字符</span>{reasonError&&<small id="release-emergency-reason-error" className="field-error">{reasonError}</small>}</div>}
 {approval?<ApprovalScope context={scope} rejection={action==="reject"}/>:<p>全部 {confirmedOrder.item_count.toLocaleString("en-US")} 项将一起{action==="execute"?"发布":action==="submit"?"提交":"处理"}，预览分页不改变操作范围。</p>}
 {action==="submit"&&order.release_type==="STANDARD"&&<ReleaseApprovals order={order}/>}
 {approval&&!allowed&&<p role="alert">当前已无可审批范围。原意见保留，请重新审阅当前资格。</p>}
 {write.storedRequest&&<PendingIntent item={write.storedRequest}/>}<PagedReleaseDetails order={confirmedOrder}/>
 {Boolean(write.error)&&<ErrorState error={write.error}/>} {write.storedRequest?.rejection&&<p role="alert">审批或进度已变化，原意见与确认范围已保留。请关闭窗口，在“申请冲突审阅”中查看最新内容并明确确认。</p>} {write.unresolved&&<p role="alert">原请求与意见已保留；再次点击同一操作将提交原请求。</p>}
 </Drawer>;
}
export function CopyDraftDialog({order,onClose}:{order:ReleaseHeader;onClose:()=>void}){
 const [original,setOriginal]=useState<ReleaseOrder>();
 const [snapshot,setSnapshot]=useState<Awaited<ReturnType<typeof releaseOrders.preview>>>();
 const [reading,setReading]=useState(false),[error,setError]=useState<unknown>();
 const write=useReleaseWrite(`copy:${order.id}`),allowed=useAccountRole("EDITOR"),navigate=useNavigate();
 const protection=useDraftProtection(write.unresolved,write.pending);
 const inspect=async()=>{setReading(true);setError(undefined);try{const source=await loadReleaseForEdit(order);setOriginal(source);setSnapshot(await releaseOrders.preview(draftFromOrder(source)))}catch(cause){setError(cause)}finally{setReading(false)}};
 return <Drawer open eyebrow="发布单" title="复制新草稿" onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>关闭</Button><Button variant="primary" disabled={write.blocked||!allowed||reading||write.pending||!snapshot&&!write.unresolved} onClick={async()=>{
  const items:DraftItem[]=(original?draftFromOrder(original).items:[]).map((item,index)=>({...item,expected_record_version:snapshot?.items[index]?.expected_record_version??item.expected_record_version}));
  const result=write.unresolved?await write.retry():await write.send({...releaseRequests.copy(order.id,order.version,items),label:`复制 ${order.id}`});if(result)protection.afterSave(()=>{onClose();navigate(`/configuration/release-orders/${result.id}`)})
 }}>{write.pending?"正在保存…":"确认最新基线并复制"}</Button></>}>
 <p>{order.release_type==="EMERGENCY"?"原单和历史永久保留。新草稿使用你核对的当前记录基线，需要重新填写应急原因并提交，仍由发布人员手动执行。":"原单和意见永久保留。新草稿使用你核对的当前记录基线，需要重新提交审批。"}</p><Button disabled={write.blocked||!allowed||reading||write.pending||write.unresolved} onClick={()=>void inspect()}>{reading?"正在读取…":"读取最新配置"}</Button>{reading&&<LoadingState label="正在读取最新配置…"/>}
 {write.storedRequest&&<PendingIntent item={write.storedRequest}/>}{snapshot&&<ReleaseDiff order={{items:snapshot.items}}/>}{Boolean(error)&&<ErrorState error={error}/>} {Boolean(write.error)&&<ErrorState error={write.error}/>} {write.unresolved&&<p role="alert">原复制请求已保留；再次点击复制将提交同一份申请。</p>}
 </Drawer>;
}

export function ReprepareDraftDialog({order,onClose}:{order:ReleaseHeader;onClose:()=>void}){
 const [original,setOriginal]=useState<ReleaseOrder>();
 const [snapshot,setSnapshot]=useState<Awaited<ReturnType<typeof releaseOrders.preview>>>();
 const [confirming,setConfirming]=useState(false);
 const [reading,setReading]=useState(false),[error,setError]=useState<unknown>();
 const write=useReleaseWrite(`reprepare:${order.id}`),allowed=useAccountRole("EDITOR")&&(order.allowed_actions.includes("reprepare")||Boolean(write.storedRequest)),navigate=useNavigate();
 const protection=useDraftProtection(write.unresolved,write.pending);
 const inspect=async()=>{setReading(true);setError(undefined);try{const source=await loadReleaseForEdit(order);setOriginal(source);setSnapshot(await releaseOrders.preview(draftFromOrder(source)))}catch(cause){setError(cause)}finally{setReading(false)}};
 const apply=async()=>{
  const items:DraftItem[]=(original?draftFromOrder(original).items:[]).map((item,index)=>({...item,expected_record_version:snapshot?.items[index]?.expected_record_version??item.expected_record_version}));
  const result=write.unresolved?await write.retry():await write.send({...releaseRequests.reprepare(order.id,order.version,items),label:`重新准备 ${order.id}`});if(result)protection.afterSave(()=>{setConfirming(false);onClose();navigate(`/configuration/release-orders/${result.id}`)});
 };
 return <><Drawer open eyebrow="发布单" title="重新准备" onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>关闭</Button><Button variant="danger" disabled={write.blocked||!allowed||reading||write.pending||!snapshot&&!write.unresolved} onClick={()=>setConfirming(true)}>{"继续重新准备"}</Button></>}>
 <p>{order.release_type==="EMERGENCY"?"先核对当前配置。确认后旧单会取消并释放目标；新草稿继承标题、发布方式和申请内容。原应急原因不会沿用，需要重新填写原因并由发布人员手动执行。":"先核对当前配置。确认后旧单会取消并释放目标，新草稿继承标题和申请内容，由当前操作者重新编辑、提交并接受独立审批。"}</p>
 <Button disabled={write.blocked||!allowed||reading||write.pending||write.unresolved} onClick={()=>void inspect()}>{reading?"正在读取…":"读取最新配置"}</Button>{reading&&<LoadingState label="正在读取最新配置…"/>}
 {write.storedRequest&&<PendingIntent item={write.storedRequest}/>}{snapshot&&<><p>新草稿将采用以下当前记录基线：</p><ReleaseDiff order={{items:snapshot.items}}/></>}{Boolean(error)&&<ErrorState error={error}/>} {Boolean(write.error)&&<ErrorState error={write.error}/>} {write.unresolved&&<p role="alert">原重新准备请求已保留；再次点击原操作将提交同一份申请。</p>}
 </Drawer><ConfirmDialog open={confirming} title="取消旧单并创建新草稿？" description={order.release_type==="EMERGENCY"?"确认后，旧应急发布单会被取消并释放全部目标；系统会创建由你申请的新应急草稿，原应急原因不会沿用。":"确认后，已批准的旧发布单会被取消并释放全部目标；系统会创建由你申请的新草稿，旧审批不会沿用。"} confirmLabel={"取消旧单并创建新草稿"} destructive pending={write.pending} confirmDisabled={!allowed||!snapshot&&!write.unresolved} onCancel={()=>setConfirming(false)} onConfirm={()=>void apply()}>{Boolean(write.error)&&<ErrorState error={write.error}/>} {write.unresolved&&<p role="alert">原正文和请求标识已保留；再次点击原操作将提交同一份申请。</p>}</ConfirmDialog></>;
}
