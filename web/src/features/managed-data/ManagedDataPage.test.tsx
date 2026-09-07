import { withDefaultRecordVersions } from "../../test/managed-data-fixture";
import { withAdminSession } from "../../test/account-session";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";

const enabledPolicy = {
  table_name: "notification_templates",
  query_policy_code: "notification_page_query_v1",
  mutation_policy_code: "notification_full_mutation_v1",
  enabled: true,
  creator: "local-fixture",
  modifier: "local-fixture",
  gmt_created: "2026-08-25T09:00:00Z",
  gmt_modified: "2026-08-25T09:00:00Z",
};

function json(value: unknown, status = 200, requestId = "req-managed-data") {
  value = withDefaultRecordVersions(value);
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json", "X-Request-ID": requestId },
  });
}

function renderPage(queryRetry: boolean | number = false) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: queryRetry, retryDelay: 0 }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TestRouter initialEntries={["/configuration/managed-data"]}>
        <ToastProvider><AppRoutes /></ToastProvider>
      </TestRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("配置内容管理页面", () => {
  it("没有已启用表规则时显示明确空状态且不调用 Managed Data API", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [{ ...enabledPolicy, enabled: false }] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));

    renderPage();

    expect(await screen.findByText("没有可用的 Managed Table")).toBeVisible();
    expect(screen.getByText("请先为真实数据库表创建并启用完整的表规则。")).toBeVisible();
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes("/tables/"))).toBe(false);
  });

  it("只列出 enabled Managed Table，并用首次查询的动态列区分 NULL 与空字符串", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [
        enabledPolicy,
        { ...enabledPolicy, table_name: "disabled_templates", enabled: false },
      ] });
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        return json({
          columns: [
            { name: "id", type: "uint64", nullable: false },
            { name: "subject", type: "string", nullable: true },
            { name: "body", type: "string", nullable: false },
          ],
          rows: [
            { id: "2", subject: null, body: "Payment received" },
            { id: "1", subject: "", body: "Welcome" },
          ],
          page: { page_number: 1, page_size: 20, total_count: 2, total_pages: 1 },
        });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage();

    expect(await screen.findByRole("heading", { name: "配置内容管理" })).toBeVisible();
    expect(screen.getByRole("link", { name: "配置内容管理" })).toHaveClass("active");
    const tableSelect = await screen.findByRole("combobox", { name: "Managed Table" });
    expect(within(tableSelect).getByRole("option", { name: "notification_templates" })).toBeVisible();
    expect(within(tableSelect).queryByRole("option", { name: "disabled_templates" })).not.toBeInTheDocument();
    expect(await screen.findByRole("columnheader", { name: /subject/ })).toHaveTextContent("string");
    expect(screen.getByRole("columnheader", { name: /subject/ })).toHaveTextContent("可为 NULL");
    expect(screen.getByRole("columnheader", { name: /id/ })).toHaveTextContent("非 NULL");
    expect(screen.getByText("NULL")).toBeVisible();
    expect(screen.getByText("空字符串")).toBeVisible();

    const queryCall = fetchMock.mock.calls.find(([url]) => String(url).endsWith("/tables/notification_templates/query"));
    expect(JSON.parse(String(queryCall?.[1]?.body))).toEqual({ conditions: [], page_number: 1 });
    await user.click(screen.getByRole("button", { name: "重新查询" }));
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/tables/notification_templates/query"))).toHaveLength(2);
  });

  it("切换 enabled Managed Table 时重置 Query Spec 并查询所选表的动态列", async () => {
    const auditPolicy = { ...enabledPolicy, table_name: "audit_messages" };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy, auditPolicy] });
      if (url.includes("/tables/") && url.endsWith("/query")) {
        const table = url.includes("audit_messages") ? "audit_messages" : "notification_templates";
        return json({
          columns: [
            { name: "id", type: "uint64", nullable: false },
            { name: table === "audit_messages" ? "event_name" : "subject", type: "string", nullable: true },
          ],
          rows: [],
          page: { page_number: 1, page_size: 20, total_count: 0, total_pages: 0 },
        });
      }
      throw new Error(`unexpected request ${url} ${String(init?.body)}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage();
    await screen.findByRole("columnheader", { name: /subject/ });
    await user.selectOptions(screen.getByRole("combobox", { name: "Managed Table" }), "audit_messages");

    expect(await screen.findByRole("columnheader", { name: /event_name/ })).toBeVisible();
    const queryCalls = fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"));
    expect(String(queryCalls.at(-1)?.[0])).toContain("/tables/audit_messages/query");
    expect(JSON.parse(String(queryCalls.at(-1)?.[1]?.body))).toEqual({ conditions: [], page_number: 1 });
  });

  it("把空字符串 exact 条件作为真实 JSON String 提交", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.endsWith("/tables/notification_templates/query")) return json({
        columns: [
          { name: "id", type: "uint64", nullable: false },
          { name: "subject", type: "string", nullable: true },
        ],
        rows: [],
        page: { page_number: 1, page_size: 20, total_count: 0, total_pages: 0 },
      });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage();
    await screen.findByRole("columnheader", { name: /subject/ });
    await user.click(screen.getByRole("button", { name: "添加条件" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "条件 1 字段" }), "subject");
    expect(screen.getByRole("textbox", { name: "条件 1 值" })).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "查询" }));

    await vi.waitFor(() => expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(2));
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      conditions: [{ field: "subject", operator: "exact", value: "" }],
      page_number: 1,
    });
  });

  it("根据字段类型构造其余七种操作符，并把八个条件作为 AND Query Spec 提交", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.endsWith("/tables/notification_templates/query")) return json({
        columns: [
          { name: "id", type: "uint64", nullable: false },
          { name: "body", type: "string", nullable: false },
          { name: "priority", type: "int64", nullable: false },
          { name: "active_from", type: "date", nullable: true },
          { name: "channel", type: "string", nullable: false },
          { name: "subject", type: "string", nullable: true },
          { name: "metadata", type: "json", nullable: true },
        ],
        rows: [],
        page: { page_number: 1, page_size: 20, total_count: 0, total_pages: 0 },
      });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage();
    await screen.findByRole("columnheader", { name: /metadata/ });
    for (let index = 0; index < 8; index += 1) await user.click(screen.getByRole("button", { name: "添加条件" }));

    const setCondition = async (index: number, field: string, operator: string) => {
      await user.selectOptions(screen.getByRole("combobox", { name: `条件 ${index} 字段` }), field);
      await user.selectOptions(screen.getByRole("combobox", { name: `条件 ${index} 操作符` }), operator);
    };
    await setCondition(1, "subject", "exact");
    await setCondition(2, "body", "contains");
    await user.type(screen.getByRole("textbox", { name: "条件 2 值" }), "ready");
    await setCondition(3, "priority", "open_range");
    await user.clear(screen.getByRole("textbox", { name: "条件 3 下界" }));
    await user.type(screen.getByRole("textbox", { name: "条件 3 下界" }), "10");
    await user.click(screen.getByRole("checkbox", { name: "条件 3 使用上界" }));
    await user.type(screen.getByRole("textbox", { name: "条件 3 上界" }), "30");
    await setCondition(4, "active_from", "closed_range");
    await user.type(screen.getByLabelText("条件 4 下界"), "2026-01-01");
    await setCondition(5, "channel", "in");
    await user.type(screen.getByRole("textbox", { name: "条件 5 集合值 1" }), "EMAIL");
    await user.click(screen.getByRole("button", { name: "条件 5 添加集合值" }));
    await setCondition(6, "channel", "not_in");
    await user.type(screen.getByRole("textbox", { name: "条件 6 集合值 1" }), "SMS");
    await setCondition(7, "subject", "is_null");
    await setCondition(8, "metadata", "is_not_null");

    expect(within(screen.getByRole("combobox", { name: "条件 8 操作符" })).queryByRole("option", { name: "contains" })).not.toBeInTheDocument();
    expect(within(screen.getByRole("combobox", { name: "条件 8 操作符" })).queryByRole("option", { name: "open_range" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "查询" }));

    await vi.waitFor(() => expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(2));
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body)).conditions).toEqual([
      { field: "subject", operator: "exact", value: "" },
      { field: "body", operator: "contains", value: "ready" },
      { field: "priority", operator: "open_range", from: "10", to: "30" },
      { field: "active_from", operator: "closed_range", from: "2026-01-01" },
      { field: "channel", operator: "in", values: ["EMAIL", ""] },
      { field: "channel", operator: "not_in", values: ["SMS"] },
      { field: "subject", operator: "is_null" },
      { field: "metadata", operator: "is_not_null" },
    ]);
  });

  it("提交单字段排序并只用服务端页信息翻页、设置每页数量和清空查询", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.endsWith("/tables/notification_templates/query")) {
        const body = JSON.parse(String(init?.body));
        const pageNumber = body.page_number ?? 1;
        const pageSize = body.page_size ?? 10;
        return json({
          columns: [
            { name: "id", type: "uint64", nullable: false },
            { name: "priority", type: "int64", nullable: false },
          ],
          rows: [{ id: String(22 - pageNumber), priority: "10" }],
          page: { page_number: pageNumber, page_size: pageSize, total_count: 21, total_pages: 3 },
        });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage();
    expect(await screen.findByText("第 1 / 3 页")).toBeVisible();
    await user.selectOptions(screen.getByRole("combobox", { name: "排序字段" }), "priority");
    await user.selectOptions(screen.getByRole("combobox", { name: "排序方向" }), "ASC");
    await user.clear(screen.getByRole("spinbutton", { name: "每页数量" }));
    await user.type(screen.getByRole("spinbutton", { name: "每页数量" }), "10");
    await user.click(screen.getByRole("button", { name: "查询" }));

    await vi.waitFor(() => expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(2));
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      conditions: [],
      order: { field: "priority", direction: "ASC" },
      page_number: 1,
      page_size: 10,
    });
    await user.click(screen.getByRole("button", { name: "下一页" }));
    expect(await screen.findByText("第 2 / 3 页")).toBeVisible();
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body)).page_number).toBe(2);
    expect(screen.getByRole("button", { name: "上一页" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "清空" }));
    await vi.waitFor(() => expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(4));
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({ conditions: [], page_number: 1 });
  });

  it("即使 Query Spec 已经为空，清空也会重新查询", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.endsWith("/tables/notification_templates/query")) return json({
        columns: [{ name: "id", type: "uint64", nullable: false }],
        rows: [],
        page: { page_number: 1, page_size: 20, total_count: 0, total_pages: 0 },
      });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage();
    await screen.findByRole("columnheader", { name: /id/ });
    await user.click(screen.getByRole("button", { name: "清空" }));

    await vi.waitFor(() => expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(2));
  });

  it("清晰呈现 Managed Data 稳定错误和 Request ID", async () => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.endsWith("/tables/notification_templates/query")) {
        return json({ error: { code: "query_timeout", message: "Managed Table query timed out", request_id: "req-timeout-27" } }, 504);
      }
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage();

    const alert = await screen.findByRole("alert", undefined, { timeout: 3_000 });
    expect(alert).toHaveTextContent("Managed Table 查询超时，请缩小条件或分页范围后重试");
    expect(alert).toHaveTextContent("req-timeout-27");
  });

  it.each([
    ["invalid_policy_snapshot", "当前规则快照无法执行", "req-policy-27"],
    ["incompatible_table", "表结构不符合 Managed Table 要求", "req-schema-27"],
  ])("呈现 %s 稳定错误和 Request ID", async (code, expectedMessage, requestId) => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.endsWith("/tables/notification_templates/query")) {
        return json({ error: { code, message: "unsafe backend detail", request_id: requestId } }, 422);
      }
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage();

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(expectedMessage);
    expect(alert).toHaveTextContent(requestId);
    expect(alert).not.toHaveTextContent("unsafe backend detail");
  });

  it("不会自动重放 POST Query Spec", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.endsWith("/tables/notification_templates/query")) {
        return json({ error: { code: "query_unavailable", message: "down", request_id: "req-no-retry-27" } }, 503);
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));

    renderPage(1);
    expect(await screen.findByRole("alert")).toHaveTextContent("req-no-retry-27");
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(1);
  });

  it("在浏览器边界拒绝无边界 Range、非法分页，并把 AND 条件限制为 20 个", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.endsWith("/tables/notification_templates/query")) return json({
        columns: [
          { name: "id", type: "uint64", nullable: false },
          { name: "priority", type: "int64", nullable: false },
        ],
        rows: [],
        page: { page_number: 1, page_size: 20, total_count: 0, total_pages: 0 },
      });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage();
    await screen.findByRole("columnheader", { name: /priority/ });
    await user.click(screen.getByRole("button", { name: "添加条件" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "条件 1 字段" }), "priority");
    await user.selectOptions(screen.getByRole("combobox", { name: "条件 1 操作符" }), "open_range");
    await user.click(screen.getByRole("checkbox", { name: "条件 1 使用下界" }));
    await user.click(screen.getByRole("button", { name: "查询" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Range 至少需要一个边界");
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(1);

    await user.clear(screen.getByRole("spinbutton", { name: "每页数量" }));
    await user.type(screen.getByRole("spinbutton", { name: "每页数量" }), "201");
    await user.click(screen.getByRole("button", { name: "查询" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("每页数量必须是 1 到 200 的整数");
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(1);

    for (let index = 1; index < 20; index += 1) await user.click(screen.getByRole("button", { name: "添加条件" }));
    expect(screen.getByRole("button", { name: "添加条件" })).toBeDisabled();
    expect(screen.getAllByRole("group", { name: /条件 \d+/ })).toHaveLength(20);
  });
});
