import { useQuery } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { shouldRetryQuery } from "../../api/client";
import { releaseOrders, releaseTables } from "../../api/release-orders";
import { Button as PrimitiveButton } from "../../components/shadcn/button";
import { Input } from "../../components/shadcn/input";
import { NativeSelect } from "../../components/shadcn/native-select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../../components/shadcn/table";
import { Button } from "../../components/ui/Button";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { ReleaseDetail, releaseStateLabels } from "../release-orders/ReleaseOrdersPage";
import { ReleaseConflictReview } from "../release-orders/ReleaseRequestReview";
import { ReleaseTime } from "../release-orders/ReleaseTime";

const listPath = "/configuration/notifications";
const views = {
  pending: { label: "待我审批", description: "仍有你可以处理的待审批表，每张发布单显示一行。", empty: "暂无待你审批的发布单。" },
  mine: { label: "我发起的", description: "你已提交的申请及其后续进展。", empty: "暂无你已提交的申请。" },
  handled: { label: "我已处理", description: "你实际批准或拒绝过的发布单，保留后续进展。", empty: "暂无你已处理的发布单。" },
  all: { label: "全部审批", description: "本部署所有已提交的发布单及其后续状态。", empty: "暂无已提交的发布单。" },
};
type ApprovalView = keyof typeof views;
function notificationFilters(params: URLSearchParams): Record<string, string> & { view: ApprovalView } {
  const requestedView = params.get("view") ?? "pending";
  const view = Object.hasOwn(views, requestedView) ? requestedView as ApprovalView : "pending";
  const filters: Record<string, string> & { view: ApprovalView } = { view, limit: params.get("limit") || "20" };
  for (const key of ["table_name", "state", "id", "after"]) {
    const value = params.get(key);
    if (value) filters[key] = value;
  }
  return filters;
}

export function NotificationsPage() {
  const { id } = useParams();
  const [params] = useSearchParams();
  const returnPath = `${listPath}?${new URLSearchParams(notificationFilters(params))}`;
  return <main className="workspace min-w-0">
    <div className="page-heading"><div><h1>通知中心</h1></div></div>
    {id ? <><ReleaseConflictReview /><ReleaseDetail key={id} id={id} listPath={returnPath} listLabel="通知中心" /></> : <NotificationList />}
  </main>;
}

