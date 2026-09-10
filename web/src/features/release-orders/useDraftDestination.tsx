import {useEffect,useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {ApiError,shouldRetryQuery} from "../../api/client";
import {decodeReleaseRequest,draftItemSchema,defaultReleaseTitle,releaseOrders,releaseRequests,releaseTitleError,type ReleaseRequestEnvelope,type DraftContentInput} from "../../api/release-orders";
import {useWorkspaceIdentity} from "../accounts/ProtectedWorkspace";
import {Button} from "../../components/ui/Button";
import {NativeSelect} from "../../components/shadcn/native-select";
import {Input} from "../../components/shadcn/input";
import {Label} from "../../components/shadcn/label";
import {ErrorState} from "../../components/ui/Feedback";

export function useDraftDestination(table:string,initialID=""){
 const accountID=useWorkspaceIdentity()!.account.id;
 const [selected,setSelected]=useState(initialID),[open,setOpen]=useState(Boolean(initialID)),[after,setAfter]=useState("");
 const [title,setTitle]=useState(()=>defaultReleaseTitle(table));
 const titleError=releaseTitleError(title);
 const [error,setError]=useState<unknown>();
 useEffect(()=>setTitle(defaultReleaseTitle(table)),[table]);
 const list=useQuery({queryKey:["release-orders","draft-destination",table,accountID,after],queryFn:()=>releaseOrders.list({applicant_id:accountID,state:"DRAFT",after}),enabled:open&&Boolean(table),retry:shouldRetryQuery});
 const prepare=async(input:DraftContentInput)=>{
  setError(undefined);
  try{
   if(!selected){
    if(titleError)throw new ApiError("release_title_invalid",titleError,422);
    return releaseRequests.create({...input,title});
   }
   const order=await releaseOrders.get(selected);
   if(order.applicant_id!==accountID||!order.allowed_actions.includes("edit")||order.state!=="DRAFT")throw new ApiError("release_destination_invalid","所选草稿已不可编辑，或不属于本人。请重新选择。",422);
   if(order.items.length+input.items.length>1000)throw new ApiError("release_item_limit","合并后的草稿不能超过 1,000 项，请调整明细。",422);
   return releaseRequests.edit(order.id,{title:order.title,table_name:order.table_name,expected_version:order.version,changes:{upserts:input.items.map(item=>({...item,table_name:item.table_name??input.table_name}))}});
  }catch(cause){setError(cause);return undefined}
 };
 const picker=(disabled:boolean)=><section className="draft-destination" aria-label="草稿去向">
  <div className={`draft-destination-fields${open&&!selected?" draft-destination-fields-open":""}`}>
   {!selected&&<div className="draft-destination-title">
    <div className="draft-destination-title-meta">
     <Label htmlFor="new-release-title">发布单标题</Label>
     <span id="new-release-title-count">{Array.from(title).length} / 100 字符</span>
    </div>
    <div className="draft-destination-title-entry">
     <Input id="new-release-title" value={title} required aria-invalid={Boolean(titleError)} aria-describedby={`new-release-title-count${titleError?" new-release-title-error":""}`} disabled={disabled} onChange={event=>setTitle(event.target.value)}/>
     {!open&&<Button disabled={disabled} onClick={()=>setOpen(true)}>选择已有草稿</Button>}
    </div>
    {titleError&&<small id="new-release-title-error" className="field-error">{titleError}</small>}
   </div>}
   {open&&<div className="draft-destination-picker">
    <Label htmlFor="release-draft-destination">保存到草稿</Label>
    <NativeSelect id="release-draft-destination" aria-label="保存到草稿" disabled={disabled} value={selected} onChange={event=>setSelected(event.target.value)}>
     <option value="">新建草稿</option>
     {selected&&!list.data?.orders.some(order=>order.id===selected)&&<option value={selected}>{selected}</option>}
     {list.data?.orders.filter(order=>!order.rollback_of_id&&order.allowed_actions.includes("edit")).map(order=><option key={order.id} value={order.id}>{order.title} · {order.id} · 版本 {order.version}</option>)}
    </NativeSelect>
    {list.isPending&&<p className="draft-destination-hint">正在读取本人草稿…</p>}
    {list.isError&&<ErrorState error={list.error}/>}
    <div className="draft-destination-pagination">
     <Button variant="ghost" disabled={disabled||!after} onClick={()=>setAfter("")}>草稿首页</Button>
     <Button variant="ghost" disabled={disabled||!list.data?.next_cursor} onClick={()=>setAfter(list.data!.next_cursor)}>更多草稿</Button>
    </div>
   </div>}
  </div>
  <p className="draft-destination-hint">同一数据源内多表合计最多 1,000 项。整单按明细顺序审批与发布。</p>
  {Boolean(error)&&<ErrorState error={error}/>}
 </section>;
 // Replaying a request from another window must not acknowledge this
 // window's different unsaved fields, title, destination or selected rows.
 const matchesInput=(request:ReleaseRequestEnvelope,input:DraftContentInput)=>{
  try{
   const intent=decodeReleaseRequest(request);
   if(!selected&&intent.action==="create")return intent.input.title===title&&intent.input.table_name===input.table_name&&JSON.stringify(intent.input.items)===JSON.stringify(input.items.map(item=>draftItemSchema.parse(item)));
   if(selected&&intent.action==="edit-details"&&intent.id===selected){
    const changes=intent.input.changes;
    return !changes.delete_detail_ids&&!changes.detail_order&&JSON.stringify(changes.upserts)===JSON.stringify(input.items.map(item=>draftItemSchema.parse({...item,table_name:item.table_name??input.table_name})));
   }
  }catch{/* An unreadable retained request cannot acknowledge current input. */}
  return false;
 };
 return {prepare,picker,matchesInput,valid:Boolean(selected)||!titleError};
}
