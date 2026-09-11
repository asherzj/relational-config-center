import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";
import { TestRouter } from "../../test/TestRouter";
import { testIdentity } from "../../test/account-session";

function mount(path = "/configuration/notifications") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false }, mutations: { retry: false } } });
  return { client, ...render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={[path]}><AppRoutes /></TestRouter></ToastProvider></QueryClientProvider>) };
}
const failure = (id: string, status = 500) => Response.json({ error: { code: "release_unavailable", message: "暂时无法读取", request_id: id } }, { status });
function session(implementation: typeof fetch): typeof fetch {
  return (input, init) => ["/api/v1/auth/session", "/api/v1/auth/activity"].includes(String(input))
    ? Promise.resolve(Response.json(testIdentity)) : implementation(input, init);
}
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks(); sessionStorage.clear(); });

it("个人未读入口展示真实未读与独立待审批数，筛选失败仍保留计数", async () => {
  let fail = false;
  const lists: URL[] = [];
  vi.stubGlobal("fetch", session(async input => {
    const url = new URL(String(input), "https://example.test");
    if (url.pathname === "/api/v1/approval-notifications") return fail ? failure("counts-failure") : Response.json({ unread_count: 3, pending_count: 7 });
    lists.push(url);
    return Response.json({ orders: [], next_cursor: "" });
  }));
  const user = userEvent.setup(); mount();
  const entry = await screen.findByRole("link", { name: "个人未读 3" });
  expect(entry).toHaveAttribute("href", "/configuration/notifications?view=all&unread=true");
  expect(screen.getByLabelText("个人待审批数")).toHaveTextContent("待审批 7");
  await user.click(entry);
  await waitFor(() => expect(lists.at(-1)?.searchParams.get("unread")).toBe("true"));
  expect(screen.getByRole("checkbox", { name: "仅看未读" })).toBeChecked();
  fail = true;
  await user.click(screen.getByRole("button", { name: "刷新通知" }));
  expect(await screen.findByText("未读与待审批数刷新失败，保留上次读取的计数。")).toBeVisible();
  expect(screen.getByRole("link", { name: "个人未读 3" })).toBeVisible();
  expect(screen.getByLabelText("个人待审批数")).toHaveTextContent("待审批 7");
});

it("前台每 30 秒和返回时刷新，隐藏与会话中断停止受保护读取", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
  let count = 1, reads = 0;
  vi.stubGlobal("fetch", session(async input => {
    reads++;
    if (String(input) === "/api/v1/approval-notifications") return Response.json({ unread_count: count, pending_count: 7 });
    return Response.json({ orders: count === 1 ? [] : [{ ...header, title: count === 2 ? "计时刷新后的申请" : "返回后的申请", notification: { sequence: "0", unread: false, pending: false } }], next_cursor: "" });
  }));
  mount();
  await screen.findByRole("link", { name: "个人未读 1" });
  count = 2;
  await act(async () => vi.advanceTimersByTimeAsync(29_999));
  expect(screen.getByRole("link", { name: "个人未读 1" })).toBeVisible();
  await act(async () => vi.advanceTimersByTimeAsync(1));
  await screen.findByRole("link", { name: "个人未读 2" });
  await screen.findByRole("link", { name: "查看详情：计时刷新后的申请" });
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  fireEvent(document, new Event("visibilitychange"));
  const backgroundReads = reads;
  count = 4;
  await act(async () => vi.advanceTimersByTimeAsync(60_000));
  expect(reads).toBe(backgroundReads);
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  fireEvent(document, new Event("visibilitychange"));
  await screen.findByRole("link", { name: "个人未读 4" });
  await screen.findByRole("link", { name: "查看详情：返回后的申请" });
  act(() => window.dispatchEvent(new CustomEvent("rcc:business-session-invalid", { detail: { code: "session_invalid" } })));
  await screen.findByRole("heading", { name: "登录本地账号" });
  const suspendedReads = reads;
  await act(async () => vi.advanceTimersByTimeAsync(90_000));
  fireEvent(window, new Event("focus"));
  fireEvent(document, new Event("visibilitychange"));
  await act(async () => Promise.resolve());
  expect(reads).toBe(suspendedReads);
});

