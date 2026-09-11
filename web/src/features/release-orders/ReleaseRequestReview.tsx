import {useReleaseJournal} from "./useReleaseJournal";
import {useState} from "react";
import {Link,useNavigate} from "react-router-dom";
import {canReviewRelease,loadReleaseForEdit,decodeReleaseRequest,draftFromOrder,releaseActionLabels,releaseActionRole,releaseOrders,releaseDetailTables,releaseRequests,type ReleaseHeader,type ReleaseOrder,type ReleaseRequestEnvelope} from "../../api/release-orders";
import {useAccountRole} from "../accounts/roles";
import {Button} from "../../components/ui/Button";
import {useToast} from "../../components/ui/Toast";
import {Textarea} from "../../components/shadcn/textarea";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {type PendingReleaseRequest} from "./release-journal";
import {useReleaseWrite} from "./useReleaseWrite";
import {ApprovalScope,ReleaseApprovals} from "./ReleaseApprovals";
import {ReleaseDiff} from "./ReleaseDiff";
import {QuickRollbackDialog} from "./QuickRollbackDialog";
import {PendingIntent,pendingRequestReason} from "./ReleaseRequestIntent";
export {PendingIntent,pendingRequestReason} from "./ReleaseRequestIntent";
import {CurrentFieldDisplayProvider} from "../field-display/CurrentFieldDisplay";

