import {useState} from "react";
import {useNavigate} from "react-router-dom";
import {decodeReleaseRequest,releaseRequests,releaseTitleError,type ReleaseType} from "../../api/release-orders";
import {Button} from "../../components/ui/Button";
import {Drawer} from "../../components/ui/Drawer";
import {ErrorState} from "../../components/ui/Feedback";
import {Input} from "../../components/shadcn/input";
import {NativeSelect} from "../../components/shadcn/native-select";
import {useDraftProtection} from "../../components/ui/LeaveProtection";
import {useToast} from "../../components/ui/Toast";
import {useAccountRole} from "../accounts/roles";
import {useReleaseWrite} from "./useReleaseWrite";
import {PendingIntent} from "./ReleaseRequestReview";

export function NewDraftDialog({onClose}:{onClose:()=>void}) {
 const allowed=useAccountRole("EDITOR");
 const write=useReleaseWrite("create");
 const [original]=useState(write.storedRequest);
 const [initial]=useState(()=>{if(!original)return;try{return decodeReleaseRequest(original)}catch{return}});
 const [title,setTitle]=useState(initial&&"title" in initial.input?initial.input.title:"");
 const [releaseType,setReleaseType]=useState<ReleaseType>(initial&&"release_type" in initial.input&&initial.input.release_type?initial.input.release_type:"STANDARD");
 const titleError=releaseTitleError(title);
 const {showToast}=useToast();
 const navigate=useNavigate(),protection=useDraftProtection(Boolean(title)||releaseType!=="STANDARD",write.pending);
 return <Drawer open eyebrow="发布单" title={original?"保存发布草稿":"新建发布草稿"} onClose={()=>protection.requestLeave(onClose)} footer={<><Button disabled={write.pending} onClick={()=>protection.requestLeave(onClose)}>取消</Button><Button variant="primary" disabled={!allowed||(!original&&Boolean(titleError))||write.pending||write.blocked} onClick={async()=>{const saved=original?await write.retry():await write.send({...releaseRequests.create({title,items:[],...(releaseType==="EMERGENCY"?{release_type:releaseType}:{})}),label:"新建发布草稿"});if(saved){showToast(releaseType==="EMERGENCY"?"应急发布草稿已创建":"发布草稿已创建");protection.afterSave(()=>{onClose();navigate(`/configuration/release-orders/${saved.id}`)})}}}>{write.pending?"正在保存…":original?"确认并保存草稿":"创建空草稿"}</Button></>}>
 {original?<><p>原申请与请求标识已保留。保存会提交这份原申请，主单状态由正常读取更新。</p><PendingIntent item={original}/></>:<><p>先保存标题和发布方式，再逐步添加明细。空草稿不占用目标；保存明细时开始占用。</p><div className="grid gap-4 mt-5"><label htmlFor="new-draft-title">发布单标题<Input id="new-draft-title" value={title} required aria-invalid={Boolean(titleError)} aria-describedby={`new-draft-title-count${titleError?" new-draft-title-error":""}`} disabled={!allowed||write.pending||write.unresolved} onChange={event=>setTitle(event.target.value)}/></label><span id="new-draft-title-count" className="text-xs text-muted-foreground">{Array.from(title).length} / 100 字符</span>{titleError&&<small id="new-draft-title-error" className="field-error">{titleError}</small>}<label htmlFor="new-draft-release-type">发布方式<NativeSelect id="new-draft-release-type" aria-label="发布方式" value={releaseType} disabled={!allowed||write.pending||write.unresolved} onChange={event=>setReleaseType(event.target.value as ReleaseType)}><option value="STANDARD">常规发布</option><option value="EMERGENCY">应急发布</option></NativeSelect></label><p className="text-xs text-muted-foreground">常规提交后进入审批；应急提交原因后进入待发布，仍需发布人员手动执行。</p></div></>} {Boolean(write.error)&&<ErrorState error={write.error}/>}</Drawer>;
}
