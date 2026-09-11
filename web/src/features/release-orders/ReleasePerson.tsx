import {useId,useState} from "react";
import {ChevronDown} from "lucide-react";
import {Button} from "../../components/ui/Button";

export function ReleasePerson({id,name}:{id:string;name?:string}){
 const [expanded,setExpanded]=useState(false);
 const [copyState,setCopyState]=useState<"copied"|"failed">();
 const detailsID=useId();
 const label=name?.trim()||`账号 ${id.slice(0,8)}`;
 const initial=Array.from(name?.trim()||"")[0]??"?";
 return <span className="inline-flex w-max min-w-0 max-w-full flex-col items-start align-middle">
  <Button variant="ghost" className="h-auto max-w-full justify-start gap-1.5 whitespace-normal px-1 py-0.5 text-left font-normal" aria-label={`${expanded?"收起":"查看"}${label}的账号信息`} aria-expanded={expanded} aria-controls={detailsID} onClick={()=>setExpanded(!expanded)}>
   <span aria-hidden className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">{initial}</span>
   <strong className="min-w-0 break-all font-medium text-foreground">{label}</strong>
   <ChevronDown aria-hidden size={12} className={`shrink-0 text-muted-foreground ${expanded?"rotate-180":""}`}/>
  </Button>
  {expanded&&<span id={detailsID} className="mt-1 flex min-w-0 max-w-full flex-wrap items-center gap-1.5 rounded-md border bg-muted/40 px-2 py-1" role="group" aria-label={`${label}的账号信息`}>
   <code className="min-w-0 break-all text-xs text-muted-foreground">{id}</code>
   <Button variant="ghost" className="h-7 px-2 text-xs" onClick={async()=>{try{await navigator.clipboard.writeText(id);setCopyState("copied")}catch{setCopyState("failed")}}}>复制账号 ID</Button>
   {copyState==="copied"&&<span role="status" className="text-xs">已复制账号 ID</span>}
   {copyState==="failed"&&<span role="alert" className="text-xs">复制失败，请选择完整账号 ID 手动复制。</span>}
  </span>}
 </span>;
}