const header = {
  release_type: "STANDARD", emergency_reason: "", table_flows: [], rollback_table_flows: [], missing_flow_tables: [],
  id: "a".repeat(32), title: "已展示的审批进展", table_names: [], applicant_id: "another-account", state: "PENDING_APPROVAL", version: "2",
  created_at: "2026-09-11T01:00:00Z", updated_at: "2026-09-11T02:00:00Z", item_count: 0, operation_counts: {}, allowed_actions: [],
  approvals: [], approval_context: { revision: "approval-2", tables: [], approvable_tables: [] }, history: [], executions: [],
  notification: { sequence: "9007199254740993", unread: true, pending: true },
};
it("详情只确认成功展示的通知进度，已读失败不阻断阅读且仅手动重试原进度", async () => {
  const acknowledgements: RequestInit[] = [];
  let headers = 0;
  vi.stubGlobal("fetch", session(async (input, init) => {
    const path = String(input);
    if (path === "/api/v1/approval-notifications") return Response.json({ unread_count: 1, pending_count: 1 });
    if (path.endsWith("/notification-read")) {
      acknowledgements.push(init!);
      return acknowledgements.length === 1 ? failure("read-failure") : Response.json({ sequence: "9007199254740994", unread: true, pending: true });
    }
    if (path.includes("/details?")) return Response.json({ order_id: header.id, version: header.version, item_count: 0, offset: 0, next_offset: null, items: [] });
    if (path.endsWith("/people")) return Response.json({ people: {} });
    if (path.endsWith(header.id)) { headers++; return Response.json(header); }
    return Response.json({ orders: [header], next_cursor: "" });
  }));
  const user = userEvent.setup(); mount(`/configuration/notifications/${header.id}?view=all&unread=true`);
  expect(await screen.findByRole("heading", { name: header.title })).toBeVisible();
  expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  expect(screen.getByRole("link", { name: "返回通知中心列表" })).toBeVisible();
  expect(await screen.findByText("标记已读失败，未读提醒已保留；详情仍可继续查看。")).toBeVisible();
  expect(screen.getByRole("alert")).toHaveTextContent("read-failure");
  expect(acknowledgements).toHaveLength(1);
  expect(JSON.parse(String(acknowledgements[0]!.body))).toEqual({ sequence: "9007199254740993" });
  expect(new Headers(acknowledgements[0]!.headers).get("X-CSRF-Token")).toBe(testIdentity.csrf_token);
  await user.click(screen.getByRole("button", { name: "重试标记已读" }));
  expect(await screen.findByText("本单还有新的未读变化。")).toBeVisible();
  expect(acknowledgements).toHaveLength(2);
  expect(acknowledgements[1]!.body).toBe(acknowledgements[0]!.body);
  expect(headers).toBe(1);
  expect(screen.getByRole("link", { name: "返回通知中心列表" })).toHaveAttribute("href", expect.stringContaining("unread=true"));
});

