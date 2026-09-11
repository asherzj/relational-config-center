import {ReleasePerson} from "./ReleasePerson";
import {ReleaseTime} from "./ReleaseTime";
import {Check,Minus} from "lucide-react";
import type {ReleaseHeader} from "../../api/release-orders";

// Temporary rollback-only presentation until #106. Forward releases display
// persisted per-table instances; this must never serve missing configuration.
export function ReleaseProgress({order,people={}}:{order:ReleaseHeader;people?:Record<string,string>}){
 const cancelled=order.state==="CANCELLED",rejected=order.state==="REJECTED",rolledBack=order.state==="ROLLED_BACK";
 const terminated=cancelled||rejected||rolledBack;
 const rank={DRAFT:0,PENDING_APPROVAL:1,APPROVED:2,SUCCEEDED:3,COMPLETED:4,REJECTED:1,CANCELLED:0,ROLLED_BACK:3}[order.state];
 const approved=order.approvals.length>0&&order.approvals.every(table=>table.state==="APPROVED");
 const submitted=order.history.some(event=>event.action==="SUBMIT")||rank>=1&&!cancelled;
 const latest=(...actions:string[])=>[...order.history].reverse().find(event=>actions.includes(event.action));
 const events=[latest("SUBMIT"),approved||rejected?latest("APPROVE","REJECT"):undefined,latest("EXECUTE"),rolledBack?latest("ROLLED_BACK","QUICK_ROLLBACK"):cancelled?latest("CANCEL","REPREPARE"):rejected?latest("REJECT"):order.state==="COMPLETED"?latest("COMPLETE"):undefined];
 const labels=["准备","审批","发布","完结"];
 const descriptions=[submitted?"已准备":cancelled?"已取消":"准备中",rejected?"已拒绝":approved?"已批准":cancelled?"已取消":rank===1?"待审批":"待提交",order.executions.some(execution=>execution.kind==="PUBLICATION")||rank>=3?"数据库已发布":terminated?"未发布":rank===2?"待执行":"待审批后发布",rolledBack?"已回滚":cancelled||rejected?"已终止":rank===4?"已完结":rank===3?"待人工完结":"待发布后完结"];
 const completed=[submitted,approved,Boolean(order.executions.some(execution=>execution.kind==="PUBLICATION"))||rank>=3,order.state==="COMPLETED"];
 return <ol aria-label="发布阶段" className="release-progress release-panel">{labels.map((label,index)=><li key={label} aria-current={!terminated&&rank===index?"step":undefined} className={completed[index]?"is-complete":!terminated&&rank===index?"is-current":""}><div className="release-step-heading"><span className="release-step-icon" aria-hidden="true">{completed[index]?<Check size={14}/>:terminated?<Minus size={14}/>:index+1}</span><strong>{label}</strong></div><div className="release-step-detail"><p>{descriptions[index]}</p>{events[index]&&<div className="release-step-meta"><ReleasePerson id={events[index]!.actor_id} name={people[events[index]!.actor_id]}/><p><ReleaseTime value={events[index]!.at}/></p></div>}</div></li>)}</ol>;
}
