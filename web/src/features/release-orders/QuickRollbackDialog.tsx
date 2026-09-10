import {useQuery} from "@tanstack/react-query";
import {useId,useRef,useState} from "react";
import {useNavigate} from "react-router-dom";
import {releaseOrders,releaseRequests,type ReleaseHeader} from "../../api/release-orders";
import {DialogDescription,DialogTitle} from "../../components/shadcn/dialog";
import {Textarea} from "../../components/shadcn/textarea";
import {Button} from "../../components/ui/Button";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {ModalSurface} from "../../components/ui/ModalSurface";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useWorkspaceIdentity} from "../accounts/ProtectedWorkspace";
import {pendingReleaseRequests} from "./release-journal";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";
import {ReleaseDiff} from "./ReleaseDiff";
import {PendingIntent,pendingRequestReason} from "./ReleaseRequestReview";

export function QuickRollbackDialog({order,onClose}:{order:ReleaseHeader;onClose:()=>void}){
 const write=useReleaseWrite(`quick-rollback:${order.id}`),navigate=useNavigate();
 const [version]=useState(order.version),[reason,setReason]=useState<string>(()=>pendingRequestReason(write.storedRequest));
 const accountID=useWorkspaceIdentity()!.account.id;
 const rejected=pendingReleaseRequests(accountID).some(item=>item.scope===`quick-rollback:${order.id}`&&item.rejection);
 const allowed=useAccountRole("PUBLISHER")&&(order.allowed_actions.includes("quick-rollback")||Boolean(write.storedRequest));
 const [repeating]=useState(Boolean(write.storedRequest));
 const titleID=useId(),descriptionID=useId(),reasonID=useId(),errorID=useId();
 const cancelRef=useRef<HTMLButtonElement>(null);
 const preview=useQuery({queryKey:["quick-rollback-preview",order.id,version],queryFn:()=>releaseOrders.quickRollbackPreview(order.id,version),enabled:!repeating&&!write.unresolved&&!rejected,retry:false,refetchOnWindowFocus:false,refetchOnMount:"always"});
 const protection=useDraftProtection(!rejected&&(Boolean(reason)||write.unresolved),write.pending);
 const close=()=>protection.requestLeave(onClose);
 const reasonBytes=new TextEncoder().encode(reason).length;
 const error=reasonBytes>2000?"快速回滚原因不能超过 2000 字节。":undefined;
 return <ModalSurface open onClose={close} pending={write.pending} labelledBy={titleID} describedBy={descriptionID} dismissLabel="取消快速回滚确认" initialFocusRef={cancelRef} className="grid w-full min-w-0 grid-rows-[auto_minmax(0,1fr)_auto] gap-5 overflow-hidden p-6 sm:max-w-[980px]">
  <header className="grid gap-3"><DialogTitle id={titleID}>快速回滚 · {order.title}</DialogTitle>
  <DialogDescription id={descriptionID}>核对当前配置将恢复成的内容，一次确认整单恢复。数据库外部改动、规则或表结构变化仍可能阻止恢复。</DialogDescription></header>
  <div className="min-h-0 overflow-y-auto space-y-5">
  {write.storedRequest&&<PendingIntent item={write.storedRequest}/>}
  {preview.isFetching?<LoadingState label="正在读取整单恢复预览…"/>:preview.isError?<ErrorState error={preview.error} onRetry={()=>void preview.refetch()}/>:preview.data&&<>
   <p>本次恢复涉及全部 {preview.data.items.length.toLocaleString("en-US")} 项，无需再次审批。成功后原单标记已回滚，释放目标记录的占用。</p>
   <section aria-label="整单恢复预览" className="[&_table]:min-w-[560px]"><h3 className="mb-3 font-semibold">当前值 → 恢复值</h3><ReleaseDiff order={preview.data} beforeLabel="当前值" proposedLabel="恢复值"/></section>
  </>}
  <div className="grid gap-2"><label htmlFor={reasonID}>快速回滚原因（选填）</label><Textarea id={reasonID} disabled={write.pending||write.unresolved} value={reason} aria-invalid={Boolean(error)} aria-describedby={error?errorID:undefined} onChange={event=>setReason(event.target.value)}/>{error&&<p id={errorID} role="alert" className="text-destructive">{error}</p>}</div>
  {Boolean(write.error)&&<ErrorState error={write.error}/>} {rejected&&<p>原原因已保留。请关闭此窗口，查看最新状态与配置后重新审阅恢复预览。</p>} {write.unresolved&&<p role="alert">原恢复预览、原因与请求标识已保留；再次点击整单快速回滚将提交原请求。</p>}
  </div>
  <footer className="flex flex-wrap justify-end gap-3 border-t pt-4"><Button ref={cancelRef} onClick={close} disabled={write.pending}>取消快速回滚</Button><Button variant="danger" disabled={write.blocked||!allowed||rejected||write.pending||(!write.unresolved&&(!preview.data||preview.isFetching||preview.isError||Boolean(error)))} onClick={async()=>{
   const result=write.unresolved?await write.retry():preview.data?await write.send({...releaseRequests.quickRollback(order.id,version,preview.data.preview_digest,reason),label:`快速回滚 ${order.title}`}):undefined;
   if(result)protection.afterSave(()=>{onClose();navigate(`/configuration/release-orders/${result.id}`)});
  }}>{write.pending?"正在处理…":"确认整单快速回滚"}</Button></footer>
 </ModalSurface>;
}
