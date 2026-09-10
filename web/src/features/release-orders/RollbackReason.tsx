import {useId,useRef,useState} from "react";
import type {ReleaseOrder} from "../../api/release-orders";
import {releaseRequests} from "../../api/release-orders";
import {DialogDescription,DialogTitle} from "../../components/shadcn/dialog";
import {Textarea} from "../../components/shadcn/textarea";
import {Button} from "../../components/ui/Button";
import {ErrorState} from "../../components/ui/Feedback";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {ModalSurface} from "../../components/ui/ModalSurface";
import {PendingIntent,pendingRequestReason} from "./ReleaseRequestReview";
import {ReleasePerson} from "./ReleasePerson";
import {ReleaseTime} from "./ReleaseTime";
import {useReleaseWrite} from "./useReleaseWrite";

export function RollbackReason({order,people}:{order:ReleaseOrder;people:Record<string,string>}) {
 const [editing,setEditing]=useState(false);
 const revisions=order.history.filter(event=>event.action==="ROLLBACK_REASON");
 const current=revisions.at(-1)??[...order.history].reverse().find(event=>event.action==="QUICK_ROLLBACK");
 const hasRecordedReason=revisions.length>0||Boolean(current?.reason);
 const mayEdit=order.allowed_actions.includes("edit-rollback-reason");
 return <><section className="release-panel" aria-label="回滚原因">
  <div className="flex flex-wrap items-start justify-between gap-3">
   <div className="min-w-0"><h2 className="text-lg font-semibold">回滚原因</h2><p className="mt-2 whitespace-pre-wrap break-all">{current?.reason?current.reason:revisions.length?"已清空回滚原因。":"尚未填写回滚原因。"}</p>
   {current&&hasRecordedReason&&<p className="mt-2 text-xs text-muted-foreground"><ReleasePerson id={current.actor_id} name={people[current.actor_id]}/> · <ReleaseTime value={current.at}/></p>}</div>
   {mayEdit&&<Button onClick={()=>setEditing(true)}>{hasRecordedReason?"修改回滚原因":"补填回滚原因"}</Button>}
  </div>
  <p className="mt-3 text-xs text-muted-foreground">原因选填，没有期限，不改变回滚执行结果或后续发布。</p>
 </section>{editing&&<RollbackReasonDialog order={order} initialReason={current?.reason??""} first={!hasRecordedReason} onClose={()=>setEditing(false)}/>}</>;
}

function RollbackReasonDialog({order,initialReason,first,onClose}:{order:ReleaseOrder;initialReason:string;first:boolean;onClose:()=>void}) {
 const write=useReleaseWrite(`edit-rollback-reason:${order.id}`);
 const [originalKey]=useState(write.storedRequest?.key);
 const [reason,setReason]=useState(()=>write.storedRequest?pendingRequestReason(write.storedRequest):initialReason);
 const titleID=useId(),descriptionID=useId(),reasonID=useId(),errorID=useId();
 const cancelRef=useRef<HTMLButtonElement>(null);
 const allowed=order.allowed_actions.includes("edit-rollback-reason");
 const protection=useDraftProtection(reason!==initialReason||write.unresolved,write.pending);
 const reasonBytes=new TextEncoder().encode(reason).length;
 const error=reasonBytes>2000?"回滚原因不能超过 2000 字节。":undefined;
 const title=`${first?"补填":"修改"}回滚原因 · ${order.title}`;
 const close=()=>protection.requestLeave(onClose);
 return <ModalSurface open onClose={close} pending={write.pending} labelledBy={titleID} describedBy={descriptionID} dismissLabel="关闭回滚原因编辑" initialFocusRef={cancelRef} className="grid w-full min-w-0 gap-5 p-6 sm:max-w-[640px]">
  <header className="grid gap-2"><DialogTitle id={titleID}>{title}</DialogTitle><DialogDescription id={descriptionID}>原因可以留空。每次保存都会记录当前操作者、保存时间和完整内容。</DialogDescription></header>
  {write.storedRequest&&<PendingIntent item={write.storedRequest}/>}<div className="grid min-w-0 gap-2"><label htmlFor={reasonID}>回滚原因（选填）</label><Textarea id={reasonID} value={reason} disabled={write.pending||write.unresolved} aria-invalid={Boolean(error)} aria-describedby={error?errorID:undefined} onChange={event=>setReason(event.target.value)}/><p className="text-xs text-muted-foreground">{reasonBytes} / 2000 字节</p>{error&&<p id={errorID} role="alert" className="text-destructive">{error}</p>}</div>
  {Boolean(write.error)&&<ErrorState error={write.error}/>} {write.unresolved&&<p role="alert">原回滚原因与请求标识已保留；再次点击保存将提交原请求。</p>}
  <footer className="flex flex-wrap justify-end gap-3 border-t pt-4"><Button ref={cancelRef} disabled={write.pending} onClick={close}>取消</Button><Button variant="primary" disabled={write.blocked||!allowed||write.pending||Boolean(error)} onClick={async()=>{const request={...releaseRequests.rollbackReason(order.id,reason),label:`修改回滚原因 ${order.id}`};const result=originalKey&&write.unresolved&&originalKey===write.storedRequest?.key?await write.retry():await write.send(request);if(result)protection.afterSave(onClose)}}>{write.pending?"正在保存…":"保存回滚原因"}</Button></footer>
 </ModalSurface>;
}
