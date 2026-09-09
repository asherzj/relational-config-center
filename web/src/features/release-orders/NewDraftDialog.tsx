import {useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {useNavigate} from "react-router-dom";
import {listTablePolicies} from "../../api/table-policies";
import {defaultReleaseTitle,releaseRequests,releaseTitleError} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {Drawer} from "../../components/ui/Drawer";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {Input} from "../../components/shadcn/input";
import {NativeSelect} from "../../components/shadcn/native-select";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";

export function NewDraftDialog({onClose}:{onClose:()=>void}) {
 const allowed=useAccountRole("EDITOR");
 const tables=useQuery({queryKey:["draft-managed-tables"],queryFn:listTablePolicies,retry:false});
 const [table,setTable]=useState(""),[title,setTitle]=useState("");
 const titleError=releaseTitleError(title);
 const write=useReleaseWrite("create"),navigate=useNavigate(),protection=useDraftProtection(Boolean(table||title),write.pending);
 return <Drawer open eyebrow="发布单" title="新建发布草稿" onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>取消</Button><Button variant="primary" disabled={!allowed||!table||Boolean(titleError)||write.pending||write.blocked} onClick={async()=>{const saved=await write.send({...releaseRequests.create({title,table_name:table,items:[]}),label:"新建发布草稿"});if(saved)protection.afterSave(()=>{onClose();navigate(`/configuration/release-orders/${saved.id}`)})}}>{write.pending?"正在创建…":"创建空草稿"}</Button></>}>
 <p>先保存标题，再逐步添加明细。空草稿不占用目标；保存明细时开始占用。</p><div className="grid gap-4 mt-5"><label>草稿表<NativeSelect aria-label="草稿表" value={table} disabled={!allowed||write.pending||write.unresolved} onChange={event=>{setTable(event.target.value);setTitle(defaultReleaseTitle(event.target.value))}}><option value="">请选择已启用的表</option>{tables.data?.filter(item=>item.enabled).map(item=><option key={item.tableName} value={item.tableName}>{item.tableName}</option>)}</NativeSelect></label><label htmlFor="new-draft-title">发布单标题<Input id="new-draft-title" value={title} required aria-invalid={Boolean(titleError)} aria-describedby={`new-draft-title-count${titleError?" new-draft-title-error":""}`} disabled={!allowed||write.pending||write.unresolved} onChange={event=>setTitle(event.target.value)}/></label><span id="new-draft-title-count" className="text-xs text-muted-foreground">{Array.from(title).length} / 100 字符</span>{titleError&&<small id="new-draft-title-error" className="field-error">{titleError}</small>}</div>{tables.isPending&&<LoadingState label="正在读取已启用的表…"/>}{tables.isError&&<ErrorState error={tables.error} onRetry={()=>void tables.refetch()}/>}{tables.isSuccess&&!tables.data.some(item=>item.enabled)&&<p>当前没有已启用的表，请联系管理员配置表规则。</p>} {Boolean(write.error)&&<ErrorState error={write.error}/>}</Drawer>;
}