export function ReleaseConflictReview({scopeFilter}:{scopeFilter?:string}){
 const {requests,error,pending,reload}=useReleaseJournal();
 if(error)return <ErrorState error={error} onRetry={()=>void reload()}/>;
 if(pending&&!requests.length)return <LoadingState label="正在读取原发布请求…"/>;
 const selected=requests.filter(item=>item.rejection&&!item.scope.startsWith("quick-rollback-preview:")&&(!scopeFilter||item.scope===scopeFilter));
 if(!selected.length)return null;
 return <section className="inline-alert release-conflict-review mb-6" aria-label="申请冲突审阅"><h2>申请冲突审阅</h2>{selected.map(item=><div key={item.key} className="mt-3"><p>{item.label}</p><PendingIntent item={item}/><RejectedRequest item={item}/></div>)}</section>;
}
function requestAction(item:PendingReleaseRequest){try{return decodeReleaseRequest(item).action}catch{return undefined}}
// A definitive rejection resolves uncertainty, but never removes the only copy
// of the user's intent. Every state action is rebuilt only after current review.
function RejectedRequest({item}:{item:PendingReleaseRequest}){
 const [current,setCurrent]=useState<ReleaseOrder&Pick<ReleaseHeader,"notification">>();
 const [preview,setPreview]=useState<Awaited<ReturnType<typeof releaseOrders.preview>>>();
 const [rebuilt,setRebuilt]=useState<ReleaseRequestEnvelope>();
 const [submitReason,setSubmitReason]=useState(()=>pendingRequestReason(item));
 const [submitReviewed,setSubmitReviewed]=useState(false);
 const [needsDraftUpdate,setNeedsDraftUpdate]=useState(false);
 const [reading,setReading]=useState(false),[error,setError]=useState<unknown>();
 const [rollbackOpen,setRollbackOpen]=useState(false);
 const action=requestAction(item),allowed=useAccountRole(releaseActionRole(action??"")),write=useReleaseWrite(item.scope),navigate=useNavigate();
 const {showToast}=useToast();
 const protection=useDraftProtection(false,write.pending);
 const inspect=async()=>{
  setReading(true);setError(undefined);setRebuilt(undefined);setPreview(undefined);setNeedsDraftUpdate(false);setSubmitReviewed(false);
  try{
   const intent=decodeReleaseRequest(item);
   const header=intent.action==="create"?undefined:await releaseOrders.get(intent.id);
   const latest=header?{...await loadReleaseForEdit(header),notification:header.notification}:undefined;setCurrent(latest);
   if(intent.action==="quick-rollback-preview")return;
   if(intent.action==="execute"||intent.action==="complete"){
    if(latest?.allowed_actions.includes(intent.action))setRebuilt(releaseRequests.action(intent.action,intent.id,latest.version));
   }else if(intent.action==="quick-rollback"){
    // Reviewing a conflict is read-only. Saving or replaying a restoration
    // preview belongs to the explicit controls in the shared recovery dialog.
    return;
   }else if(intent.action==="edit-rollback-reason"){
    if(latest?.allowed_actions.includes("edit-rollback-reason"))setRebuilt(releaseRequests.rollbackReason(intent.id,intent.input.reason));
   }else if(intent.action==="cancel"||intent.action==="approve"||intent.action==="reject"){
    if(latest&&(intent.action==="cancel"?latest.allowed_actions.includes("cancel"):canReviewRelease(latest,intent.action)))setRebuilt(releaseRequests.action(intent.action,intent.id,latest.version,intent.input.reason,latest.approval_context));
   }else if(intent.action==="submit"){
    if(latest?.allowed_actions.includes("submit")){
      const snapshot=await releaseOrders.preview(draftFromOrder(latest));setPreview(snapshot);
      const changed=snapshot.items.some((entry,index)=>entry.expected_record_version!==latest.items[index]!.expected_record_version);
      const reason=latest.release_type==="EMERGENCY"?submitReason:"";
      setNeedsDraftUpdate(changed);setSubmitReviewed(!changed);if(!changed&&(latest.release_type==="STANDARD"||Boolean(reason.trim())&&Array.from(reason).length<=2000))setRebuilt(releaseRequests.action("submit",intent.id,latest.version,reason));
    }
   }else if(intent.action==="edit-details"){
    if(latest?.allowed_actions.includes("edit")){
     const snapshot=await releaseOrders.preview({items:intent.input.changes.upserts??[]});setPreview(snapshot);
     setRebuilt(releaseRequests.edit(intent.id,{...intent.input,expected_version:latest.version,changes:{...intent.input.changes,upserts:(intent.input.changes.upserts??[]).map((entry,index)=>({...entry,expected_record_version:snapshot.items[index]!.expected_record_version}))}}));
    }
   }else if(intent.action==="copy"||intent.action==="reprepare"){
    if(latest?.allowed_actions.includes(intent.action)){
     const snapshot=await releaseOrders.preview({items:intent.input.items});setPreview(snapshot);
     const items=intent.input.items.map((entry,index)=>({...entry,expected_record_version:snapshot.items[index]!.expected_record_version}));
     setRebuilt(intent.action==="copy"?releaseRequests.copy(intent.id,latest.version,items):releaseRequests.reprepare(intent.id,latest.version,items));
    }
   }else if(intent.action==="create"||latest?.allowed_actions.includes("edit")){
    const snapshot=await releaseOrders.preview(intent.input);setPreview(snapshot);
    const input={...intent.input,items:intent.input.items.map((entry,index)=>({...entry,expected_record_version:snapshot.items[index]!.expected_record_version}))};
    setRebuilt(intent.action==="create"?releaseRequests.create(input):releaseRequests.edit(intent.id,{...input,expected_version:latest!.version}));
   }
  }catch(cause){setError(cause)}finally{setReading(false)}
 };
 const submitReasonError=current?.release_type==="EMERGENCY"?(!submitReason.trim()?"应急原因必填。":Array.from(submitReason).length>2000?"应急原因不能超过 2,000 个字符。":undefined):undefined;
 const confirmLabel=action==="cancel"?`确认按最新状态取消${current?.state==="DRAFT"?"草稿":"发布单"}`:action==="create"||action==="edit"||action==="edit-details"?"确认重建并保存草稿":action==="submit"&&current?.release_type==="EMERGENCY"?"确认按最新状态提交应急发布":`确认按最新状态${action?releaseActionLabels[action]:"重建"}`;
 return <><p role="alert">服务器已明确拒绝原请求。原申请保留，请查看最新状态与配置后决定是否重建。</p>
  <Button disabled={!action||!allowed||reading||write.pending} onClick={()=>void inspect()}>{reading?"正在检查…":"查看最新状态与配置"}</Button>{reading&&<LoadingState label="正在检查最新状态与配置…"/>}
  {current&&<CurrentFieldDisplayProvider tableNames={releaseDetailTables(current)}><p>最新发布单版本：{current.version}，状态：{current.state}</p>{action==="submit"&&<p>最新发布方式：{current.release_type==="EMERGENCY"?"应急发布":"常规发布"}</p>}{(action==="approve"||action==="reject")&&<><ReleaseApprovals order={current}/>{rebuilt?<ApprovalScope context={current.approval_context} rejection={action==="reject"}/>:<p role="alert">当前已无可处理的审批范围，原意见保留。</p>}</>}<ReleaseDiff order={current}/></CurrentFieldDisplayProvider>}
  {preview&&<CurrentFieldDisplayProvider tableNames={releaseDetailTables(preview)}><p>原申请与最新记录基线的差异：</p><ReleaseDiff order={{items:preview.items}}/></CurrentFieldDisplayProvider>}
  {needsDraftUpdate&&current&&<p>草稿记录基线已变化，请先<Link to={`/configuration/release-orders/${current.id}`}>编辑草稿并核对最新配置</Link>，再重新检查提交。</p>}
  {action==="submit"&&current?.release_type==="EMERGENCY"&&!needsDraftUpdate&&<div className="grid gap-2 my-4"><label htmlFor={`rebuild-emergency-reason-${item.key}`}>重建应急原因</label><Textarea id={`rebuild-emergency-reason-${item.key}`} aria-label="重建应急原因" value={submitReason} required aria-invalid={Boolean(submitReasonError)} disabled={!submitReviewed||reading||write.pending} onChange={event=>{if(!submitReviewed||!current.allowed_actions.includes("submit"))return;const reason=event.target.value;setSubmitReason(reason);const invalid=!reason.trim()||Array.from(reason).length>2000;setRebuilt(invalid?undefined:releaseRequests.action("submit",current.id,current.version,reason))}}/><span className="text-xs text-muted-foreground">{Array.from(submitReason).length.toLocaleString("en-US")} / 2,000 字符</span>{submitReasonError&&<small className="field-error">{submitReasonError}</small>}</div>}
  {action==="quick-rollback"&&current&&!current.allowed_actions.includes("quick-rollback")&&<p>当前发布单已不能快速回滚，原原因保留供核对。</p>}
  {action==="quick-rollback"&&current&&<Button disabled={!allowed||reading||write.pending} onClick={()=>setRollbackOpen(true)}>打开恢复预览</Button>}
  {rollbackOpen&&current&&<CurrentFieldDisplayProvider tableNames={current.table_names}><QuickRollbackDialog order={current} onClose={()=>setRollbackOpen(false)}/></CurrentFieldDisplayProvider>}
  {(current||preview)&&action!=="quick-rollback"&&<Button disabled={write.blocked||!allowed||reading||write.pending||!rebuilt} onClick={async()=>{
   if(!rebuilt)return;write.confirmRebuild();const result=await write.send({...rebuilt,label:item.label});
   if(result){if(action==="submit"&&current?.release_type==="EMERGENCY")showToast("应急发布已提交");protection.afterSave(()=>navigate(`/configuration/release-orders/${result.id}`));}
  }}>{confirmLabel}</Button>}
  {Boolean(error)&&<ErrorState error={error}/>} {Boolean(write.error)&&<ErrorState error={write.error}/>}
 </>;
}
