import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useLocation } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";
import { TestRouter } from "../../test/TestRouter";
import { withAccountSession } from "../../test/account-session";

function LocationProbe() {
  const location = useLocation();
  return <output aria-label="当前地址">{location.pathname + location.search}</output>;
}
function mount(path = "/configuration/notifications") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={[path]}><AppRoutes /><LocationProbe /></TestRouter></ToastProvider></QueryClientProvider>);
}
afterEach(() => { vi.unstubAllGlobals(); sessionStorage.clear(); });

it("查看者从正式通知中心路由默认读取待我审批，不发送他人身份筛选", async () => {
  const reads: URL[] = [];
  vi.stubGlobal("fetch", withAccountSession(vi.fn(async input => {
    const url = new URL(String(input), "https://example.test");
    reads.push(url);
    return Response.json({ orders: [], next_cursor: "" });
  })));
  mount();
  expect(await screen.findByRole("heading", { name: "通知中心" })).toBeVisible();
  await waitFor(() => expect(reads.some(url => url.pathname === "/api/v1/release-orders" && url.searchParams.get("view") === "pending")).toBe(true));
  expect(screen.getByRole("link", { name: "通知中心" })).toHaveAttribute("aria-current", "page");
  expect(screen.getByRole("link", { name: "待我审批" })).toHaveAttribute("aria-current", "page");
  expect(reads.every(url => !url.searchParams.has("applicant_id"))).toBe(true);
  expect(await screen.findByText("暂无待你审批的发布单。")).toBeVisible();
});

it("从后续页进入详情，加载与失败期间均能返回原视图和筛选页", async () => {
  const reads: URL[] = [];
  let failDetail!: (response: Response) => void;
  const detailResult = new Promise<Response>(resolve => { failDetail = resolve; });
  vi.stubGlobal("fetch", withAccountSession(vi.fn(async input => {
    const url = new URL(String(input), "https://example.test");
    reads.push(url);
    if (url.pathname === `/api/v1/release-orders/${summary.id}`) return detailResult;
    return Response.json({ orders: [summary], next_cursor: "" });
  })));
  const user = userEvent.setup();
  mount("/configuration/notifications?view=handled&table_name=items&state=PENDING_APPROVAL&after=previous-cursor");
  await user.click(await screen.findByRole("link", { name: `查看详情：${summary.title}` }));
  const back = await screen.findByRole("link", { name: "返回通知中心列表" });
  expect(within(screen.getByRole("main")).getByRole("status")).toHaveTextContent("正在加载");
  const destination = new URL(back.getAttribute("href")!, "https://example.test");
  expect(destination.pathname).toBe("/configuration/notifications");
  expect(destination.searchParams.get("view")).toBe("handled");
  expect(destination.searchParams.get("after")).toBe("previous-cursor");
  await act(async () => failDetail(Response.json({ error: { code: "release_unavailable", message: "详情暂时不可用", request_id: "notification-detail-failure" } }, { status: 500 })));
  expect(await screen.findByRole("alert")).toHaveTextContent("notification-detail-failure");
  await user.click(screen.getByRole("link", { name: "返回通知中心列表" }));
  expect(await screen.findByRole("link", { name: `查看详情：${summary.title}` })).toBeVisible();
  expect(screen.getByRole("textbox", { name: "表名" })).toHaveValue("items");
  expect(screen.getByRole("combobox", { name: "状态" })).toHaveValue("PENDING_APPROVAL");
  expect(screen.getByLabelText("当前地址")).toHaveTextContent("after=previous-cursor");
  await waitFor(() => expect(reads.at(-1)?.searchParams.get("view")).toBe("handled"));
});

