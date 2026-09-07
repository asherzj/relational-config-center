import {z} from "zod";
import {ApiError,isUncertainWriteError} from "../../api/client";
import {businessSession} from "../../api/business-session";
import {releaseOrders} from "../../api/release-orders";

const pendingSchema=z.object({scope:z.string(),path:z.string().regex(/^\/api\/v1\/release-orders(?:\/[a-f0-9]{32}(?:\/cancel)?)?$/),method:z.enum(["POST","PUT"]),body:z.string(),key:z.string(),label:z.string(),rejection:z.enum(["release_version_conflict","record_version_conflict","release_state_invalid"]).optional()});
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
export function uncertainReleaseError(error:unknown){return isUncertainWriteError(error)||(error instanceof ApiError&&error.status>=500)}