function NotificationList() {
  const [params, setParams] = useSearchParams();
  const filters = notificationFilters(params);
  const currentView = views[filters.view];
  const search = new URLSearchParams(filters).toString();
  const list = useQuery({ queryKey: ["release-orders", filters], queryFn: () => releaseOrders.list(filters), retry: shouldRetryQuery });
  const filtered = Boolean(filters.table_name || filters.state || filters.id);
  const firstPage = { ...filters };
  delete firstPage.after;
  return <>
    <nav aria-label="审批视图" className="mb-4 grid grid-cols-2 gap-2 sm:flex sm:flex-wrap">
      {Object.entries(views).map(([view, { label }]) => <PrimitiveButton key={view} asChild variant={filters.view === view ? "default" : "outline"}>
        <Link to={`${listPath}?${new URLSearchParams({ ...firstPage, view })}`} aria-current={filters.view === view ? "page" : undefined}>{label}</Link>
      </PrimitiveButton>)}
    </nav>
    <p className="mb-6 text-muted-foreground" aria-live="polite">{currentView.description}</p>
    <section className="min-w-0 overflow-hidden rounded-xl border bg-card" aria-label={`${currentView.label}列表`}>
      <form key={search} className="flex flex-wrap items-end gap-3 border-b p-4" onSubmit={event => {
        event.preventDefault();
        const values = new FormData(event.currentTarget);
        const next: Record<string, string> = { view: filters.view, limit: filters.limit! };
        for (const key of ["table_name", "id", "state"]) {
          const value = String(values.get(key) ?? "").trim();
          if (value) next[key] = value;
        }
        setParams(next);
      }}>
        <label className="min-w-0 w-full sm:w-52">表名<Input name="table_name" defaultValue={filters.table_name ?? ""} /></label>
        <label className="min-w-0 w-full sm:w-64">单号<Input name="id" defaultValue={filters.id ?? ""} /></label>
        <label className="min-w-0 w-full sm:w-44">状态<NativeSelect name="state" aria-label="状态" defaultValue={filters.state ?? ""}>
          <option value="">全部状态</option>
          {Object.entries(releaseStateLabels).filter(([state]) => state !== "DRAFT").map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </NativeSelect></label>
        <Button type="submit">查询审批</Button>
        <Button type="button" disabled={list.isFetching} onClick={() => void list.refetch()}>{list.isFetching ? "正在刷新…" : "刷新列表"}</Button>
      </form>
      {list.isError && <><ErrorState error={list.error} message="审批列表读取失败，请重试。" onRetry={() => void list.refetch()} />{list.data && <p className="px-4 pb-4 text-sm text-muted-foreground">刷新失败，以下保留上次读取的结果。</p>}</>}
      {list.isPending ? <LoadingState /> : list.data && <>
        <Table className="min-w-[920px] table-fixed" containerProps={{ tabIndex: 0, role: "region", "aria-label": `${currentView.label}发布单`, className: "focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring" }}>
          <TableHeader><TableRow>
            <TableHead className="w-80">标题 / 单号 / 表</TableHead><TableHead className="w-52">申请人账号 ID</TableHead><TableHead className="w-36">状态 / 审批进度</TableHead><TableHead className="w-24">变更</TableHead><TableHead className="w-44">最近更新</TableHead><TableHead className="sticky right-0 z-10 w-28 bg-card">操作</TableHead>
          </TableRow></TableHeader>
          <TableBody>{list.data.orders.map(order => {
            const detailPath = `${listPath}/${encodeURIComponent(order.id)}?${search}`;
            return <TableRow key={order.id}>
              <TableCell><Link className="font-medium break-all underline-offset-4 hover:underline" to={detailPath}>{order.title}</Link><p className="mt-1 break-all font-mono text-xs text-muted-foreground">{order.id}</p><p className="mt-1 break-all font-mono text-xs text-muted-foreground">{releaseTables(order).join("、")}</p></TableCell>
              <TableCell className="break-all font-mono text-xs">{order.applicant_id}</TableCell>
              <TableCell>{releaseStateLabels[order.state]}{order.approvals.length > 0 && <p className="mt-1 text-xs text-muted-foreground">已通过 {order.approvals.filter(table => table.state === "APPROVED").length} / {order.approvals.length} 表</p>}</TableCell>
              <TableCell>{order.item_count} 项</TableCell>
              <TableCell className="text-xs"><ReleaseTime value={order.updated_at} /></TableCell>
              <TableCell className="sticky right-0 z-10 bg-card whitespace-nowrap"><PrimitiveButton asChild variant="ghost" size="sm"><Link to={detailPath} aria-label={`查看详情：${order.title}`}>查看详情</Link></PrimitiveButton></TableCell>
            </TableRow>;
          })}</TableBody>
        </Table>
        {list.data.orders.length === 0 && <p className="feedback-state">{filtered ? "没有符合筛选条件的发布单。" : currentView.empty}</p>}
      </>}
      <footer className="flex flex-wrap items-center justify-between gap-3 border-t p-4">
        <p className="text-xs text-muted-foreground">{filters.after ? "后续页" : "首页"} · 按单号升序</p>
        <div className="flex flex-wrap gap-2"><Button disabled={!filters.after || list.isFetching} onClick={() => setParams(firstPage)}>回到首页</Button><Button disabled={!list.data?.next_cursor || list.isFetching || list.isError} onClick={() => setParams({ ...filters, after: list.data!.next_cursor })}>下一页</Button></div>
      </footer>
    </section>
  </>;
}