const summary = {
  release_type: "STANDARD", table_flows: [], missing_flow_tables: [],
  notification: { sequence: "0", unread: false, pending: false },
  id: "a".repeat(32), title: "运营与财务共同核对的多表申请", table_names: ["items", "prices"],
  applicant_id: "applicant-permanent-id", state: "PENDING_APPROVAL", version: "2",
  created_at: "2026-09-11T01:00:00Z", updated_at: "2026-09-11T02:00:00Z", item_count: 2,
  operation_counts: { MODIFY: 2 }, allowed_actions: ["approve", "reject"],
  approvals: [{ table_name: "items", roles: [], state: "APPROVED" }, { table_name: "prices", roles: [], state: "PENDING" }],
  approval_context: { revision: "revision-2", tables: [{ table_name: "prices", mode: "ROLE", reason: "当前角色成员", can_approve: true }], approvable_tables: ["prices"] },
};
it("四类视图和筛选写入 URL，分页沿用筛选且一张多表申请只显示一行", async () => {
  const reads: URL[] = [];
  vi.stubGlobal("fetch", withAccountSession(vi.fn(async input => {
    const url = new URL(String(input), "https://example.test");
    reads.push(url);
    return Response.json({ orders: [summary], next_cursor: url.searchParams.has("after") ? "" : "cursor+next/=" });
  })));
  const user = userEvent.setup();
  mount();
  await screen.findByRole("link", { name: `查看详情：${summary.title}` });
  expect(screen.getByRole("row", { name: new RegExp(summary.title) })).toHaveTextContent("已通过 1 / 2 表");
  expect(screen.getAllByRole("row")).toHaveLength(2);
  for (const [label, view] of [["我发起的", "mine"], ["我已处理", "handled"], ["全部审批", "all"]]) {
    await user.click(screen.getByRole("link", { name: label }));
    await waitFor(() => expect(reads.at(-1)?.searchParams.get("view")).toBe(view));
    expect(screen.getByLabelText("当前地址")).toHaveTextContent(`view=${view}`);
  }
  await user.type(screen.getByRole("textbox", { name: "表名" }), "items");
  await user.type(screen.getByRole("textbox", { name: "单号" }), summary.id);
  await user.selectOptions(screen.getByRole("combobox", { name: "状态" }), "PENDING_APPROVAL");
  await user.click(screen.getByRole("button", { name: "查询审批" }));
  await waitFor(() => expect(reads.at(-1)?.searchParams.get("table_name")).toBe("items"));
  await user.click(await screen.findByRole("button", { name: "下一页" }));
  await waitFor(() => expect(reads.at(-1)?.searchParams.get("after")).toBe("cursor+next/="));
  expect(reads.at(-1)?.searchParams.get("view")).toBe("all");
  expect(reads.at(-1)?.searchParams.get("id")).toBe(summary.id);
  expect(reads.at(-1)?.searchParams.get("state")).toBe("PENDING_APPROVAL");
  const detail = await screen.findByRole("link", { name: `查看详情：${summary.title}` });
  const destination = new URL(detail.getAttribute("href")!, "https://example.test");
  expect(destination.pathname).toBe(`/configuration/notifications/${summary.id}`);
  expect(destination.searchParams.get("after")).toBe("cursor+next/=");
  const beforeRefresh = reads.length;
  await user.click(screen.getByRole("button", { name: "刷新列表" }));
  await waitFor(() => expect(reads.length).toBeGreaterThan(beforeRefresh));
  expect(reads.at(-1)?.searchParams.get("after")).toBe("cursor+next/=");
  expect(screen.getByLabelText("当前地址")).toHaveTextContent("after=cursor%2Bnext%2F%3D");
  await user.click(screen.getByRole("link", { name: "待我审批" }));
  await waitFor(() => expect(reads.at(-1)?.searchParams.get("view")).toBe("pending"));
  expect(reads.at(-1)?.searchParams.has("after")).toBe(false);
  expect(screen.queryByRole("option", { name: "草稿" })).not.toBeInTheDocument();
});

it("刷新失败保留已知列表并说明读取失败，重试成功后恢复结果", async () => {
  let reads = 0;
  vi.stubGlobal("fetch", withAccountSession(vi.fn(async () => {
    reads++;
    if (reads === 2) return Response.json({ error: { code: "release_unavailable", message: "读取失败", request_id: "notification-list-failure" } }, { status: 500 });
    return Response.json({ orders: reads === 1 ? [summary] : [], next_cursor: "" });
  })));
  const user = userEvent.setup();
  mount();
  await screen.findByRole("link", { name: `查看详情：${summary.title}` });
  await user.click(screen.getByRole("button", { name: "刷新列表" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("notification-list-failure");
  expect(screen.getByRole("alert")).toHaveTextContent("审批列表读取失败，请重试。");
  expect(screen.getByRole("alert")).toHaveTextContent("release_unavailable");
  expect(screen.getByRole("alert")).not.toHaveTextContent("请保留原请求");
  expect(screen.getByText("刷新失败，以下保留上次读取的结果。")).toBeVisible();
  expect(screen.getByRole("link", { name: `查看详情：${summary.title}` })).toBeVisible();
  expect(screen.queryByText("暂无待你审批的发布单。")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "重试" }));
  expect(await screen.findByText("暂无待你审批的发布单。")).toBeVisible();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
