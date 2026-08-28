import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";

const tablePolicy = {
  table_name: "notification_templates",
  query_policy_code: "standard_page_query_v1",
  mutation_policy_code: "standard_mutation_v1",
  enabled: true,
  creator: "local-admin",
  modifier: "local-admin",
  gmt_created: "2026-08-24T09:00:00Z",
  gmt_modified: "2026-08-24T10:30:00Z",
};

const discoveryTables = [
  {
    table_name: "notification_templates",
    table_comment: "通知模板",
    policy_exists: true,
    policy_enabled: true,
    compatible: true,
    incompatibility_reason: null,
  },
  {
    table_name: "audit_events",
    table_comment: "审计事件",
    policy_exists: false,
    policy_enabled: false,
    compatible: false,
    incompatibility_reason: "primary_key_must_be_id",
  },
  {
    table_name: "message_templates",
    table_comment: "消息模板",
    policy_exists: false,
    policy_enabled: false,
    compatible: true,
    incompatibility_reason: null,
  },
];

const queryPolicies = [
  {
    code: "strict_page_query_v2", name: "严格分页", description: "", type_code: "page_query",
    default_order_field: "id", default_order_direction: "ASC", default_page_size: 10, max_page_size: 50,
    status: "ACTIVE", creator: "admin", modifier: "admin", gmt_created: "2026-08-21T09:00:00Z", gmt_modified: "2026-08-21T09:00:00Z",
  },
  {
    code: "standard_page_query_v1", name: "标准分页", description: "", type_code: "page_query",
    default_order_field: "id", default_order_direction: "DESC", default_page_size: 20, max_page_size: 200,
    status: "ACTIVE", creator: "admin", modifier: "admin", gmt_created: "2026-08-20T09:00:00Z", gmt_modified: "2026-08-20T09:00:00Z",
  },
  {
    code: "draft_page_query_v2", name: "分页草稿", description: "", type_code: "page_query",
    default_order_field: "id", default_order_direction: "ASC", default_page_size: 10, max_page_size: 100,
    status: "DRAFT", creator: "admin", modifier: "admin", gmt_created: "2026-08-20T09:00:00Z", gmt_modified: "2026-08-20T09:00:00Z",
  },
  {
    code: "future_query_v1", name: "未来查询", description: "", type_code: "future_query",
    default_order_field: "id", default_order_direction: "ASC", default_page_size: 10, max_page_size: 50,
    status: "ACTIVE", creator: "admin", modifier: "admin", gmt_created: "2026-08-20T09:00:00Z", gmt_modified: "2026-08-20T09:00:00Z",
  },
];

const mutationPolicies = [
  {
    code: "readonly_mutation_v2", name: "只读变更", description: "", type_code: "single_table_mutation",
    allow_add: false, allow_modify: false, allow_delete: false,
    create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
    status: "ACTIVE", creator: "admin", modifier: "admin", gmt_created: "2026-08-21T09:00:00Z", gmt_modified: "2026-08-21T09:00:00Z",
  },
  {
    code: "standard_mutation_v1", name: "标准变更", description: "", type_code: "single_table_mutation",
    allow_add: true, allow_modify: true, allow_delete: false,
    create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
    status: "ACTIVE", creator: "admin", modifier: "admin", gmt_created: "2026-08-20T09:00:00Z", gmt_modified: "2026-08-20T09:00:00Z",
  },
  {
    code: "deprecated_mutation_v1", name: "旧变更", description: "", type_code: "single_table_mutation",
    allow_add: false, allow_modify: false, allow_delete: false,
    create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
    status: "DEPRECATED", creator: "admin", modifier: "admin", gmt_created: "2026-08-20T09:00:00Z", gmt_modified: "2026-08-20T09:00:00Z",
  },
  {
    code: "future_mutation_v1", name: "未来变更", description: "", type_code: "future_mutation",
    allow_add: true, allow_modify: true, allow_delete: false,
    create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
    status: "ACTIVE", creator: "admin", modifier: "admin", gmt_created: "2026-08-20T09:00:00Z", gmt_modified: "2026-08-20T09:00:00Z",
  },
];

