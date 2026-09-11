import type {ReleaseHeader} from "../../api/release-orders";
import {ReleasePerson} from "./ReleasePerson";
import {ReleaseTime} from "./ReleaseTime";

const phases={DRAFT:"准备中",PENDING_APPROVAL:"待审批",APPROVED:"待执行发布",SUCCEEDED:"数据库已发布，待人工完结",COMPLETED:"已完结",REJECTED:"已拒绝，整单终止",CANCELLED:"已取消",ROLLED_BACK:"已回滚"};
const nodeStates={PENDING:"待开始",ACTIVE:"进行中",COMPLETED:"已完成",REJECTED:"已拒绝",STOPPED:"已终止"};
const nodeTypes={APPROVAL:"审批",PUBLICATION:"发布",COMPLETION:"完结"};

export function ReleasePhase({order,people={}}:{order:ReleaseHeader;people?:Record<string,string>}){
 const submitted=order.history.find(event=>event.action==="SUBMIT");
 return <section className="release-panel min-w-0" aria-label="发布阶段">
  <p className="text-xs text-muted-foreground">常规发布 · 整单阶段</p>
  <h2 className="mt-2 text-lg font-semibold">{phases[order.state]}</h2>
  {submitted&&<div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs"><span>准备完成于提交</span><ReleasePerson id={submitted.actor_id} name={people[submitted.actor_id]}/><ReleaseTime value={submitted.at}/></div>}
 </section>;
}

export function ReleaseFlows({order,people={}}:{order:ReleaseHeader;people?:Record<string,string>}){
 return <section className="release-panel min-w-0" aria-label="逐表发布流程">
  <h2 className="text-lg font-semibold">逐表发布流程</h2>
  <p className="mt-2 text-muted-foreground">各表使用保存时的流程。全部表审批通过后，整单统一发布。</p>
  {order.table_flows.length===0&&order.missing_flow_tables.length===0&&<p className="mt-4">添加明细并保存草稿后显示各表流程。</p>}
  <div className="divide-y">{order.table_flows.map(flow=><section key={flow.instance_id} aria-label={`${flow.table_name} 发布流程`} className="min-w-0 py-5 last:pb-0">
   <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-2"><h3 className="font-semibold font-mono break-all">{flow.table_name}</h3><p className="break-all">{flow.template_name}</p></div>
   <ol aria-label={`${flow.table_name} 流程节点`} className="mt-4 grid gap-5 min-[760px]:grid-cols-3">{flow.node_list.map((node,index)=><li key={node.code} aria-current={node.state==="ACTIVE"?"step":undefined} className="flex min-w-0 items-start gap-3">
    <span aria-hidden="true" className={`release-step-icon ${node.state==="COMPLETED"?"bg-success-soft text-success":node.state==="ACTIVE"?"bg-primary text-primary-foreground":""}`}>{index+1}</span>
    <div className="min-w-0 break-all"><h4 className="font-medium">{node.name}</h4><p className="mt-1 text-xs text-muted-foreground">{nodeTypes[node.type]} · {node.required_role==="TABLE_APPROVER"?"按表审批资格":"发布权限"}</p><p className={`mt-2 ${node.state==="COMPLETED"?"text-success":node.state==="REJECTED"?"text-destructive":"text-muted-foreground"}`}>{nodeStates[node.state]}</p>{node.actor_id&&<div className="mt-2 text-xs"><ReleasePerson id={node.actor_id} name={people[node.actor_id]}/></div>}{node.at&&<div className="mt-1 text-xs"><ReleaseTime value={node.at}/></div>}</div>
   </li>)}</ol>
   <details className="mt-4 min-w-0 text-xs text-muted-foreground"><summary className="w-fit cursor-pointer rounded-sm focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring">查看实例来源与版本</summary><dl className="mt-3 grid gap-2 break-all"><div><dt>流程实例</dt><dd className="font-mono">{flow.instance_id}</dd></div><div><dt>来源模板</dt><dd className="font-mono">{flow.template_code} · 版本 {flow.template_version}</dd></div><div><dt>表关联版本</dt><dd>{flow.association_version}</dd></div><div><dt>实例保存时间</dt><dd><ReleaseTime value={flow.instantiated_at}/></dd></div></dl></details>
  </section>)}</div>
 </section>;
}
