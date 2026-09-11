import type { ApprovalContext, ReleaseHeader } from "../../api/release-orders";
import { ReleasePerson } from "./ReleasePerson";
import { ReleaseTime } from "./ReleaseTime";

const modeLabels = { ROLE: "角色成员审批", ADMIN: "默认 ADMIN 审批", UNAVAILABLE: "暂无独立审批人", COMPLETED: "已完成审批" };
export function ReleaseApprovals({ order, people = {} }: { order: Pick<ReleaseHeader, "state" | "approvals" | "approval_context">; people?: Record<string, string> }) {
  const draft = order.state === "DRAFT";
  const passed = order.approvals.filter(approval => approval.state === "APPROVED").length;
  return <section className="release-panel min-w-0" aria-label={draft ? "提交审批安排" : "逐表审批进度"}>
    <h2 className="text-lg font-semibold">{draft ? "提交审批安排" : `已通过 ${passed} / ${order.approvals.length} 表`}</h2>
    <p className="mt-2 mb-4 text-muted-foreground">{draft ? "提交时固定各表角色；角色成员资格按处理时的当前状态判断。" : "每表任一合格成员通过即可，所有表通过后才能整单发布。已完成决定保留。"}</p>
    {order.approvals.length === 0 ? <p>{draft ? "添加变更明细后显示各表的审批安排。" : "此单未提交审批，没有冻结的审批安排。"}</p> : <ul className="grid divide-y">{order.approvals.map(approval => {
      const context = order.approval_context.tables.find(table => table.table_name === approval.table_name), decision = approval.decision;
      return <li key={approval.table_name} className="min-w-0 py-4 first:pt-0 last:pb-0 break-all">
        <div className="flex flex-wrap justify-between gap-2"><h3 className="font-semibold font-mono">{approval.table_name}</h3><span className={approval.state === "APPROVED" ? "text-success" : approval.state === "REJECTED" ? "text-destructive" : "text-muted-foreground"}>{draft ? "待提交" : approval.state === "APPROVED" ? "已通过" : approval.state === "REJECTED" ? "已拒绝" : "待审批"}</span></div>
        <p className="mt-2">{draft ? "当前分配" : "提交时分配"}：{approval.roles.map(role => role.name).join("、") || "无角色 · 默认 ADMIN 规则"}</p>
        {approval.roles.length > 0 && <ul className="mt-1 text-xs text-muted-foreground">{approval.roles.map(role => <li key={role.id}>{role.name} · {role.id}</li>)}</ul>}
        {context && !decision && <div className="mt-2"><p>{modeLabels[context.mode]}{context.can_approve ? " · 当前可由你审批" : ""}</p>{context.reason && <p className="text-muted-foreground">{context.reason}</p>}{context.mode === "UNAVAILABLE" && <p role="status" className="text-warning">请管理员补充已启用且不是申请人的角色成员或 ADMIN。</p>}</div>}
        {decision && <div className="mt-3 grid gap-2"><p>资格来源：{decision.source === "ADMIN" ? "默认 ADMIN" : decision.roles.map(role => role.name).join("、")}</p>{decision.roles.length > 0 && <p className="text-xs text-muted-foreground">{decision.roles.map(role => `${role.name} · ${role.id}`).join("；")}</p>}<ReleasePerson id={decision.actor_id} name={people[decision.actor_id]} /><ReleaseTime value={decision.at} /><p className="whitespace-pre-wrap">{decision.reason}</p></div>}
      </li>;
    })}</ul>}
  </section>;
}
export function ApprovalScope({ context, rejection = false }: { context: Pick<ApprovalContext, "approvable_tables">; rejection?: boolean }) {
  return <section aria-label="本次审批范围" className="my-4 rounded-lg border p-4 min-w-0"><h2 className="font-semibold">本次{rejection ? "拒绝" : "批准"}范围 · {context.approvable_tables.length} 表</h2><ul className="mt-2 grid gap-1 break-all font-mono">{context.approvable_tables.map(table => <li key={table}>{table}</li>)}</ul><p className="mt-2 text-muted-foreground">{rejection ? "本次拒绝依据以上待审批表，生效后整单终止。" : "一次批准覆盖以上全部有权且待审批的表。其他表仍须由相应负责人审批。"}</p></section>;
}