it("已读失败后刷新同一详情不会自动重试，成功读取新进度才确认新的序号", async () => {
  const seen: string[] = [];
  let sequence = "10", readCount = 0;
  let finishRefresh!: () => void;
  vi.stubGlobal("fetch", session(async (input, init) => {
    const path = String(input);
    if (path === "/api/v1/approval-notifications") return Response.json({ unread_count: 1, pending_count: 1 });
    if (path.endsWith("/notification-read")) { seen.push(JSON.parse(String(init!.body)).sequence); return failure("ack-offline"); }
    if (path.includes("/details?")) return Response.json({ order_id: header.id, version: header.version, item_count: 0, offset: 0, next_offset: null, items: [] });
    if (path.endsWith("/people")) return Response.json({ people: {} });
    readCount++;
    if (readCount === 2) await new Promise<void>(resolve => { finishRefresh = resolve; });
    return Response.json({ ...header, notification: { sequence, unread: true, pending: true } });
  }));
  const { client } = mount(`/configuration/release-orders/${header.id}`);
  await screen.findByText("标记已读失败，未读提醒已保留；详情仍可继续查看。");
  act(() => { void client.invalidateQueries({ queryKey: ["release-order", header.id] }); });
  await waitFor(() => expect(readCount).toBe(2));
  expect(screen.getByText("标记已读失败，未读提醒已保留；详情仍可继续查看。")).toBeVisible();
  await act(async () => { finishRefresh(); });
  expect(seen).toEqual(["10"]);
  sequence = "11";
  await act(async () => { await client.invalidateQueries({ queryKey: ["release-order", header.id] }); });
  await waitFor(() => expect(seen).toEqual(["10", "11"]));
});

it("切换账号后旧账号迟到的计数响应不能覆盖新账号，未读不进入持久存储", async () => {
  let identity = testIdentity, defer = false;
  let finishOld!: (response: Response) => void;
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const path = String(input);
    if (["/api/v1/auth/session", "/api/v1/auth/activity"].includes(path)) return Response.json(identity);
    if (path === "/api/v1/approval-notifications") {
      if (defer && identity.account.id === testIdentity.account.id) return new Promise<Response>(resolve => { finishOld = resolve; });
      return Response.json({ unread_count: identity.account.id === testIdentity.account.id ? 3 : 8, pending_count: 2 }, { headers: { "X-RCC-Account-ID": identity.account.id } });
    }
    return Response.json({ orders: [], next_cursor: "" });
  });
  const user = userEvent.setup(); mount();
  await screen.findByRole("link", { name: "个人未读 3" });
  defer = true;
  await user.click(screen.getByRole("button", { name: "刷新通知" }));
  identity = { ...testIdentity, csrf_token: "other-csrf", account: { ...testIdentity.account, id: "99999999-2222-4333-8444-555555555555", username: "other", display_name: "另一账号" } };
  act(() => window.dispatchEvent(new CustomEvent("rcc:business-session-invalid", { detail: { code: "account_changed" } })));
  await screen.findByRole("link", { name: "个人未读 8" });
  await act(async () => finishOld(Response.json({ unread_count: 99, pending_count: 99 }, { headers: { "X-RCC-Account-ID": testIdentity.account.id } })));
  expect(screen.getByRole("link", { name: "个人未读 8" })).toBeVisible();
  expect(screen.queryByRole("link", { name: "个人未读 99" })).not.toBeInTheDocument();
  expect(JSON.stringify({ ...localStorage, ...sessionStorage })).not.toContain("unread_count");
});

it("详情读取失败不确认未见进度，首次计数失败不显示虚假的零", async () => {
  let acknowledgements = 0;
  vi.stubGlobal("fetch", session(async input => {
    const path = String(input);
    if (path.endsWith("/notification-read")) acknowledgements++;
    return failure(path === "/api/v1/approval-notifications" ? "first-count-failure" : "detail-failure");
  }));
  mount(`/configuration/notifications/${header.id}`);
  expect(await screen.findByRole("link", { name: "个人未读 暂不可用" })).toBeVisible();
  expect(await screen.findByRole("alert")).toHaveTextContent("detail-failure");
  expect(screen.queryByRole("link", { name: "个人未读 0" })).not.toBeInTheDocument();
  expect(acknowledgements).toBe(0);
});

