import {z} from "zod";
import {ApiError,isUncertainWriteError} from "../../api/client";
import {businessSession} from "../../api/business-session";
import {releaseOrders} from "../../api/release-orders";

const pendingSchema=z.object({scope:z.string(),path:z.string().regex(/^\/api\/v1\/release-orders(?:\/[a-f0-9]{32}(?:\/(?:cancel|submit|approve|reject|copy|execute|rollback|complete|quick-rollback|reprepare))?)?$/),method:z.enum(["POST","PUT"]),body:z.string(),key:z.string(),label:z.string(),rejection:z.enum(["release_version_conflict","record_version_conflict","release_state_invalid","release_target_conflict","release_frozen_changed","rollback_conflict"]).optional()});
export type PendingReleaseRequest=z.infer<typeof pendingSchema>;
export const releaseJournalChanged="rcc:release-journal-changed";
const storageKey=(accountID:string)=>`rcc:release-requests:${accountID}`;
export function pendingReleaseRequests(accountID:string):PendingReleaseRequest[]{
 const raw=sessionStorage.getItem(storageKey(accountID));if(!raw)return [];
 return z.array(pendingSchema).parse(JSON.parse(raw));
}
export function rememberReleaseRequest(accountID:string,value:PendingReleaseRequest){
 const previous=pendingReleaseRequests(accountID).filter(item=>item.scope!==value.scope);
 sessionStorage.setItem(storageKey(accountID),JSON.stringify([...previous,value]));window.dispatchEvent(new Event(releaseJournalChanged));
}
export function forgetReleaseRequest(accountID:string,key:string){
 const remaining=pendingReleaseRequests(accountID).filter(item=>item.key!==key);
 if(remaining.length)sessionStorage.setItem(storageKey(accountID),JSON.stringify(remaining));else sessionStorage.removeItem(storageKey(accountID));window.dispatchEvent(new Event(releaseJournalChanged));
}
export async function sendReleaseRequest(accountID:string,value:PendingReleaseRequest){
 if(businessSession().credentials?.accountID!==accountID)throw new ApiError("stale_session","请使用原申请账号恢复此请求。",0);
 return releaseOrders.write(value.path,value.method,value.body,value.key);
}
export function uncertainReleaseError(error:unknown){return isUncertainWriteError(error)||(error instanceof ApiError&&(error.status>=500||error.status===401||error.status===403))}

// All actions targeting the same order share an exclusion boundary. The durable
// journal survives reload; this in-memory set also excludes simultaneous replay.
const sending=new Set<string>();
export const releaseRequestOrder=(request:Pick<PendingReleaseRequest,"path">)=>request.path.match(/^\/api\/v1\/release-orders\/([a-f0-9]{32})(?:\/|$)/)?.[1];
export function conflictingReleaseRequest(accountID:string,path:string,scope:string){
 const id=releaseRequestOrder({path});
 return id&&pendingReleaseRequests(accountID).find(item=>!item.rejection&&item.scope!==scope&&releaseRequestOrder(item)===id);
}
export function releaseRequestSending(accountID:string,key:string){return sending.has(`${accountID}:${key}`)}
export function startReleaseRequest(accountID:string,key:string){
 if(releaseRequestSending(accountID,key))return false;
 sending.add(`${accountID}:${key}`);window.dispatchEvent(new Event(releaseJournalChanged));return true;
}
export function finishReleaseRequest(accountID:string,key:string){sending.delete(`${accountID}:${key}`);window.dispatchEvent(new Event(releaseJournalChanged))}
