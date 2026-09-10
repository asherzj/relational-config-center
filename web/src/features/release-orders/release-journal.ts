import {z} from "zod";
import {ApiError,isUncertainWriteError} from "../../api/client";
import {businessSession} from "../../api/business-session";
import {releaseOrders} from "../../api/release-orders";

const pendingSchema=z.object({scope:z.string(),path:z.string().regex(/^\/api\/v1\/release-orders(?:\/[a-f0-9]{32}(?:\/(?:cancel|submit|approve|reject|copy|execute|complete|quick-rollback|rollback-reason|reprepare))?)?$/),method:z.enum(["POST","PUT"]),body:z.string(),key:z.string(),label:z.string(),rejection:z.enum(["release_version_conflict","record_version_conflict","release_state_invalid","release_target_conflict","release_frozen_changed"]).optional()});
export type PendingReleaseRequest=z.infer<typeof pendingSchema>;
export const releaseJournalChanged="rcc:release-journal-changed";
// IndexedDB is the single durable authority. React reads an in-memory mirror
// only after account hydration; every write rechecks the authoritative record in
// a readwrite transaction, so another tab cannot overwrite an unresolved key.
const journalDatabase="rcc-release-requests";
const mirror=new Map<string,PendingReleaseRequest[]>();
const channel=typeof BroadcastChannel!=="undefined"?new BroadcastChannel("rcc-release-journal"):undefined;
channel?.addEventListener("message",event=>{if(typeof event.data==="string")void hydrateReleaseRequests(event.data).catch(()=>{window.dispatchEvent(new Event(releaseJournalChanged))})});
function changed(accountID:string,broadcast:boolean){window.dispatchEvent(new Event(releaseJournalChanged));if(broadcast)channel?.postMessage(accountID)}
function journalTransaction(accountID:string,change?:(requests:PendingReleaseRequest[])=>PendingReleaseRequest[]):Promise<void>{
 return new Promise((resolve,reject)=>{
  const opening=indexedDB.open(journalDatabase,1);
  opening.onupgradeneeded=()=>opening.result.createObjectStore("accounts");
  opening.onerror=()=>reject(opening.error);
  opening.onsuccess=()=>{
   const db=opening.result;
   const transaction=db.transaction("accounts",change?"readwrite":"readonly");
   const store=transaction.objectStore("accounts");
   let next:PendingReleaseRequest[]=[],failure:unknown;
   const reading=store.get(accountID);
   reading.onsuccess=()=>{
    try{
     const previous=z.array(pendingSchema).parse(reading.result??[]);
     next=change?change(previous):previous;
     if(change){if(next.length)store.put(next,accountID);else store.delete(accountID)}
    }catch(cause){failure=cause;transaction.abort()}
   };
   transaction.onabort=()=>{db.close();reject(failure??transaction.error)};
   transaction.onerror=()=>{failure??=transaction.error};
   // Request success is insufficient: the complete transaction must commit
   // before the caller can send the associated HTTP operation.
   transaction.oncomplete=()=>{db.close();mirror.set(accountID,next);changed(accountID,Boolean(change));resolve()};
  };
 });
}
type HydrationState={pending:boolean,error?:ApiError};
const hydration=new Map<string,HydrationState>();
const reads=new Map<string,Promise<void>>();
export function releaseJournalState(accountID:string):HydrationState{return hydration.get(accountID)??{pending:true}}
export function ensureReleaseRequests(accountID:string){return hydration.has(accountID)?(reads.get(accountID)??Promise.resolve()):hydrateReleaseRequests(accountID)}
export function hydrateReleaseRequests(accountID:string):Promise<void>{
 const active=reads.get(accountID);if(active)return active;
 hydration.set(accountID,{pending:!mirror.has(accountID)});changed(accountID,false);
 const read=journalTransaction(accountID).then(()=>{hydration.set(accountID,{pending:false})},cause=>{
  hydration.set(accountID,{pending:false,error:new ApiError("release_journal_unavailable","浏览器无法读取原发布请求，尚未发送。请恢复浏览器存储后重试。",0)});throw cause;
 }).finally(()=>{reads.delete(accountID);changed(accountID,false)});
 reads.set(accountID,read);return read;
}
export function pendingReleaseRequests(accountID:string):PendingReleaseRequest[]{return mirror.get(accountID)??[]}
export function rememberReleaseRequest(accountID:string,value:PendingReleaseRequest,replacesKey?:string){
 return journalTransaction(accountID,previous=>{
  for(const saved of previous){
   if(saved.key===value.key){
    if(saved.scope!==value.scope||saved.body!==value.body||saved.path!==value.path||saved.method!==value.method)throw new ApiError("idempotency_conflict","原请求标识对应的正文不能改变。",409);
   }else if(saved.scope===value.scope){
    if(!saved.rejection||saved.key!==replacesKey)throw new ApiError("release_request_pending","另一个窗口已有待处理的原请求，请先读取并恢复。",409);
   }
  }
  return [...previous.filter(item=>item.scope!==value.scope),pendingSchema.parse(value)];
 });
}
export function forgetReleaseRequest(accountID:string,key:string){
 return journalTransaction(accountID,previous=>previous.filter(item=>item.key!==key));
}
export async function sendReleaseRequest(accountID:string,value:PendingReleaseRequest){
 if(businessSession().credentials?.accountID!==accountID)throw new ApiError("stale_session","请使用原申请账号恢复此请求。",0);
 return releaseOrders.write(value.path,value.method,value.body,value.key);
}
export function uncertainReleaseError(error:unknown){return isUncertainWriteError(error)||(error instanceof ApiError&&(error.status>=500||error.status===401||error.status===403))}

// Only active sends exclude another same-order action. Durable unresolved
// requests survive reload by scope; each original key remains immutable.
const sending=new Set<string>();
export const releaseRequestOrder=(request:Pick<PendingReleaseRequest,"path">)=>request.path.match(/^\/api\/v1\/release-orders\/([a-f0-9]{32})(?:\/|$)/)?.[1];
export function conflictingReleaseRequest(accountID:string,path:string,scope:string){
 const id=releaseRequestOrder({path});
 return id&&pendingReleaseRequests(accountID).find(item=>releaseRequestSending(accountID,item.key)&&item.scope!==scope&&releaseRequestOrder(item)===id);
}
export function releaseRequestSending(accountID:string,key:string){return sending.has(`${accountID}:${key}`)}
export function startReleaseRequest(accountID:string,key:string){
 if(releaseRequestSending(accountID,key))return false;
 sending.add(`${accountID}:${key}`);window.dispatchEvent(new Event(releaseJournalChanged));return true;
}
export function finishReleaseRequest(accountID:string,key:string){sending.delete(`${accountID}:${key}`);window.dispatchEvent(new Event(releaseJournalChanged))}
