import { withDefaultRecordVersions } from "../../test/managed-data-fixture";
import { withAdminSession as withSession } from "../../test/account-session";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";

const enabledPolicy = {
  version: "1",
  table_name: "notification_templates",
  query_policy_code: "notification_page_query_v1",
  mutation_policy_code: "notification_full_mutation_v1",
  enabled: true,
  creator: "local-fixture",
  modifier: "local-fixture",
  created_at: "2026-08-25T09:00:00Z",
  updated_at: "2026-08-25T09:00:00Z",
};

let lastColumns: {name:string;type:string;nullable:boolean}[]=[];
function withAdminSession(implementation: typeof fetch): typeof fetch {
 return withSession(async (input,init) => {
  if(String(input).includes("/table-field-policies/")) return json({table_name:"notification_templates",query_capacity:{max_conditions:256,max_values_per_condition:100,queryable_fields:lastColumns.length,supported:true},fields:lastColumns.map(column=>({field_name:column.name,column_type:column.type,nullable:column.nullable,generated:false,auto_increment:false,has_default:false,state:"missing",warning:"",policy:null,audit:null,effective:{field_name:column.name,display_name:column.name,description:"",display_order:0,is_visible:true,is_queryable:true,query_operators:["exact"],ui_type:"text",ui_options:{options:[]},editable_on_add:true,editable_on_modify:true,is_required:false,enabled:false}}))});
  const response=await implementation(input,init);
  if(String(input).endsWith("/query") && response.ok) lastColumns=(await response.clone().json()).columns;
  return response;
 });
}
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

describe("统一变更入口页面", () => {
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

    expect(await screen.findByRole("heading", { name: "统一变更入口" })).toBeVisible();
    expect(screen.getByRole("link", { name: "统一变更入口" })).toHaveClass("active");
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

  it("按当前字段配置隐藏和排序列表列，并用真实字段和值消除重复标签歧义", async () => {
    const columns = [
      { name: "beta", type: "string", nullable: false },
      { name: "internal", type: "string", nullable: false },
      { name: "id", type: "uint64", nullable: false },
      { name: "alpha", type: "string", nullable: false },
    ];
    const field = (name: string, displayName: string, displayOrder: number, isVisible = true, optionValue?: string) => ({
      field_name: name, column_type: name === "id" ? "uint64" : "string", nullable: false, generated: false,
      auto_increment: false, has_default: false, state: "active", warning: "", audit: null,
      policy: null,
      effective: { field_name: name, display_name: displayName, description: "", display_order: displayOrder,
        is_visible: isVisible, is_queryable: true, query_operators: ["exact"], ui_type: optionValue ? "select" : "text",
        ui_options: { options: optionValue ? [{ label: "启用", value: optionValue }] : [] }, editable_on_add: true,
        editable_on_modify: true, is_required: false, enabled: true },
    });
    let fieldPolicyReads = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [enabledPolicy] });
      if (url.includes("/table-field-policies/")) {
        fieldPolicyReads++;
        return json({ table_name: "notification_templates", query_capacity: { max_conditions: 256, max_values_per_condition: 100, queryable_fields: 4, supported: true }, fields: [
          field("beta", "状态", 5, true, "b"), field("internal", "内部字段", 0, false), field("id", "编号", 1), field("alpha", "状态", 5, true, "a"),
        ] });
      }
      if (url.endsWith("/tables/notification_templates/query")) return json({
        columns, rows: [{ beta: "b", internal: "secret", id: "1", alpha: "a" }, { beta: "b", internal: "hidden", id: "2", alpha: "a" }],
        page: { page_number: 1, page_size: 20, total_count: 2, total_pages: 1 },
      });
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.includes("/query-policies/")) return json({code:"notification_page_query_v1",name:"Query",description:"",type_code:"page_query",default_order_field:"id",default_order_direction:"DESC",default_page_size:20,max_page_size:100,status:"ACTIVE",creator:"fixture",modifier:"fixture",created_at:"2026-08-25T09:00:00Z",updated_at:"2026-08-25T09:00:00Z"});
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.includes("/mutation-policies/")) return json({code:"notification_full_mutation_v1",name:"Mutation",description:"",type_code:"single_table_mutation",allow_add:true,allow_modify:true,allow_delete:true,create_operator_field:"",create_time_field:"",modify_operator_field:"",modify_time_field:"",status:"ACTIVE",creator:"fixture",modifier:"fixture",created_at:"2026-08-25T09:00:00Z",updated_at:"2026-08-25T09:00:00Z"});
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withSession(fetchMock));
    const user = userEvent.setup();

    renderPage();

    expect(await screen.findAllByText("启用")).toHaveLength(4);
    const headers = await screen.findAllByRole("columnheader");
    expect(headers.map(header => header.textContent)).toEqual(expect.arrayContaining([expect.stringMatching(/^编号id/), expect.stringMatching(/^状态alpha/), expect.stringMatching(/^状态beta/)]));
    expect(headers.findIndex(header => header.textContent?.startsWith("编号"))).toBeLessThan(headers.findIndex(header => header.textContent?.startsWith("状态alpha")));
    expect(headers.findIndex(header => header.textContent?.startsWith("状态alpha"))).toBeLessThan(headers.findIndex(header => header.textContent?.startsWith("状态beta")));
    expect(screen.queryByRole("columnheader", { name: /internal/ })).not.toBeInTheDocument();
    expect(screen.queryByText("secret")).not.toBeInTheDocument();
    expect(screen.getAllByLabelText("真实值：a")).toHaveLength(2);
    expect(screen.getAllByLabelText("真实值：b")).toHaveLength(2);
    expect(fieldPolicyReads).toBe(1);
    await user.click(screen.getByRole("checkbox", { name: "选择记录 1" }));
    await user.click(await screen.findByRole("button", { name: "删除已选 1 项" }));
    await user.click(screen.getByText(/明细 1 · 记录 1/));
    expect(await screen.findByText("secret")).toBeVisible();
    expect(screen.getAllByText("内部字段")).not.toHaveLength(0);
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
    await user.click(await screen.findByRole("checkbox", { name: "subject 值 空字符串" }));
    expect(screen.getByRole("textbox", { name: "筛选 subject 值" })).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "查询" }));

    await vi.waitFor(() => expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(2));
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      conditions: [{ field: "subject", operator: "exact", value: "" }],
      page_number: 1,
    });
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


});
