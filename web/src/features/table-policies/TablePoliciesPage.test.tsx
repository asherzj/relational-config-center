import { testAdminIdentity, withAdminSession } from "../../test/account-session";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";
import { businessSessionInvalid } from "../../api/business-session";

const tablePolicy = {
  version:"1",
  table_name: "notification_templates",
  query_policy_code: "standard_page_query_v1",
  mutation_policy_code: "standard_mutation_v1",
  enabled: true,
  creator: "local-admin",
  modifier: "local-admin",
  created_at: "2026-08-24T09:00:00Z",
  updated_at: "2026-08-24T10:30:00Z",
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
    status: "ACTIVE", creator: "admin", modifier: "admin", created_at: "2026-08-21T09:00:00Z", updated_at: "2026-08-21T09:00:00Z",
  },
  {
    code: "standard_page_query_v1", name: "标准分页", description: "", type_code: "page_query",
    default_order_field: "id", default_order_direction: "DESC", default_page_size: 20, max_page_size: 200,
    status: "ACTIVE", creator: "admin", modifier: "admin", created_at: "2026-08-20T09:00:00Z", updated_at: "2026-08-20T09:00:00Z",
  },
  {
    code: "draft_page_query_v2", name: "分页草稿", description: "", type_code: "page_query",
    default_order_field: "id", default_order_direction: "ASC", default_page_size: 10, max_page_size: 100,
    status: "DRAFT", creator: "admin", modifier: "admin", created_at: "2026-08-20T09:00:00Z", updated_at: "2026-08-20T09:00:00Z",
  },
  {
    code: "future_query_v1", name: "未来查询", description: "", type_code: "future_query",
    default_order_field: "id", default_order_direction: "ASC", default_page_size: 10, max_page_size: 50,
    status: "ACTIVE", creator: "admin", modifier: "admin", created_at: "2026-08-20T09:00:00Z", updated_at: "2026-08-20T09:00:00Z",
  },
];

const mutationPolicies = [
  {
    code: "readonly_mutation_v2", name: "只读变更", description: "", type_code: "single_table_mutation",
    allow_add: false, allow_modify: false, allow_delete: false,
    create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
    status: "ACTIVE", creator: "admin", modifier: "admin", created_at: "2026-08-21T09:00:00Z", updated_at: "2026-08-21T09:00:00Z",
  },
  {
    code: "standard_mutation_v1", name: "标准变更", description: "", type_code: "single_table_mutation",
    allow_add: true, allow_modify: true, allow_delete: false,
    create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
    status: "ACTIVE", creator: "admin", modifier: "admin", created_at: "2026-08-20T09:00:00Z", updated_at: "2026-08-20T09:00:00Z",
  },
  {
    code: "deprecated_mutation_v1", name: "旧变更", description: "", type_code: "single_table_mutation",
    allow_add: false, allow_modify: false, allow_delete: false,
    create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
    status: "DEPRECATED", creator: "admin", modifier: "admin", created_at: "2026-08-20T09:00:00Z", updated_at: "2026-08-20T09:00:00Z",
  },
  {
    code: "future_mutation_v1", name: "未来变更", description: "", type_code: "future_mutation",
    allow_add: true, allow_modify: true, allow_delete: false,
    create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
    status: "ACTIVE", creator: "admin", modifier: "admin", created_at: "2026-08-20T09:00:00Z", updated_at: "2026-08-20T09:00:00Z",
  },
];

const queryPolicyTypes = { types: [{ code: "page_query" }, { code: "future_query" }] };
const mutationPolicyTypes = { types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }, { code: "future_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] };

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
}