it("从详情完成审批动作后立即读取真实待审批数，不等待下一次轮询", async () => {
  let cancelled = false;
  const current = () => ({ ...header, notification: { sequence: "0", unread: false, pending: false }, state: cancelled ? "CANCELLED" : "PENDING_APPROVAL", allowed_actions: cancelled ? [] : ["cancel"] });
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const path = String(input);
    if (["/api/v1/auth/session", "/api/v1/auth/activity"].includes(path)) return Response.json({ ...testIdentity, account: { ...testIdentity.account, roles: ["ADMIN"] } });
    if (path === "/api/v1/approval-notifications") return Response.json({ unread_count: 0, pending_count: cancelled ? 0 : 1 });
    if (path.endsWith("/cancel")) { cancelled = true; return Response.json({ ...current(), items: [] }); }
    if (path.includes("/details?")) return Response.json({ order_id: header.id, version: header.version, item_count: 0, offset: 0, next_offset: null, items: [] });
    if (path.endsWith("/people")) return Response.json({ people: {} });
    return Response.json(current());
  });
  const user = userEvent.setup(); mount(`/configuration/notifications/${header.id}`);
  await user.click(await screen.findByRole("button", { name: "更多操作" }));
  expect(screen.getByLabelText("个人待审批数")).toHaveTextContent("待审批 1");
  await user.click(screen.getByRole("menuitem", { name: "取消发布单" }));
  await user.type(screen.getByRole("textbox", { name: "取消原因" }), "取消当前申请");
  await user.click(screen.getByRole("button", { name: "确认取消发布单" }));
  await waitFor(() => expect(screen.getByLabelText("个人待审批数")).toHaveTextContent("待审批 0"));
});

it("详情在后台标签加载完成时不标记已读，返回前台实际展示后才确认", async () => {
  const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
  let acknowledgements = 0;
  vi.stubGlobal("fetch", session(async input => {
    const path = String(input);
    if (path === "/api/v1/approval-notifications") return Response.json({ unread_count: 1, pending_count: 1 });
    if (path.endsWith("/notification-read")) { acknowledgements++; return Response.json({ ...header.notification, unread: false }); }
    if (path.includes("/details?")) return Response.json({ order_id: header.id, version: header.version, item_count: 0, offset: 0, next_offset: null, items: [] });
    if (path.endsWith("/people")) return Response.json({ people: {} });
    return Response.json(header);
  }));
  mount(`/configuration/notifications/${header.id}`);
  await screen.findByRole("heading", { name: header.title });
  await act(async () => Promise.resolve());
  expect(acknowledgements).toBe(0);
  visibility.mockReturnValue("visible");
  fireEvent(document, new Event("visibilitychange"));
  await screen.findByText("已读；本单仍待你审批。");
  expect(acknowledgements).toBe(1);
});

it("返回列表后才完成的已读确认仍更新当前账号计数和列表", async () => {
  let read = false, finishRead!: () => void;
  vi.stubGlobal("fetch", session(async input => {
    const path = String(input);
    if (path === "/api/v1/approval-notifications") return Response.json({ unread_count: read ? 0 : 1, pending_count: 1 });
    if (path.endsWith("/notification-read")) { await new Promise<void>(resolve => { finishRead = resolve; }); read = true; return Response.json({ ...header.notification, unread: false }); }
    if (path.includes("/details?")) return Response.json({ order_id: header.id, version: header.version, item_count: 0, offset: 0, next_offset: null, items: [] });
    if (path.endsWith("/people")) return Response.json({ people: {} });
    if (path.includes("release-orders?")) return Response.json({ orders: [{ ...header, notification: { ...header.notification, unread: !read } }], next_cursor: "" });
    return Response.json(header);
  }));
  const user = userEvent.setup(); mount(`/configuration/notifications/${header.id}`);
  await screen.findByText("正在标记已展示的进展为已读…");
  await user.click(screen.getByRole("link", { name: "返回通知中心列表" }));
  await screen.findByRole("link", { name: `查看详情：${header.title}` });
  expect(screen.getByRole("link", { name: "个人未读 1" })).toBeVisible();
  await act(async () => finishRead());
  await screen.findByRole("link", { name: "个人未读 0" });
  expect(within(screen.getByRole("row", { name: new RegExp(header.title) })).queryByText("未读", { exact: true })).not.toBeInTheDocument();
});
