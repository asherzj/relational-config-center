import {useEffect,useRef,useState} from "react";
import {useQuery,useQueryClient} from "@tanstack/react-query";
import {useNavigate} from "react-router-dom";
import {listTableReleaseTemplates,putTableReleaseTemplate,type AssociationWrite,type TableReleaseTemplate} from "../../api/table-release-templates";
import {listReleaseTemplates} from "../../api/release-templates";
import {ApiError,isUncertainWriteError} from "../../api/client";
import {Button} from "../../components/ui/Button";
import {useToast} from "../../components/ui/Toast";
import {Drawer} from "../../components/ui/Drawer";
import {ErrorState,LoadingState} from "../../components/ui/Feedback";
import {useDraftProtection,useLeaveProtection} from "../../components/ui/LeaveProtection";
import {Label} from "../../components/shadcn/label";
import {NativeSelect} from "../../components/shadcn/native-select";
import {Checkbox} from "../../components/shadcn/checkbox";
import {useAccountRole} from "../accounts/roles";
import type {ReleaseTemplate} from "../release-templates/model";

export function TableReleaseTemplatesDrawer({tableName}:{tableName:string}){
 const navigate=useNavigate();const canManage=useAccountRole("ADMIN");const protection=useLeaveProtection();
 const associations=useQuery({queryKey:["table-release-templates",tableName],queryFn:()=>listTableReleaseTemplates(tableName),enabled:canManage,retry:false});
 const templates=useQuery({queryKey:["release-templates","list"],queryFn:listReleaseTemplates,enabled:canManage,retry:false});
 const close=()=>protection.requestLeave(()=>navigate(`/platform/table-policies/${encodeURIComponent(tableName)}`));
 return <Drawer open title="表发布流程设置" eyebrow={tableName} onClose={close} footer={<Button onClick={close}>关闭</Button>}>
  {!canManage&&!associations.data?<p>仅管理员可以管理表发布流程。</p>:associations.isPending||templates.isPending?<LoadingState label="正在读取表关联与模板…"/>:(associations.isError&&!associations.data)||(templates.isError&&!templates.data)?<ErrorState error={associations.error??templates.error} onRetry={()=>{void associations.refetch();void templates.refetch();}}/>:<div className="policy-form">{(associations.isError||templates.isError)&&<ErrorState error={associations.error??templates.error}/>}<p className="text-muted-foreground">每种方式选择一个模板。应急关联始终启用，替换成功后整体切换；此处配置用于后续流程实例。</p>{(["STANDARD","EMERGENCY"] as const).map(type=><AssociationForm key={`${tableName}:${type}`} table={tableName} type={type} initial={associations.data.find(row=>row.type===type)} currentReady={associations.isSuccess&&!associations.isFetching} templates={templates.data} canManage={canManage}/>)}</div>}
 </Drawer>;
}

function AssociationForm({table,type,initial,currentReady,templates,canManage}:{table:string;type:TableReleaseTemplate["type"];initial?:TableReleaseTemplate;currentReady:boolean;templates:ReleaseTemplate[];canManage:boolean}){
 const {showToast}=useToast();const client=useQueryClient();
 const [saved,setSaved]=useState(initial);const [code,setCode]=useState(initial?.template_code??"");const [enabled,setEnabled]=useState(initial?.enabled??true);
 const [pending,setPending]=useState(false);const [error,setError]=useState<unknown>(null);const [message,setMessage]=useState("");const original=useRef<AssociationWrite|null>(null);const inFlight=useRef(false);
 const uncertain=original.current!==null;const label=type==="STANDARD"?"常规":"应急";const selected=templates.find(item=>item.code===code);
 const dirty=code!==(saved?.template_code??"")||enabled!==(saved?.enabled??true)||uncertain;
 const protection=useDraftProtection(dirty,pending,inFlight);
 useEffect(()=>{if(!dirty&&!pending&&currentReady){setSaved(initial);setCode(initial?.template_code??"");setEnabled(initial?.enabled??true);}},[initial,currentReady,dirty,pending]);
 async function save(){
  if(!canManage||inFlight.current||(!original.current&&!code))return;
  const write=original.current??{table,type,templateCode:code,enabled,version:saved?.version??"0",key:`table-template-${crypto.randomUUID()}`};
  inFlight.current=true;setPending(true);setMessage("");
  try{const result=await putTableReleaseTemplate(write);setSaved(result);setCode(result.template_code);setEnabled(result.enabled);original.current=null;setError(null);showToast(`${label}关联已保存`);await client.invalidateQueries({queryKey:["table-release-templates",table]});}
  catch(err){setError(err);if(err instanceof ApiError&&err.code==="table_release_template_conflict")original.current=null;else if(isUncertainWriteError(err)||original.current)original.current=write;}
  finally{inFlight.current=false;setPending(false);protection.submissionSettled();}
 }
 async function reviewLatest(){
  if(inFlight.current||uncertain)return;inFlight.current=true;setPending(true);
  try{const rows=await listTableReleaseTemplates(table);setSaved(rows.find(row=>row.type===type));client.setQueryData(["table-release-templates",table],rows);setError(null);setMessage("已读取最新关联，当前输入保留；核对后可重新保存。");}
  catch(err){setError(err);}finally{inFlight.current=false;setPending(false);protection.submissionSettled();}
 }
 return <section className="grid min-w-0 gap-3 border-t pt-5" aria-label={`${label}发布关联`}>
  <h3 className="font-semibold">{label}发布</h3>
  <p className="break-all text-muted-foreground">当前：{initial?`${initial.template_name} · ${initial.template_code} · ${initial.enabled&&initial.template_enabled?"有效":"已停用"} · v${initial.version}`:"尚未选择模板"}</p>
  <fieldset className="grid min-w-0 gap-3" disabled={!canManage||pending||uncertain}>
   <Label className="field"><span>{label}模板</span><NativeSelect aria-label={`${label}模板`} value={code} onChange={event=>setCode(event.target.value)}><option value="">请选择{label}模板</option>{templates.filter(item=>item.type===type).map(item=><option key={item.code} value={item.code} disabled={!item.enabled&&item.code!==saved?.template_code}>{item.name} · {item.code}{item.enabled?"":" · 已停用"}</option>)}</NativeSelect></Label>
   {type==="STANDARD"?<Label className="flex items-center gap-2"><Checkbox checked={enabled} onCheckedChange={value=>setEnabled(value===true)}/>启用常规关联</Label>:<p>应急关联始终启用，不提供停用、解绑或删除。</p>}
  </fieldset>
  {selected&&<ol className="grid gap-2" aria-label={`${label}模板节点`}>{selected.nodes.map((node,index)=><li className="break-words" key={node.code}>{index+1}. {node.name}</li>)}</ol>}
  {initial&&<p className="break-all text-muted-foreground">最近修改：{initial.modifier} · {new Date(initial.updated_at).toLocaleString("zh-CN")}</p>}
  {error!=null&&<ErrorState error={error}/>}{uncertain&&<p role="alert">保存结果未知；输入和原请求已保留，再次保存将原样重推。</p>}{message&&<p role="status">{message}</p>}
  <div className="flex flex-wrap gap-2"><Button variant="primary" disabled={!canManage||pending||(!uncertain&&(!code||!dirty))} onClick={()=>void save()}>{pending?"正在保存…":uncertain?`重推${label}原请求`:`保存${label}关联`}</Button>{error!=null&&!uncertain&&<Button disabled={pending||!canManage} onClick={()=>void reviewLatest()}>读取最新关联并保留输入</Button>}</div>
 </section>;
}