function renderPage(initialEntry = "/platform/table-policies") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, retryDelay: 0 }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TestRouter initialEntries={[initialEntry]}>
        <ToastProvider><AppRoutes /></ToastProvider>
      </TestRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("表规则分配页面", () => {
  it("通过正式路由展示真实表发现状态、表规则目录并在客户端筛选", async () => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    })));
    const user = userEvent.setup();

    renderPage();

    expect(await screen.findByRole("heading", { name: "表规则分配" })).toBeVisible();
    expect(screen.getByRole("link", { name: "表规则分配" })).toHaveClass("active");
    expect((await screen.findAllByText("notification_templates")).length).toBeGreaterThan(0);
    expect(screen.getByText("通知模板")).toBeVisible();
    expect(screen.getByText("audit_events")).toBeVisible();
    expect(screen.getByText("主键必须命名为 id")).toBeVisible();
    expect(screen.getByText("standard_page_query_v1")).toBeVisible();
    expect(screen.getByText("standard_mutation_v1")).toBeVisible();
    expect(screen.getAllByText("已启用").length).toBeGreaterThan(0);

    await user.type(screen.getByRole("searchbox", { name: "筛选表规则" }), "missing_table");
    expect(screen.queryByText("standard_page_query_v1")).not.toBeInTheDocument();
    expect(screen.getByText("没有匹配的表规则")).toBeVisible();
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
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/table-policies?mode=create");

    expect(await screen.findByRole("heading", { name: "新建表规则分配" })).toBeVisible();
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
    await user.selectOptions(screen.getByRole("combobox", { name: "Active 查询规则" }), "standard_page_query_v1");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active 变更规则" }), "standard_mutation_v1");
    const preview = screen.getByRole("region", { name: "所选规则效果预览" });
    expect(preview).toHaveTextContent("创建后保持未启用");
    expect(preview).toHaveTextContent("按 id 降序排列");
    expect(preview).toHaveTextContent("新增：规则允许");
    expect(preview).toHaveTextContent("删除：规则禁止");
    await user.click(screen.getByRole("button", { name: "创建未启用分配" }));

    const createCall = fetchMock.mock.calls.find(([url, init]) => String(url).endsWith("/table-policies") && init?.method === "POST");
    expect(JSON.parse(String(createCall?.[1]?.body))).toEqual({
      table_name: "message_templates",
      query_policy_code: "standard_page_query_v1",
      mutation_policy_code: "standard_mutation_v1",
      concurrency_key: [],
    });
    expect(await screen.findByText("表规则已创建并保持未启用")).toBeVisible();
  });

  it.each(["create", "disable"])("manually replays the original %s request after response loss", async kind => {
    const writes:RequestInit[]=[];
    vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
      const url=String(input);
      if(init?.method==="POST"&&url.includes("/table-policies")){
        writes.push(init);if(writes.length===1)throw new TypeError("response lost after commit");
        return json({...tablePolicy,table_name:kind==="create"?"message_templates":"notification_templates",enabled:false,version:kind==="create"?"1":"2"},kind==="create"?201:200);
      }
      if(url.endsWith("/database-tables"))return json({tables:discoveryTables});
      if(url.endsWith("/query-policy-types"))return json(queryPolicyTypes);
      if(url.endsWith("/mutation-policy-types"))return json(mutationPolicyTypes);
      if(url.endsWith("/query-policies"))return json({policies:queryPolicies});
      if(url.endsWith("/mutation-policies"))return json({policies:mutationPolicies});
      if(url.endsWith("/table-policies"))return json({policies:[tablePolicy]});
      if(url.endsWith("/table-policies/message_templates"))return json({...tablePolicy,table_name:"message_templates",enabled:false});
      if(url.endsWith("/table-policies/notification_templates"))return json(tablePolicy);
      throw new Error(url);
    })));
    const user=userEvent.setup();renderPage(kind==="create"?"/platform/table-policies?mode=create":"/platform/table-policies/notification_templates");
    if(kind==="create"){
      await user.selectOptions(await screen.findByRole("combobox",{name:"真实数据库表"}),"message_templates");
      await user.selectOptions(screen.getByRole("combobox",{name:"Active 查询规则"}),"standard_page_query_v1");
      await user.selectOptions(screen.getByRole("combobox",{name:"Active 变更规则"}),"standard_mutation_v1");
      await user.click(screen.getByRole("button",{name:"创建未启用分配"}));
    }else{await user.click(await screen.findByRole("button",{name:"停用"}));await user.click(screen.getByRole("button",{name:"确认停用"}));}
    await screen.findByText(/操作结果未知/);expect(writes).toHaveLength(1);
    expect(screen.queryByRole("button",{name:"只读核对当前状态"})).not.toBeInTheDocument();
    if(kind==="create")expect(screen.getByRole("combobox",{name:"真实数据库表"})).toBeDisabled();
    await user.click(screen.getByRole("button",{name:"重推原请求"}));
    await waitFor(()=>expect(writes).toHaveLength(2));
    expect(writes[1]?.body).toEqual(writes[0]?.body);
    expect(new Headers(writes[1]?.headers).get("Idempotency-Key")).toEqual(new Headers(writes[0]?.headers).get("Idempotency-Key"));
    await screen.findByText(kind==="create"?"表规则已创建并保持未启用":"表规则已停用");
  });

  it("recovers a state-command version conflict without treating cached data as a successful refresh", async()=>{
    const writes:RequestInit[]=[];let reads=0;let readFails=false;let slowRead=false;let releaseRead:(()=>void)|undefined;
    vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
      const url=String(input);
      if(init?.method==="POST"&&url.endsWith("/disable")){writes.push(init);if(writes.length===1)return json({error:{code:"dependency_unavailable",message:"down",request_id:"down"}},503);if(writes.length===2)return json({error:{code:"table_policy_conflict",message:"changed",request_id:"write-conflict"}},409);return json({...tablePolicy,enabled:false,version:"4"});}
      if(url.endsWith("/notification_templates")){reads++;if(slowRead)return new Promise<Response>(resolve=>{releaseRead=()=>{slowRead=false;resolve(json({...tablePolicy,version:"3"}));};});return readFails?json({error:{code:"invalid_request",message:"read failed",request_id:"read-failure"}},422):json({...tablePolicy,version:reads===1?"1":"3"});}
      if(url.endsWith("/database-tables"))return json({tables:discoveryTables});
      if(url.endsWith("/query-policy-types"))return json(queryPolicyTypes);
      if(url.endsWith("/mutation-policy-types"))return json(mutationPolicyTypes);
      if(url.endsWith("/query-policies"))return json({policies:queryPolicies});
      if(url.endsWith("/mutation-policies"))return json({policies:mutationPolicies});
      if(url.endsWith("/table-policies"))return json({policies:[tablePolicy]});throw new Error(url);
    })));
    renderPage("/platform/table-policies/notification_templates");await userEvent.click(await screen.findByRole("button",{name:"停用"}));await userEvent.click(screen.getByRole("button",{name:"确认停用"}));await screen.findByText(/操作结果未知/);
    await userEvent.click(screen.getByRole("button",{name:"重推原请求"}));await screen.findByText(/表规则已被其他管理员修改/);expect(screen.queryByRole("button",{name:"重推原请求"})).not.toBeInTheDocument();
    readFails=true;await userEvent.click(screen.getByRole("button",{name:"读取最新表规则并保留输入"}));await screen.findByText("请求编号：read-failure");expect(screen.getByText(/表规则已被其他管理员修改/)).toBeVisible();
    readFails=false;slowRead=true;await userEvent.click(screen.getByRole("button",{name:"读取最新表规则并保留输入"}));expect(screen.getByRole("button",{name:"停用"})).toBeDisabled();await userEvent.click(screen.getByRole("button",{name:"停用"}));expect(writes).toHaveLength(2);releaseRead?.();await waitFor(()=>expect(screen.queryByText(/表规则已被其他管理员修改/)).not.toBeInTheDocument());
    await userEvent.click(screen.getByRole("button",{name:"停用"}));await userEvent.click(screen.getByRole("button",{name:"确认停用"}));await waitFor(()=>expect(writes).toHaveLength(3));expect(JSON.parse(String(writes[2]?.body)).expected_version).toBe("3");expect(new Headers(writes[2]?.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]?.headers).get("Idempotency-Key"));
    await screen.findByText("表规则已停用");await waitFor(()=>expect(screen.getByRole("button",{name:"停用"})).toBeEnabled());
  });

  it("keeps a table-rule assignment across same-account login and revalidates its candidates", async () => {
    let signedIn = true;
    let discoveryReads = 0;
    let writes = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/auth/session")) return signedIn ? json(testAdminIdentity) : json({ error: { code: "session_invalid", message: "expired", request_id: "req-session" } }, 401);
      if (url.endsWith("/auth/csrf")) return json({ csrf_token: "preauth-csrf" });
      if (url.endsWith("/auth/login")) { signedIn = true; return json(testAdminIdentity); }
      if (url.endsWith("/database-tables")) { discoveryReads += 1; return json({ tables: discoveryTables }); }
      if (url.endsWith("/query-policy-types")) return json(queryPolicyTypes);
      if (url.endsWith("/mutation-policy-types")) return json(mutationPolicyTypes);
      if (url.endsWith("/query-policies")) return json({ policies: queryPolicies });
      if (url.endsWith("/mutation-policies")) return json({ policies: mutationPolicies });
      if (url.endsWith("/table-policies") && init?.method === "POST") { writes += 1; return json(tablePolicy, 201); }
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage("/platform/table-policies?mode=create");
    await user.selectOptions(await screen.findByRole("combobox", { name: "真实数据库表" }), "message_templates");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active 查询规则" }), "standard_page_query_v1");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active 变更规则" }), "standard_mutation_v1");
    signedIn = false;
    act(() => window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: "session_invalid" } })));
    await user.type(await screen.findByLabelText("用户名"), "test.user");
    await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
    await user.click(screen.getByRole("button", { name: "登录" }));
    await waitFor(() => expect(screen.queryByRole("heading", { name: "登录本地账号" })).not.toBeInTheDocument());
 await waitFor(()=>expect(screen.getByRole("combobox",{name:"真实数据库表"})).toBeVisible());
    expect(screen.getByRole("combobox", { name: "真实数据库表" })).toHaveValue("message_templates");
    expect(screen.getByRole("combobox", { name: "Active 查询规则" })).toHaveValue("standard_page_query_v1");
    expect(screen.getByRole("combobox", { name: "Active 变更规则" })).toHaveValue("standard_mutation_v1");
    expect(discoveryReads).toBeGreaterThanOrEqual(2);
    expect(writes).toBe(0);
  });

  it("从可复制详情 URL 一起替换两个 Code，并警告 enabled 分配下一次请求立即生效", async () => {
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
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");

    expect(await screen.findByRole("heading", { name: "表规则详情" })).toBeVisible();
    expect(await screen.findByDisplayValue("notification_templates")).toBeDisabled();
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/table-policies/notification_templates", expect.any(Object));
    await user.click(screen.getByRole("button", { name: "替换所选规则" }));
    await user.selectOptions(await screen.findByRole("combobox", { name: "Active 查询规则" }), "strict_page_query_v2");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active 变更规则" }), "readonly_mutation_v2");
    await user.click(screen.getByRole("button", { name: "检查并替换" }));

    const dialog = await screen.findByRole("alertdialog", { name: "替换已启用的表规则？" });
    expect(dialog).toHaveTextContent("下一次请求立即生效");
    expect(fetchMock.mock.calls.some(([url, init]) => String(url).endsWith("/notification_templates") && init?.method === "PUT")).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认替换" }));

    const replaceCall = fetchMock.mock.calls.find(([url, init]) => String(url).endsWith("/notification_templates") && init?.method === "PUT");
    expect(JSON.parse(String(replaceCall?.[1]?.body))).toEqual({
      table_name: "notification_templates",
      query_policy_code: "strict_page_query_v2",
      mutation_policy_code: "readonly_mutation_v2",
      concurrency_key: [],
      expected_version: "1",
    });
    expect(await screen.findByText("表规则已替换")).toBeVisible();
  });

  it("已启用分配一起替换失败后呈现稳定错误和 Request ID", async () => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
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
    })));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates?mode=replace");
    await user.selectOptions(await screen.findByRole("combobox", { name: "Active 查询规则" }), "strict_page_query_v2");
    await user.selectOptions(screen.getByRole("combobox", { name: "Active 变更规则" }), "readonly_mutation_v2");
    await user.click(screen.getByRole("button", { name: "检查并替换" }));
    await user.click(await screen.findByRole("button", { name: "确认替换" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("请选择 Active 且可分配的变更规则");
    expect(screen.getByRole("alert")).toHaveTextContent("req-replace-24");
    expect(screen.queryByRole("alertdialog", { name: "替换已启用的表规则？" })).not.toBeInTheDocument();
  });

  it("规则目录不可用时仍显示表规则详情，并明确无法确认规则效果", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/query-policy-types") || url.endsWith("/mutation-policy-types") || url.endsWith("/query-policies") || url.endsWith("/mutation-policies")) return json({ error: { code: "unavailable", message: "down" } }, 503);
      if (url.endsWith("/table-policies/notification_templates")) return json(tablePolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));

    renderPage("/platform/table-policies/notification_templates");

    expect(await screen.findByDisplayValue("notification_templates")).toBeVisible();
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith("/query-policies"))).toBe(true);
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith("/mutation-policies"))).toBe(true);
    expect(await screen.findByText("无法确认查询效果")).toBeVisible();
    expect(screen.getByRole("region", { name: "当前已选规则效果" })).toHaveTextContent("无法确认变更效果");
  });

  it("详情读取失败时呈现稳定错误并可重试专用详情端点", async () => {
    let detailAttempts = 0;
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/table-policies/notification_templates")) {
        detailAttempts += 1;
        if (detailAttempts === 1) return json({ error: { code: "table_policy_not_found", message: "Table Policy does not exist", request_id: "req-detail-24" } }, 404);
        return json(tablePolicy);
      }
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    })));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");

    expect(await screen.findByRole("alert")).toHaveTextContent("表规则不存在或已被移除");
    expect(screen.getByRole("alert")).toHaveTextContent("req-detail-24");
    await user.click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByDisplayValue("notification_templates")).toBeVisible();
    expect(detailAttempts).toBe(2);
  });

  it("真实表名为 new 时仍使用可复制详情 URL，而不是打开创建页", async () => {
    const namedNewPolicy = { ...tablePolicy, table_name: "new" };
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: [] });
      if (url.endsWith("/table-policies/new")) return json(namedNewPolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [namedNewPolicy] });
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage("/platform/table-policies/new");

    expect(await screen.findByRole("heading", { name: "表规则详情" })).toBeVisible();
    expect(await screen.findByDisplayValue("new")).toBeVisible();
    expect(screen.queryByRole("heading", { name: "新建表规则分配" })).not.toBeInTheDocument();
  });

  it("Discovery 成功但没有真实表时展示明确空态", async () => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: [] });
      if (url.endsWith("/table-policies")) return json({ policies: [] });
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage();

    expect(await screen.findByText("没有可发现的数据库表")).toBeVisible();
  });

  it("关闭详情后进入新建会清空旧分配，不能提交已分配表", async () => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/database-tables")) return json({ tables: discoveryTables });
      if (url.endsWith("/query-policy-types")) return json(queryPolicyTypes);
      if (url.endsWith("/mutation-policy-types")) return json(mutationPolicyTypes);
      if (url.endsWith("/query-policies")) return json({ policies: queryPolicies });
      if (url.endsWith("/mutation-policies")) return json({ policies: mutationPolicies });
      if (url.endsWith("/table-policies/notification_templates")) return json(tablePolicy);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      throw new Error(`unexpected request ${url}`);
    })));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");
    await screen.findByDisplayValue("notification_templates");
    await user.click(screen.getAllByRole("button", { name: "关闭" }).at(-1)!);
    await user.click(screen.getByRole("button", { name: "新建分配" }));

    expect(await screen.findByRole("combobox", { name: "真实数据库表" })).toHaveValue("");
    expect(screen.getByRole("button", { name: "创建未启用分配" })).toBeDisabled();
  });

  it("确认后启用和停用表规则", async () => {
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
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");
    await user.click(await screen.findByRole("button", { name: "启用" }));
    expect(await screen.findByRole("alertdialog", { name: "启用表规则？" })).toHaveTextContent("实时 Schema");
    await user.click(screen.getByRole("button", { name: "确认启用" }));
    expect(await screen.findByText("表规则已启用")).toBeVisible();

    await user.click(await screen.findByRole("button", { name: "停用" }));
    expect(await screen.findByRole("alertdialog", { name: "停用表规则？" })).toHaveTextContent("Managed Table");
    await user.click(screen.getByRole("button", { name: "确认停用" }));
    expect(await screen.findByText("表规则已停用")).toBeVisible();
  });

  it("清晰呈现 Admin 的实时 Schema 稳定错误和 Request ID", async () => {
    const disabledPolicy = { ...tablePolicy, enabled: false };
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
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
    })));
    const user = userEvent.setup();

    renderPage("/platform/table-policies/notification_templates");
    await user.click(await screen.findByRole("button", { name: "启用" }));
    await user.click(screen.getByRole("button", { name: "确认启用" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("实时表结构");
    expect(screen.getByRole("alert")).toHaveTextContent("req-schema-24");
  });
});