const queryPolicyTypes = { types: [{ code: "page_query" }, { code: "future_query" }] };
const mutationPolicyTypes = { types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }, { code: "future_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] };

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
}

function renderPage(initialEntry = "/platform/table-policies") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <ToastProvider><AppRoutes /></ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("表策略分配页面", () => {
  it("通过正式路由展示真实表发现状态、Table Policy 目录并在客户端筛选", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    }));
    const user = userEvent.setup();

    renderPage();

    expect(await screen.findByRole("heading", { name: "表策略分配" })).toBeVisible();
    expect(screen.getByRole("link", { name: "表策略分配" })).toHaveClass("active");
    expect((await screen.findAllByText("notification_templates")).length).toBeGreaterThan(0);
    expect(screen.getByText("通知模板")).toBeVisible();
    expect(screen.getByText("audit_events")).toBeVisible();
    expect(screen.getByText("主键必须命名为 id")).toBeVisible();
    expect(screen.getByText("standard_page_query_v1")).toBeVisible();
    expect(screen.getByText("standard_mutation_v1")).toBeVisible();
    expect(screen.getAllByText("已启用").length).toBeGreaterThan(0);

    await user.type(screen.getByRole("searchbox", { name: "筛选表策略" }), "missing_table");
    expect(screen.queryByText("standard_page_query_v1")).not.toBeInTheDocument();
    expect(screen.getByText("没有匹配的表策略")).toBeVisible();
  });

  it("只从兼容未分配表和 Active Policy 创建 disabled 分配", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/query-policy-types")) return json(queryPolicyTypes);
      if (url.endsWith("/mutation-policy-types")) return json(mutationPolicyTypes);
      if (url.endsWith("/query-policies")) return json({ policies: queryPolicies });
      if (url.endsWith("/mutation-policies")) return json({ policies: mutationPolicies });
      if (url.endsWith("/table-policies") && init?.method === "POST") {
        return json({ ...tablePolicy, ...JSON.parse(String(init.body)), enabled: false }, 201);
      }
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    renderPage("/platform/table-policies?mode=create");

    expect(await screen.findByRole("heading", { name: "新建 Table Policy 分配" })).toBeVisible();
    const tableSelect = await screen.findByRole("combobox", { name: "真实数据库表" });
    expect(screen.getByRole("option", { name: /message_templates/ })).toBeVisible();
    expect(screen.queryByRole("option", { name: /notification_templates/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /audit_events/ })).not.toBeInTheDocument();
    expect(screen.getByRole("option", { name: /standard_page_query_v1/ })).toBeVisible();
    expect(screen.queryByRole("option", { name: /draft_page_query_v2/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /future_query_v1/ })).not.toBeInTheDocument();
    expect(screen.getByRole("option", { name: /standard_mutation_v1/ })).toBeVisible();
    expect(screen.queryByRole("option", { name: /deprecated_mutation_v1/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /future_mutation_v1/ })).not.toBeInTheDocument();

    await user.selectOptions(tableSelect, "message_templates");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active Query Policy" }), "standard_page_query_v1");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active Mutation Policy" }), "standard_mutation_v1");
    await user.click(screen.getByRole("button", { name: "创建未启用分配" }));

    const createCall = fetchMock.mock.calls.find(([url, init]) => String(url).endsWith("/table-policies") && init?.method === "POST");
    expect(JSON.parse(String(createCall?.[1]?.body))).toEqual({
      table_name: "message_templates",
      query_policy_code: "standard_page_query_v1",
      mutation_policy_code: "standard_mutation_v1",
    });
    expect(await screen.findByText("Table Policy 已创建并保持未启用")).toBeVisible();
  });

  it("从可复制详情 URL 原子替换两个 Code，并警告 enabled 分配下一次请求立即生效", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/query-policy-types")) return json(queryPolicyTypes);
      if (url.endsWith("/mutation-policy-types")) return json(mutationPolicyTypes);
      if (url.endsWith("/query-policies")) return json({ policies: queryPolicies });
      if (url.endsWith("/mutation-policies")) return json({ policies: mutationPolicies });
      if (url.endsWith("/table-policies/notification_templates") && init?.method === "PUT") {
        return json({ ...tablePolicy, ...JSON.parse(String(init.body)), enabled: true });
      }
      if (url.endsWith("/table-policies/notification_templates")) return json(tablePolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");

    expect(await screen.findByRole("heading", { name: "Table Policy 详情" })).toBeVisible();
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/table-policies/notification_templates", expect.any(Object));
    expect(await screen.findByDisplayValue("notification_templates")).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "原子替换" }));
    await user.selectOptions(await screen.findByRole("combobox", { name: "Active Query Policy" }), "strict_page_query_v2");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active Mutation Policy" }), "readonly_mutation_v2");
    await user.click(screen.getByRole("button", { name: "检查并替换" }));

    const dialog = await screen.findByRole("alertdialog", { name: "替换已启用的 Table Policy？" });
    expect(dialog).toHaveTextContent("下一次请求立即生效");
    expect(fetchMock.mock.calls.some(([url, init]) => String(url).endsWith("/notification_templates") && init?.method === "PUT")).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认原子替换" }));

    const replaceCall = fetchMock.mock.calls.find(([url, init]) => String(url).endsWith("/notification_templates") && init?.method === "PUT");
    expect(JSON.parse(String(replaceCall?.[1]?.body))).toEqual({
      table_name: "notification_templates",
      query_policy_code: "strict_page_query_v2",
      mutation_policy_code: "readonly_mutation_v2",
    });
    expect(await screen.findByText("Table Policy 已原子替换")).toBeVisible();
  });

  it("已启用分配原子替换失败后呈现稳定错误和 Request ID", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/query-policy-types")) return json(queryPolicyTypes);
      if (url.endsWith("/mutation-policy-types")) return json(mutationPolicyTypes);
      if (url.endsWith("/query-policies")) return json({ policies: queryPolicies });
      if (url.endsWith("/mutation-policies")) return json({ policies: mutationPolicies });
      if (url.endsWith("/table-policies/notification_templates") && init?.method === "PUT") {
        return json({ error: { code: "mutation_policy_not_assignable", message: "Mutation Policy cannot be assigned", request_id: "req-replace-24" } }, 422);
      }
      if (url.endsWith("/table-policies/notification_templates")) return json(tablePolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    }));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates?mode=replace");
    await user.selectOptions(await screen.findByRole("combobox", { name: "Active Query Policy" }), "strict_page_query_v2");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active Mutation Policy" }), "readonly_mutation_v2");
    await user.click(screen.getByRole("button", { name: "检查并替换" }));
    await user.click(await screen.findByRole("button", { name: "确认原子替换" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("请选择 Active 且可分配的 Mutation Policy");
    expect(screen.getByRole("alert")).toHaveTextContent("req-replace-24");
    expect(screen.queryByRole("alertdialog", { name: "替换已启用的 Table Policy？" })).not.toBeInTheDocument();
  });

  it("详情只依赖专用详情端点，不因 Policy 候选目录不可用而阻塞查看", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/table-policies/notification_templates")) return json(tablePolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`detail must not request assignment candidates: ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage("/platform/table-policies/notification_templates");

    expect(await screen.findByDisplayValue("notification_templates")).toBeVisible();
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith("/query-policies"))).toBe(false);
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith("/mutation-policies"))).toBe(false);
  });

  it("详情读取失败时呈现稳定错误并可重试专用详情端点", async () => {
    let detailAttempts = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/table-policies/notification_templates")) {
        detailAttempts += 1;
        if (detailAttempts === 1) return json({ error: { code: "table_policy_not_found", message: "Table Policy does not exist", request_id: "req-detail-24" } }, 404);
        return json(tablePolicy);
      }
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    }));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");

    expect(await screen.findByRole("alert")).toHaveTextContent("Table Policy 不存在或已被移除");
    expect(screen.getByRole("alert")).toHaveTextContent("req-detail-24");
    await user.click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByDisplayValue("notification_templates")).toBeVisible();
    expect(detailAttempts).toBe(2);
  });

  it("真实表名为 new 时仍使用可复制详情 URL，而不是打开创建页", async () => {
    const namedNewPolicy = { ...tablePolicy, table_name: "new" };
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: [] });
      if (url.endsWith("/table-policies/new")) return json(namedNewPolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [namedNewPolicy] });
      throw new Error(`unexpected request ${url}`);
    }));

    renderPage("/platform/table-policies/new");

    expect(await screen.findByRole("heading", { name: "Table Policy 详情" })).toBeVisible();
    expect(await screen.findByDisplayValue("new")).toBeVisible();
    expect(screen.queryByRole("heading", { name: "新建 Table Policy 分配" })).not.toBeInTheDocument();
  });

  it("Discovery 成功但没有真实表时展示明确空态", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: [] });
      if (url.endsWith("/table-policies")) return json({ policies: [] });
      throw new Error(`unexpected request ${url}`);
    }));

    renderPage();

    expect(await screen.findByText("没有可发现的数据库表")).toBeVisible();
  });

  it("关闭详情后进入新建会清空旧分配，不能提交已分配表", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/query-policy-types")) return json(queryPolicyTypes);
      if (url.endsWith("/mutation-policy-types")) return json(mutationPolicyTypes);
      if (url.endsWith("/query-policies")) return json({ policies: queryPolicies });
      if (url.endsWith("/mutation-policies")) return json({ policies: mutationPolicies });
      if (url.endsWith("/table-policies/notification_templates")) return json(tablePolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    }));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");
    await screen.findByDisplayValue("notification_templates");
    await user.click(screen.getAllByRole("button", { name: "关闭" }).at(-1)!);
    await user.click(screen.getByRole("button", { name: "新建分配" }));

    expect(await screen.findByRole("combobox", { name: "真实数据库表" })).toHaveValue("");
    expect(screen.getByRole("button", { name: "创建未启用分配" })).toBeDisabled();
  });

  it("确认后启用和停用 Table Policy", async () => {
    let currentPolicy = { ...tablePolicy, enabled: false };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/query-policies")) return json({ policies: queryPolicies });
      if (url.endsWith("/mutation-policies")) return json({ policies: mutationPolicies });
      if (url.endsWith("/table-policies/notification_templates/enable") && init?.method === "POST") {
        currentPolicy = { ...currentPolicy, enabled: true };
        return json(currentPolicy);
      }
      if (url.endsWith("/table-policies/notification_templates/disable") && init?.method === "POST") {
        currentPolicy = { ...currentPolicy, enabled: false };
        return json(currentPolicy);
      }
      if (url.endsWith("/table-policies/notification_templates")) return json(currentPolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [currentPolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");
    await user.click(await screen.findByRole("button", { name: "启用" }));
    expect(await screen.findByRole("alertdialog", { name: "启用 Table Policy？" })).toHaveTextContent("实时 Schema");
    await user.click(screen.getByRole("button", { name: "确认启用" }));
    expect(await screen.findByText("Table Policy 已启用")).toBeVisible();

    await user.click(await screen.findByRole("button", { name: "停用" }));
    expect(await screen.findByRole("alertdialog", { name: "停用 Table Policy？" })).toHaveTextContent("Managed Table");
    await user.click(screen.getByRole("button", { name: "确认停用" }));
    expect(await screen.findByText("Table Policy 已停用")).toBeVisible();
  });

  it("清晰呈现 Admin 的实时 Schema 稳定错误和 Request ID", async () => {
    const disabledPolicy = { ...tablePolicy, enabled: false };
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/query-policies")) return json({ policies: queryPolicies });
      if (url.endsWith("/mutation-policies")) return json({ policies: mutationPolicies });
      if (url.endsWith("/table-policies/notification_templates/enable") && init?.method === "POST") {
        return json({ error: { code: "incompatible_policy_definition", message: "Policy definition is incompatible with the live table Schema", request_id: "req-schema-24" } }, 422);
      }
      if (url.endsWith("/table-policies/notification_templates")) return json(disabledPolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [disabledPolicy] });
      throw new Error(`unexpected request ${url}`);
    }));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");
    await user.click(await screen.findByRole("button", { name: "启用" }));
    await user.click(screen.getByRole("button", { name: "确认启用" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("实时表结构");
    expect(screen.getByRole("alert")).toHaveTextContent("req-schema-24");
  });
});
