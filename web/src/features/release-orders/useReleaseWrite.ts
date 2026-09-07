import {useQueryClient} from "@tanstack/react-query";
import {useRef,useState} from "react";
import {useWorkspaceIdentity} from "../accounts/ProtectedWorkspace";
import {forgetReleaseRequest,pendingReleaseRequests,rememberReleaseRequest,sendReleaseRequest,uncertainReleaseError,type PendingReleaseRequest} from "./release-journal";
import {ApiError} from "../../api/client";
import type {ReleaseOrder} from "../../api/release-orders";

export function useReleaseWrite(scope:string){
 const accountID=useWorkspaceIdentity()!.account.id;
 const client=useQueryClient();
 const [pending,setPending]=useState(false),[error,setError]=useState<unknown>(),[unresolved,setUnresolved]=useState(()=>pendingReleaseRequests(accountID).some(item=>item.scope===scope&&!item.rejection));
 const busy=useRef(false),confirmedRebuild=useRef<string|undefined>(undefined);
 const send=async(input:Omit<PendingReleaseRequest,"scope"|"key">):Promise<ReleaseOrder|undefined>=>{
  if(busy.current)return;busy.current=true;setPending(true);setError(undefined);
  const stored=pendingReleaseRequests(accountID).find(item=>item.scope===scope);
  if(stored?.rejection&&confirmedRebuild.current!==stored.key){
   setError(new ApiError("release_rebuild_required","仍有待重建的原申请，请先查看最新状态与配置并明确确认重建。",409));
   busy.current=false;setPending(false);return;
  }
  confirmedRebuild.current=undefined;
  const previous=stored?.rejection?undefined:stored;
  const intent=previous??{...input,scope,key:crypto.randomUUID()};
  try{
   // Persist before dispatch, so a refresh during the request is also recoverable.
   rememberReleaseRequest(accountID,intent);
   const order=await sendReleaseRequest(accountID,intent);
   forgetReleaseRequest(accountID,intent.key);setUnresolved(false);
   // A replay acknowledges the original write; mounted details must read current state.
   void client.invalidateQueries({queryKey:["release-orders"]});
   void client.invalidateQueries({queryKey:["release-order",order.id]});
   return order;
  }catch(cause){
   setError(cause);
   // These write conflicts are returned only after original-key deduplication.
   // They prove no original success exists; authentication/read failures do not.
   const rejected=cause instanceof ApiError&&["release_version_conflict","record_version_conflict","release_state_invalid","release_target_conflict"].includes(cause.code);
   const keep=Boolean(previous)&&!rejected||uncertainReleaseError(cause);
   setUnresolved(keep);
   if(rejected)rememberReleaseRequest(accountID,{...intent,rejection:cause.code as PendingReleaseRequest["rejection"]});
   else if(stored?.rejection&&!keep)rememberReleaseRequest(accountID,{...intent,rejection:stored.rejection});
   else if(!keep)forgetReleaseRequest(accountID,intent.key);
  }finally{busy.current=false;setPending(false)}
 };
 return {send,pending,error,unresolved,clearError:()=>setError(undefined),confirmRebuild:()=>{
  confirmedRebuild.current=pendingReleaseRequests(accountID).find(item=>item.scope===scope&&item.rejection)?.key;
 }};
}
