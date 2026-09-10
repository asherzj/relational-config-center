import {useReleaseJournal} from "./useReleaseJournal";
import {useQueryClient} from "@tanstack/react-query";
import {useRef,useState} from "react";
import {useWorkspaceIdentity} from "../accounts/ProtectedWorkspace";
import {hydrateReleaseRequests,conflictingReleaseRequest,finishReleaseRequest,releaseRequestSending,startReleaseRequest,forgetReleaseRequest,pendingReleaseRequests,rememberReleaseRequest,sendReleaseRequest,uncertainReleaseError,type PendingReleaseRequest} from "./release-journal";
import {ApiError} from "../../api/client";
import type {ReleaseOrder} from "../../api/release-orders";

export function useReleaseWrite(scope:string){
 const accountID=useWorkspaceIdentity()!.account.id;
 const client=useQueryClient(),journal=useReleaseJournal();
 const [pending,setPending]=useState(false),[error,setError]=useState<unknown>();
 const storedRequest=pendingReleaseRequests(accountID).find(item=>item.scope===scope);
 const sharedPending=Boolean(storedRequest&&releaseRequestSending(accountID,storedRequest.key));
 const unresolved=Boolean(storedRequest&&!storedRequest.rejection&&!pending&&!sharedPending);
 const scopeID=scope.split(":").at(-1);
 const blocked=Boolean(scopeID&&conflictingReleaseRequest(accountID,`/api/v1/release-orders/${scopeID}`,scope));
 const busy=useRef(false),confirmedRebuild=useRef<string|undefined>(undefined);
 const send=async(input:Omit<PendingReleaseRequest,"scope"|"key">):Promise<ReleaseOrder|undefined>=>{
  if(busy.current)return;
  if(conflictingReleaseRequest(accountID,input.path,scope)){setError(new ApiError("release_request_pending","此发布单已有请求正在处理，请稍后再执行。",409));return;}
  busy.current=true;setPending(true);setError(undefined);
  try{await hydrateReleaseRequests(accountID)}catch{
   setError(new ApiError("release_journal_unavailable","浏览器无法读取原发布请求，尚未发送。请恢复浏览器存储后重试。",0));busy.current=false;setPending(false);return;
  }
  const stored=pendingReleaseRequests(accountID).find(item=>item.scope===scope);
  const rebuild=Boolean(stored?.rejection&&confirmedRebuild.current===stored.key);
  confirmedRebuild.current=undefined;
  const previous=rebuild?undefined:stored;
  const intent=previous??{...input,scope,key:crypto.randomUUID()};
  if(!startReleaseRequest(accountID,intent.key)){busy.current=false;setPending(false);return;}
  let recorded=false;
  try{
   // Persist before dispatch, so a refresh during the request is also recoverable.
   await rememberReleaseRequest(accountID,intent,rebuild?stored?.key:undefined);recorded=true;
   const order=await sendReleaseRequest(accountID,intent);
   // A replay acknowledges the original write; mounted details must read current state.
   void client.invalidateQueries({queryKey:["release-orders"]});
   if(order.publication)void client.invalidateQueries({queryKey:["managed-data"]});
   void client.invalidateQueries({queryKey:["release-order",order.id]});
   void client.invalidateQueries({queryKey:["release-order-people",order.id]});
   if(order.rollback_of_id){
    void client.invalidateQueries({queryKey:["release-order",order.rollback_of_id]});
    void client.invalidateQueries({queryKey:["release-order-people",order.rollback_of_id]});
   }
   if(order.copied_from_id){
    void client.invalidateQueries({queryKey:["release-order",order.copied_from_id]});
    void client.invalidateQueries({queryKey:["release-order-people",order.copied_from_id]});
   }
   // HTTP success is already a business success. Local cleanup cannot turn it
   // into an execution failure or prevent authoritative queries refreshing.
   try{await forgetReleaseRequest(accountID,intent.key)}catch{
    setError(new ApiError("release_journal_cleanup","操作已成功，浏览器暂时无法清理原请求；正在重新读取当前状态。再次点击仍会使用同一请求。",0));
   }
   // A second tab may have saved a different intent while this editor was open.
   // Acknowledging that request must not close and discard this tab's input.
   return previous&&previous.body!==input.body?undefined:order;
  }catch(cause){
   if(!recorded){setError(cause instanceof ApiError?cause:new ApiError("release_journal_unavailable","浏览器无法保存完整请求，尚未发送。当前输入和已有待恢复请求保留，请释放浏览器存储空间后重试。",0));return;}
   // These write conflicts are returned only after original-key deduplication.
   // They prove no original success exists; authentication/read failures do not.
   const rejected=cause instanceof ApiError&&["release_version_conflict","record_version_conflict","release_state_invalid","release_target_conflict","release_frozen_changed","rollback_conflict"].includes(cause.code);
   const keep=Boolean(previous)||uncertainReleaseError(cause)||(cause instanceof ApiError&&cause.executionOutcome==="not_committed");
   try{if(rejected)await rememberReleaseRequest(accountID,{...intent,rejection:cause.code as PendingReleaseRequest["rejection"]});
   else if(stored?.rejection&&!keep)await rememberReleaseRequest(accountID,{...intent,rejection:stored.rejection});
   else if(!keep)await forgetReleaseRequest(accountID,intent.key);

   }catch{/* The durable request remains available for an explicit retry. */}
   setError(cause);
   // A timeout or error cannot identify current state. Only normal reads do.
   void client.invalidateQueries({queryKey:["release-orders"]});
   if(scopeID)void client.invalidateQueries({queryKey:["release-order",scopeID]});
 }finally{finishReleaseRequest(accountID,intent.key);busy.current=false;setPending(false)}
 };
 const retry=()=>{
  const stored=pendingReleaseRequests(accountID).find(item=>item.scope===scope);
  if(!stored)return Promise.resolve(undefined);
  return send({path:stored.path,method:stored.method,body:stored.body,label:stored.label});
 };
 return {send,retry,storedRequest,pending:pending||sharedPending||journal.pending,blocked,error:error??journal.error,unresolved,clearError:()=>setError(undefined),confirmRebuild:()=>{
  confirmedRebuild.current=pendingReleaseRequests(accountID).find(item=>item.scope===scope&&item.rejection)?.key;
 }};
}
