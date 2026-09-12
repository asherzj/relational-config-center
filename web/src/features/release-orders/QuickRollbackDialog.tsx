import {useQuery,useQueryClient} from "@tanstack/react-query";
import {useEffect,useId,useRef,useState} from "react";
import {useNavigate} from "react-router-dom";
import {releaseOrders,releaseRequests,type QuickRollbackPreview,type ReleaseHeader} from "../../api/release-orders";
import {DialogDescription,DialogTitle} from "../../components/shadcn/dialog";
import {Textarea} from "../../components/shadcn/textarea";
import {Button} from "../../components/ui/Button";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {ModalSurface} from "../../components/ui/ModalSurface";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useWorkspaceReady} from "../accounts/ProtectedWorkspace";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";
import {ReleaseDiff} from "./ReleaseDiff";
import {ReleaseFlows} from "./ReleaseFlows";
import {PendingIntent,pendingRequestReason} from "./ReleaseRequestIntent";

export function QuickRollbackDialog({order:initialOrder,onClose}:{order:ReleaseHeader;onClose:()=>void}){
 const ready=useWorkspaceReady();
 const current=useQuery({queryKey:["release-order",initialOrder.id],queryFn:()=>releaseOrders.get(initialOrder.id),initialData:initialOrder,enabled:ready,retry:false});
 const order=current.data;
 const write=useReleaseWrite(`quick-rollback:${order.id}`),previewWrite=useReleaseWrite(`quick-rollback-preview:${order.id}`,true),navigate=useNavigate(),client=useQueryClient();
 const [reason,setReason]=useState<string>(()=>pendingRequestReason(write.storedRequest));
 const [preview,setPreview]=useState<QuickRollbackPreview>(),[reading,setReading]=useState(false),[readError,setReadError]=useState<unknown>();
 const initialized=useRef(false),saving=useRef(false);
 const canPublish=useAccountRole("PUBLISHER"),currentAllowed=canPublish&&!current.isError&&order.allowed_actions.includes("quick-rollback");
 const rejected=Boolean(write.storedRequest?.rejection),previewRejected=Boolean(previewWrite.storedRequest?.rejection);
 const pending=write.pending||previewWrite.pending||reading||current.isFetching;
 const allowed=canPublish&&(currentAllowed||Boolean(write.storedRequest));
 const titleID=useId(),descriptionID=useId(),reasonID=useId(),errorID=useId();
 const cancelRef=useRef<HTMLButtonElement>(null);
 useEffect(()=>{
  if(pending)return;
  const cancel=cancelRef.current,active=document.activeElement;
  // Initial asynchronous saving can disable the preferred cancel action. Once
  // it becomes available, restore safe focus only if the user has not moved it.
  if(cancel&&(active===document.body||active===cancel.closest('[data-modal-surface="true"]')))cancel.focus();
 },[pending]);
 const protection=useDraftProtection((rejected?reason!==pendingRequestReason(write.storedRequest):Boolean(reason))||write.unresolved||previewWrite.unresolved,pending);
 const close=()=>protection.requestLeave(onClose);
 const reasonBytes=new TextEncoder().encode(reason).length;
 const error=reasonBytes>2000?"快速回滚原因不能超过 2000 字节。":undefined;
 const savePreview=async()=>{
  if(saving.current||!canPublish||previewWrite.blocked||write.pending)return;
  saving.current=true;setReading(true);setReadError(undefined);setPreview(undefined);
  try{
   let version=order.version;
   if((rejected||previewRejected)&&!previewWrite.unresolved){
    const latest=await client.fetchQuery({queryKey:["release-order",order.id],queryFn:()=>releaseOrders.get(order.id)});
    if(!latest.allowed_actions.includes("quick-rollback"))return;
    version=latest.version;if(previewRejected)previewWrite.confirmRebuild();
   }
   const result=previewWrite.unresolved?await previewWrite.retry():await previewWrite.send({...releaseRequests.quickRollbackPreview(order.id,version),label:`保存恢复预览 ${order.title}`});
   if(result){
    // An original-key replay may describe an earlier successful save. The
    // current header remains authoritative for whether execution is available.
    await client.fetchQuery({queryKey:["release-order",order.id],queryFn:()=>releaseOrders.get(order.id)});
    setPreview(result);
   }
  }catch(cause){setReadError(cause)}finally{saving.current=false;setReading(false)}
 };
 useEffect(()=>{if(write.storedRequest?.rejection)setPreview(undefined)},[write.storedRequest?.key,write.storedRequest?.rejection]);
 useEffect(()=>{
  if(initialized.current||pending)return;
  initialized.current=true;
  if(currentAllowed&&!write.storedRequest&&!previewWrite.storedRequest)void savePreview();
 });
 const previewCurrent=Boolean(preview&&preview.expected_version===order.version&&currentAllowed);
 const historicalDescription=`这是原请求已保存的历史恢复预览。${order.state==="COMPLETED"?"原单已完结":order.state==="ROLLED_BACK"?"原单已回滚":"原单当前状态或版本已变化"}，不能用于新的回滚。`;
 return <ModalSurface open onClose={close} pending={pending} labelledBy={titleID} describedBy={descriptionID} dismissLabel="取消快速回滚确认" initialFocusRef={cancelRef} className="grid w-full min-w-0 grid-rows-[auto_minmax(0,1fr)_auto] gap-5 overflow-hidden p-6 sm:max-w-[980px]">
  <header className="grid gap-3"><DialogTitle id={titleID}>快速回滚 · {order.title}</DialogTitle>
  <DialogDescription id={descriptionID}>{preview&&!previewCurrent?historicalDescription:"核对当前配置将恢复成的内容，一次确认整单恢复。数据库外部改动、规则或表结构变化仍可能阻止恢复。"}</DialogDescription></header>
  <div className="min-h-0 overflow-y-auto space-y-5">
  {write.storedRequest&&<PendingIntent item={write.storedRequest}/>}
  {previewWrite.storedRequest&&<PendingIntent item={previewWrite.storedRequest}/>}
  {previewWrite.pending||reading?<LoadingState label="正在保存整单恢复预览…"/>:null}
  {current.isError&&<ErrorState error={current.error} onRetry={()=>void current.refetch()}/>}
  {Boolean(previewWrite.error)&&<ErrorState error={previewWrite.error}/>}{Boolean(readError)&&<ErrorState error={readError}/>}
  {previewWrite.unresolved&&<p role="alert">保存恢复预览的结果尚未确认，原发布单版本、正文与请求标识已保留；请手动再次保存恢复预览。</p>}
  {previewRejected&&<p role="alert">原恢复预览请求已被明确拒绝。原因输入保留，读取最新状态后可重新保存恢复预览。</p>}
  {!preview&&(!write.storedRequest||rejected)&&<Button disabled={pending||previewWrite.blocked||!canPublish||(!currentAllowed&&!previewWrite.storedRequest)} onClick={()=>void savePreview()}>{previewRejected?"读取最新状态并重新保存恢复预览":"保存恢复预览"}</Button>}
  {preview&&<>
   <ReleaseFlows restoration order={{release_type:preview.release_type,table_flows:preview.table_flows,missing_flow_tables:[]}}/>
   <p>本次恢复涉及全部 {preview.items.length.toLocaleString("en-US")} 项，无需再次审批。成功后原单标记已回滚，释放目标记录的占用。</p>
   <section aria-label="整单恢复预览" className="[&_table]:min-w-[560px]"><h3 className="mb-3 font-semibold">当前值 → 恢复值</h3><ReleaseDiff order={preview} beforeLabel="当前值" proposedLabel="恢复值"/></section>
  </>}
  <div className="grid gap-2"><label htmlFor={reasonID}>快速回滚原因（选填）</label><Textarea id={reasonID} disabled={write.pending||write.unresolved} value={reason} aria-invalid={Boolean(error)} aria-describedby={error?errorID:undefined} onChange={event=>setReason(event.target.value)}/>{error&&<p id={errorID} role="alert" className="text-destructive">{error}</p>}</div>
  {Boolean(write.error)&&<ErrorState error={write.error}/>} {rejected&&<p>原原因已保留。请关闭此窗口，查看最新状态与配置后重新审阅恢复预览。</p>} {write.unresolved&&<p role="alert">原恢复预览、原因与请求标识已保留；再次点击整单快速回滚将提交原请求。</p>}
  </div>
  <footer className="flex flex-wrap justify-end gap-3 border-t pt-4"><Button ref={cancelRef} onClick={close} disabled={pending}>取消快速回滚</Button><Button variant="danger" disabled={write.blocked||!allowed||pending||(!write.unresolved&&(!previewCurrent||Boolean(error)))} onClick={async()=>{
   if(rejected&&preview&&previewCurrent)write.confirmRebuild();
   const result=write.unresolved?await write.retry():preview&&previewCurrent?await write.send({...releaseRequests.quickRollback(order.id,preview.expected_version,preview.preview_digest,reason),label:`快速回滚 ${order.title}`}):undefined;
   if(result)protection.afterSave(()=>{onClose();navigate(`/configuration/release-orders/${result.id}`)});
  }}>{write.pending?"正在处理…":rejected?"确认按最新状态快速回滚":"确认整单快速回滚"}</Button></footer>
 </ModalSurface>;
}
