import {useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {ApiError,shouldRetryQuery} from "../../api/client";
import {draftFromOrder,releaseOrders,releaseRequests,type DraftInput} from "../../api/release-orders";
import {useWorkspaceIdentity} from "../accounts/ProtectedWorkspace";
import {Button} from "../../components/ui/Button";
import {NativeSelect} from "../../components/shadcn/native-select";
import {ErrorState} from "../../components/ui/Feedback";

export function useDraftDestination(table:string,initialID=""){
 const accountID=useWorkspaceIdentity()!.account.id;
 const [selected,setSelected]=useState(initialID),[open,setOpen]=useState(Boolean(initialID)),[after,setAfter]=useState("");
 const [error,setError]=useState<unknown>();
 const list=useQuery({queryKey:["release-orders","draft-destination",table,accountID,after],queryFn:()=>releaseOrders.list({table_name:table,applicant_id:accountID,state:"DRAFT",after}),enabled:open&&Boolean(table),retry:shouldRetryQuery});
 const prepare=async(input:DraftInput)=>{
  setError(undefined);
  try{
   if(!selected)return releaseRequests.create(input);
   const order=await releaseOrders.get(selected);
   if(order.table_name!==table||order.applicant_id!==accountID||!order.allowed_actions.includes("edit")||order.state!=="DRAFT")throw new ApiError("release_destination_invalid","所选草稿已不可编辑，或不属于本人当前表。请重新选择。",422);
   const body=draftFromOrder(order);body.items.push(...input.items);
   if(body.items.length>1000)throw new ApiError("release_item_limit","合并后的草稿不能超过 1,000 项，请调整明细。",422);
   return releaseRequests.edit(order.id,body);
  }catch(cause){setError(cause);return undefined}
 };
 const picker=(disabled:boolean)=><section className="grid gap-2 my-3" aria-label="草稿去向"><p>一单同表，最多 1,000 项。整单提交审批与发布。</p>{!open?<Button disabled={disabled} onClick={()=>setOpen(true)}>选择已有草稿</Button>:<><label>保存到草稿<NativeSelect aria-label="保存到草稿" disabled={disabled} value={selected} onChange={event=>setSelected(event.target.value)}><option value="">新建草稿</option>{selected&&!list.data?.orders.some(order=>order.id===selected)&&<option value={selected}>{selected}</option>}{list.data?.orders.filter(order=>!order.rollback_of_id&&order.allowed_actions.includes("edit")).map(order=><option key={order.id} value={order.id}>{order.id} · 版本 {order.version}</option>)}</NativeSelect></label>{list.isPending&&<p>正在读取本人同表草稿…</p>}{list.isError&&<ErrorState error={list.error}/>}<div className="flex gap-2"><Button disabled={disabled||!after} onClick={()=>setAfter("")}>草稿首页</Button><Button disabled={disabled||!list.data?.next_cursor} onClick={()=>setAfter(list.data!.next_cursor)}>更多草稿</Button></div></>}{Boolean(error)&&<ErrorState error={error}/>}</section>;
 return {prepare,picker};
}
