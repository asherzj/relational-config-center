import {releaseFixture,withReleaseReadRoutes} from "../../test/release-fixture";
import {rememberReleaseRequest,pendingReleaseRequests} from "../release-orders/release-journal";
import { defaultFieldPolicies } from "../../test/field-policy-fixture";
import { withDefaultRecordVersions } from "../../test/managed-data-fixture";
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
  table_name: "notification_templates",
  query_policy_code: "notification_page_query_v1",
  mutation_policy_code: "notification_full_mutation_v1",
  enabled: true,
  creator: "fixture",
  modifier: "fixture",
  created_at: "2026-08-25T09:00:00Z",
  updated_at: "2026-08-25T09:00:00Z",
};

const mutationPolicy = {
  code: "notification_full_mutation_v1",
  name: "Notification full mutation",
  description: "",
  type_code: "single_table_mutation",
  allow_add: true,
  allow_modify: false,
  allow_delete: false,
  create_operator_field: "creator",
  create_time_field: "gmt_created",
  modify_operator_field: "modifier",
  modify_time_field: "gmt_modified",
  status: "ACTIVE",
  creator: "fixture",
  modifier: "fixture",
  created_at: "2026-08-25T09:00:00Z",
  updated_at: "2026-08-25T09:00:00Z",
};

const queryPolicy = {
  code: "notification_page_query_v1",
  name: "Notification query",
  description: "",
  type_code: "page_query",
  default_order_field: "id",
  default_order_direction: "DESC",
  default_page_size: 20,
  max_page_size: 100,
  status: "ACTIVE",
  creator: "fixture",
  modifier: "fixture",
  created_at: "2026-08-25T09:00:00Z",
  updated_at: "2026-08-25T09:00:00Z",
};

const columns = [
  { name: "id", type: "uint64", nullable: false },
  { name: "template_key", type: "string", nullable: false },
  { name: "subject", type: "string", nullable: true },
  { name: "body", type: "string", nullable: false },
  { name: "creator", type: "string", nullable: false },
  { name: "gmt_created", type: "timestamp", nullable: false },
  { name: "modifier", type: "string", nullable: false },
  { name: "gmt_modified", type: "timestamp", nullable: false },
] as const;

const row = {
  id: "41",
  template_key: "welcome",
  subject: null,
  body: "",
  creator: "fixture",
  gmt_created: "2026-08-25T09:00:00Z",
  modifier: "fixture",
  gmt_modified: "2026-08-25T09:00:00Z",
};

function json(value: unknown, status = 200, requestId = "req-change-set") {
  value = releaseFixture(withDefaultRecordVersions(value));
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json", "X-Request-ID": requestId },
  });
}

function renderPage() {
  vi.stubGlobal("fetch",withReleaseReadRoutes(globalThis.fetch));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return {...render(
    <QueryClientProvider client={client}>
      <TestRouter initialEntries={["/configuration/managed-data"]}>
        <ToastProvider><AppRoutes /></ToastProvider>
      </TestRouter>
    </QueryClientProvider>,
  ),client};
}

function readFetch(input: RequestInfo | URL, init: RequestInit | undefined, policy = mutationPolicy) {
  const url = String(input);
  if (url.includes("/table-field-policies/")) return json(defaultFieldPolicies("notification_templates", columns));
  if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
  if (url.endsWith("/query-policies/notification_page_query_v1")) return json(queryPolicy);
  if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
  if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
  if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(policy);
  if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
    return json({ columns, rows: [row], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
  }
  throw new Error(`unexpected request ${url}`);
}

afterEach(() => {vi.unstubAllGlobals();sessionStorage.clear()});

describe("Managed Data draft confirmation", () => {
  it("fails closed by capability, offers optional id, and excludes every server-managed Auto Fill field from ADD", async () => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn((input: RequestInfo | URL, init?: RequestInit) => Promise.resolve(readFetch(input, init)))));
    const user = userEvent.setup();

    renderPage();

    const add = await screen.findByRole("button", { name: "新增记录" });
    await vi.waitFor(() => expect(add).toBeEnabled());
    expect(screen.getByRole("button", { name: "修改记录 41" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "删除记录 41" })).toBeDisabled();
    expect(screen.getByText("MODIFY 未由当前变更规则授权")).toBeVisible();
    expect(screen.getByText("DELETE 未由当前变更规则授权")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "查看当前表规则能力" }));
    const currentAbility = screen.getByRole("region", { name: "当前表规则能力" });
    expect(currentAbility).toHaveTextContent("按 id 降序排列");
    expect(currentAbility).toHaveTextContent("默认每页数量为 20");
    expect(currentAbility).toHaveTextContent("新增：规则允许");
    expect(currentAbility).toHaveTextContent("修改：规则禁止");
    expect(currentAbility).toHaveTextContent("本次实时表结构确认了 8 列");

    await user.click(add);
    const editor = screen.getByRole("dialog", { name: "新增 notification_templates 记录" });
    expect(within(editor).getByRole("checkbox", { name: "包含 template_key" })).toBeVisible();
    expect(within(editor).getByRole("checkbox", { name: "包含 subject" })).toBeVisible();
    expect(within(editor).getByRole("checkbox", { name: "包含 id" })).not.toBeChecked();
    expect(within(editor).queryByRole("checkbox", { name: "包含 creator" })).not.toBeInTheDocument();
    expect(within(editor).queryByRole("checkbox", { name: "包含 gmt_created" })).not.toBeInTheDocument();
    expect(within(editor).queryByRole("checkbox", { name: "包含 modifier" })).not.toBeInTheDocument();
    expect(within(editor).queryByRole("checkbox", { name: "包含 gmt_modified" })).not.toBeInTheDocument();
  });

  it("fails closed when an applicable Auto Fill target is absent from the live dynamic Schema", async () => {
    const invalidPolicy = { ...mutationPolicy, create_operator_field: "missing_creator", allow_modify: true, allow_delete: true };
    vi.stubGlobal("fetch", withAdminSession(vi.fn((input: RequestInfo | URL, init?: RequestInit) => Promise.resolve(readFetch(input, init, invalidPolicy)))));

    renderPage();

    const add = await screen.findByRole("button", { name: "新增记录" });
    await vi.waitFor(() => expect(screen.getByText("ADD Auto Fill 字段 missing_creator 不存在于实时 Schema")).toBeVisible());
    expect(add).toBeDisabled();
    expect(screen.getByRole("button", { name: "修改记录 41" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "删除记录 41" })).toBeEnabled();
  });

  it("keeps a Change Set across same-account login, refetches rules and schema, and requires confirmation again", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    let signedIn = true;
    let reads = 0;
    let writes = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
  if (url.includes("/table-field-policies/")) return json(defaultFieldPolicies("notification_templates", columns));
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/notification_page_query_v1")) return json(queryPolicy);
      if (url.endsWith("/auth/session") || url.endsWith("/auth/activity")) return signedIn ? json(testAdminIdentity) : json({ error: { code: "session_invalid", message: "expired", request_id: "req-session" } }, 401);
      if (url.endsWith("/auth/csrf")) return json({ csrf_token: "preauth-csrf" });
      if (url.endsWith("/auth/login")) { signedIn = true; return json(testAdminIdentity); }
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/release-orders") && init?.method === "POST") { writes += 1; throw new Error("draft save requires another confirmation"); }
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        reads += 1;
        return json({ columns, rows: [row], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "新增记录" }));
    await user.click(screen.getByRole("checkbox", { name: "包含 template_key" }));
    await user.type(screen.getByRole("textbox", { name: "template_key 值" }), "recover-intent");
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
    signedIn = false;
    act(() => window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: "session_invalid" } })));
    expect(screen.getByLabelText("主导航")).not.toBeVisible();
    await user.type(await screen.findByLabelText("用户名"), "test.user");
    await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
    await user.click(screen.getByRole("button", { name: "登录" }));
    await waitFor(() => expect(screen.queryByRole("heading", { name: "登录本地账号" })).not.toBeInTheDocument());
    const changeSet = screen.getByRole("dialog", { name: "ADD Change Set" });
    expect(within(changeSet).getByText("recover-intent")).toBeVisible();
    expect(within(changeSet).getByRole("button", { name: "确认并保存草稿" })).toBeEnabled();
    expect(reads).toBeGreaterThanOrEqual(2);
    expect(writes).toBe(0);
  });

  it("keeps MODIFY input but rechecks the exact current target before rebuilding its Change Set", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    const currentRow = { ...row, body: "concurrent-update" };
    let signedIn = true;
    let recovered = false;
    let exactReads = 0;
    let writes = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
  if (url.includes("/table-field-policies/")) return json(defaultFieldPolicies("notification_templates", columns));
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/notification_page_query_v1")) return json(queryPolicy);
      if (url.endsWith("/auth/session") || url.endsWith("/auth/activity")) return signedIn ? json(testAdminIdentity) : json({ error: { code: "session_invalid", message: "expired", request_id: "req-session" } }, 401);
      if (url.endsWith("/auth/csrf")) return json({ csrf_token: "preauth-csrf" });
      if (url.endsWith("/auth/login")) { signedIn = true; recovered = true; return json(testAdminIdentity); }
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/release-orders") && init?.method === "POST") { writes += 1; return json({ affected: 1 }); }
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        const exact = String(init.body).includes('"field":"id"');
        if (exact) exactReads += 1;
        return json({ columns, rows: exact ? [currentRow] : recovered ? [] : [row], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "修改记录 41" }));
    await user.type(screen.getByRole("textbox", { name: "body 值" }), "operator-intent");
    signedIn = false;
    act(() => window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: "session_invalid" } })));
    await user.type(await screen.findByLabelText("用户名"), "test.user");
    await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
    await user.click(screen.getByRole("button", { name: "登录" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "查看 Change Set" })).toBeEnabled());
    expect(screen.getByRole("textbox", { name: "body 值" })).toHaveValue("operator-intent");
    expect(exactReads).toBe(1);
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
    const bodyRow = within(screen.getByRole("dialog", { name: "MODIFY Change Set" })).getByRole("row", { name: /body/ });
    expect(within(bodyRow).getByText("concurrent-update")).toBeVisible();
    expect(within(bodyRow).getByText("operator-intent")).toBeVisible();
    expect(writes).toBe(0);
  });

  it("allows a fresh editor opened after the recovered list was already rechecked", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    let signedIn = true;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
  if (url.includes("/table-field-policies/")) return json(defaultFieldPolicies("notification_templates", columns));
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/notification_page_query_v1")) return json(queryPolicy);
      if (url.endsWith("/auth/session") || url.endsWith("/auth/activity")) return signedIn ? json(testAdminIdentity) : json({ error: { code: "session_invalid", message: "expired", request_id: "req-session" } }, 401);
      if (url.endsWith("/auth/csrf")) return json({ csrf_token: "preauth-csrf" });
      if (url.endsWith("/auth/login")) { signedIn = true; return json(testAdminIdentity); }
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") return json({ columns, rows: [row], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("button", { name: "修改记录 41" });
    signedIn = false;
    act(() => window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: "session_invalid" } })));
    await user.type(await screen.findByLabelText("用户名"), "test.user");
    await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
    await user.click(screen.getByRole("button", { name: "登录" }));
    await user.click(await screen.findByRole("button", { name: "新增记录" }));
    expect(screen.getByRole("button", { name: "查看 Change Set" })).toBeEnabled();
  });

  it.each([false, true])("keeps a recovered MODIFY Change Set gated through failed latest reads (version changed: %s)", async (versionChanged) => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    const currentRow = { ...row, body: "current-after-retry" };
    let signedIn = true;
    let recovered = false;
    let exactReads = 0;
    let writes = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
  if (url.includes("/table-field-policies/")) return json(defaultFieldPolicies("notification_templates", columns));
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/notification_page_query_v1")) return json(queryPolicy);
      if (url.endsWith("/auth/session") || url.endsWith("/auth/activity")) return signedIn ? json(testAdminIdentity) : json({ error: { code: "session_invalid", message: "expired", request_id: "req-session" } }, 401);
      if (url.endsWith("/auth/csrf")) return json({ csrf_token: "preauth-csrf" });
      if (url.endsWith("/auth/login")) { signedIn = true; recovered = true; return json(testAdminIdentity); }
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/release-orders") && init?.method === "POST") { writes += 1; return json({ affected: 1 }); }
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        const exact = String(init.body).includes('"field":"id"');
        if (exact && recovered && (++exactReads === 1 || (versionChanged && exactReads === 3))) return json({ error: { code: "query_unavailable", message: "down", request_id: "req-recheck" } }, 503);
        return json({ columns, rows: [exact && recovered ? currentRow : row], record_versions: [exact && recovered && versionChanged ? "1" : "0"], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "修改记录 41" }));
    await user.type(screen.getByRole("textbox", { name: "body 值" }), "operator-intent");
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
    // Finish the visible dialog's animation-frame focus before interrupting it.
    // jsdom otherwise allows that pending focus to target the hidden workspace.
    await waitFor(() => expect(screen.getByRole("dialog", { name: "MODIFY Change Set" })).toHaveFocus());
    signedIn = false;
    act(() => window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: "session_invalid" } })));
    await user.type(await screen.findByLabelText("用户名"), "test.user");
    await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
    expect(screen.getByLabelText("用户名")).toHaveValue("test.user");
    expect(screen.getByLabelText("密码")).toHaveValue("correct horse battery staple");
    await user.click(screen.getByRole("button", { name: "登录" }));
    await waitFor(() => expect(document.querySelector(".protected-workspace")).toHaveAttribute("data-session-status", "ready"));
    const changeSet = await screen.findByRole("dialog", { name: "MODIFY Change Set" });
    expect(await within(changeSet).findByRole("alert")).toHaveTextContent("req-recheck");
    expect(within(changeSet).getByRole("button", { name: "确认并保存草稿" })).toBeDisabled();
    expect(within(changeSet).getByRole("button", { name: "返回修改" })).toBeEnabled();
    await user.click(within(changeSet).getByRole("button", { name: "重试" }));
    if (versionChanged) {
      await within(changeSet).findByRole("button", { name: "基于最新值重建差异" });
      expect(within(changeSet).getByRole("button", { name: "确认并保存草稿" })).toBeDisabled();
      await user.click(within(changeSet).getByRole("button", { name: "查看最新值" }));
      expect(await within(changeSet).findByRole("alert")).toHaveTextContent("req-recheck");
      expect(within(changeSet).getByRole("button", { name: "确认并保存草稿" })).toBeDisabled();
      expect(within(changeSet).getByText("operator-intent")).toBeVisible();
      expect(writes).toBe(0);
      await user.click(within(changeSet).getByRole("button", { name: "重试" }));
      await user.click(await within(changeSet).findByRole("button", { name: "基于最新值重建差异" }));
    }
    await waitFor(() => expect(within(changeSet).getByRole("button", { name: "确认并保存草稿" })).toBeEnabled());
    expect(within(changeSet).getByText("current-after-retry")).toBeVisible();
    expect(within(changeSet).getByText("operator-intent")).toBeVisible();
    expect(exactReads).toBe(versionChanged ? 4 : 2);
    expect(writes).toBe(0);
  });

it("preserves a stale change, reads latest separately, and requires an explicit rebuild before retry", async () => {
  const writes: unknown[] = [];
  let latestReads = 0;
  vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
  if (url.includes("/table-field-policies/")) return json(defaultFieldPolicies("notification_templates", columns));
    if (url.endsWith("/release-orders") && init?.method === "POST") {
      writes.push(JSON.parse(String(init.body)));
      return json({ error: { code: "record_version_conflict", message: "record changed", request_id: "stale-33" } }, 409);
    }
    if (url.endsWith("/query")) {
      const exact = JSON.parse(String(init?.body)).conditions?.[0]?.field === "id";
      if (exact) latestReads++;
      return json({ columns, rows: [exact ? { ...row, body: "other editor" } : row], record_versions: [exact ? "9007199254740994" : "9007199254740993"], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
    }
    return readFetch(input, init, { ...mutationPolicy, allow_modify: true });
  })));
  const user = userEvent.setup();
  renderPage();
  const modify = await screen.findByRole("button", { name: "修改记录 41" });
  await waitFor(() => expect(modify).toBeEnabled());
  await user.click(modify);
  await user.type(screen.getByRole("textbox", { name: "body 值" }), "my pending input");
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  await user.click(screen.getByRole("button", { name: "确认并保存草稿" }));
  await screen.findByText("stale-33", { exact: false });
  expect(writes).toEqual([{title:"notification_templates 配置变更",items:[{table_name:"notification_templates",operation:"MODIFY",id:"41",content:{template_key:"welcome",subject:null,body:"my pending input"},expected_record_version:"9007199254740993"}]}]);
  expect(screen.getByRole("button", { name: "确认并保存草稿" })).toBeDisabled();
  expect(latestReads).toBe(0);
  await user.click(screen.getByRole("button", { name: "查看最新值" }));
  await screen.findByText("other editor");
  expect(screen.getByText("my pending input")).toBeVisible();
  expect(writes).toHaveLength(1);
  expect(screen.getByRole("button", { name: "确认并保存草稿" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "基于最新值重建差异" }));
  const dialog = screen.getByRole("dialog", { name: "MODIFY Change Set" });
  expect(within(dialog).getByText("other editor")).toBeVisible();
  expect(within(dialog).getByText("my pending input")).toBeVisible();
  expect(writes).toHaveLength(1);
  await user.click(screen.getByRole("button", { name: "返回修改" }));
  expect(screen.getByRole("textbox", { name: "body 值" })).toHaveValue("my pending input");
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(within(screen.getByRole("dialog", { name: "MODIFY Change Set" })).getByText("other editor")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "确认并保存草稿" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(writes[1]).toEqual({title:"notification_templates 配置变更",items:[{table_name:"notification_templates",operation:"MODIFY",id:"41",content:{template_key:"welcome",subject:null,body:"my pending input"},expected_record_version:"9007199254740994"}]});
});

it("从现有 Change Set 保存发布草稿，不调用记录写接口",async()=>{
 const writes:RequestInit[]=[];const releaseID="11111111222222223333333344444444";
 const saved={id:releaseID,title:"notification_templates 配置变更",applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-07T08:00:00Z",updated_at:"2026-09-07T08:00:00Z",history:[],allowed_actions:["edit","cancel"],items:[{table_name:"notification_templates",operation:"MODIFY",id:"41",expected_record_version:"0",content:{template_key:"welcome",subject:null,body:"draft only"},before:row,fields:[]}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input)==="/api/v1/release-orders"&&init?.method==="POST"){writes.push(init);return json(saved,201)}
  if(String(input)===`/api/v1/release-orders/${releaseID}`)return json(saved);
  if(String(input).includes("/rows"))throw new Error("draft must not write business data");
  return readFetch(input,init,{...mutationPolicy,allow_modify:true});
 })));
 const user=userEvent.setup();renderPage();
 await user.click(await screen.findByRole("button",{name:"修改记录 41"}));
 await user.type(screen.getByLabelText("body 值"),"draft only");
 await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
 await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));
 expect(await screen.findByRole("heading",{name:"notification_templates 配置变更"})).toBeVisible();
 expect(writes).toHaveLength(1);
 expect(JSON.parse(String(writes[0].body))).toEqual({title:"notification_templates 配置变更",items:[{table_name:"notification_templates",operation:"MODIFY",id:"41",expected_record_version:"0",content:{template_key:"welcome",subject:null,body:"draft only"}}]});
});
});

it.each(["ADD","MODIFY","DELETE"] as const)("%s 的唯一确认保存草稿并保留字段三态与记录版本",async(operation)=>{
 const writes:RequestInit[]=[];const savedID="aaaaaaaabbbbbbbbccccccccdddddddd";let saved:unknown;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path==="/api/v1/release-orders"&&init?.method==="POST"){
   writes.push(init);const intent=JSON.parse(String(init.body));saved={id:savedID,title:intent.title,applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-08T00:00:00Z",updated_at:"2026-09-08T00:00:00Z",history:[],allowed_actions:["edit"],items:intent.items.map((item:object)=>({...item,id:operation==="ADD"?null:"41",expected_record_version:operation==="ADD"?"":"0",before:operation==="ADD"?null:row,fields:[]}))};return json(saved,201)
  }
  if(path===`/api/v1/release-orders/${savedID}`)return json(saved);
  if(path.includes("/rows"))throw new Error("removed row API must never be called");
  return readFetch(input,init,{...mutationPolicy,allow_modify:true,allow_delete:true});
 })));
 const user=userEvent.setup();renderPage();
 if(operation==="DELETE")await user.click(await screen.findByRole("button",{name:"删除记录 41"}));
 else {
  await user.click(await screen.findByRole("button",{name:operation==="ADD"?"新增记录":"修改记录 41"}));
  if(operation==="ADD")await user.click(screen.getByLabelText("包含 body"));
  if(operation==="ADD"){await user.click(screen.getByLabelText("包含 template_key"));await user.type(screen.getByLabelText("template_key 值"),"new-key")}
  await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
 }
 const dialog=screen.getByRole("dialog",{name:`${operation} Change Set`});
 expect(within(dialog).queryByRole("button",{name:"确认并执行"})).not.toBeInTheDocument();
 expect(within(dialog).getByRole("row",{name:/body/})).toBeVisible();
 if(operation==="ADD"){
  const title=within(dialog).getByRole("textbox",{name:"发布单标题"});
  expect(title).toHaveValue("notification_templates 配置变更");
  expect(title).toBeRequired();
  await user.clear(title);
  expect(title).toHaveAttribute("aria-invalid","true");
  expect(title).toHaveAccessibleDescription(/发布单标题必填/);
  expect(within(dialog).getByText("发布单标题必填。")).toBeVisible();
  expect(within(dialog).getByRole("button",{name:"确认并保存草稿"})).toBeDisabled();
  await user.type(title,"新增消息模板");
 }
 await user.click(within(dialog).getByRole("button",{name:"确认并保存草稿"}));
 expect(await screen.findByRole("heading",{name:operation==="ADD"?"新增消息模板":"notification_templates 配置变更"})).toBeVisible();
 const item=JSON.parse(String(writes[0]!.body)).items[0];expect(writes).toHaveLength(1);expect(item.operation).toBe(operation);
 expect(item.content).toEqual(operation==="DELETE"?{}:operation==="ADD"?{body:"",template_key:"new-key"}:{template_key:"welcome",subject:null,body:""});
 if(operation!=="ADD"){expect(item.id).toBe("41");expect(item.expected_record_version).toBe("0")}
});

it("空字符串主键的DELETE仍保存准确身份和版本",async()=>{
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input)==="/api/v1/release-orders"&&init?.method==="POST") {writes.push(init);return json({error:{code:"release_invalid",message:"fixture end",request_id:"empty-id-proof"}},422)}
  if(String(input).endsWith("/tables/notification_templates/query"))return json({columns:columns.map(c=>c.name==="id"?{...c,type:"string"}:c),rows:[{...row,id:""}],record_versions:["9007199254740993"],page:{page_number:1,page_size:20,total_count:1,total_pages:1}});
  return readFetch(input,init,{...mutationPolicy,allow_delete:true});
 })));
 const user=userEvent.setup();renderPage();
 await user.click(await screen.findByRole("button",{name:/^删除记录/}));
 await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0].body)).items[0]).toEqual({table_name:"notification_templates",operation:"DELETE",id:"",expected_record_version:"9007199254740993",content:{}});
});

it("首次发布能力明确拒绝后保留输入并允许返回修改",async()=>{
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input)==="/api/v1/release-orders"&&init?.method==="POST"){
   writes.push(init);return json({error:{code:"publication_unsupported",message:"cannot publish this table",request_id:"known-release-rejection"}},422);
  }
  return readFetch(input,init);
 })));
 const user=userEvent.setup();renderPage();
 await user.click(await screen.findByRole("button",{name:"新增记录"}));
 await user.click(screen.getByLabelText("包含 template_key"));
 await user.type(screen.getByLabelText("template_key 值"),"kept-after-rejection");
 await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
 await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));
 const dialog=screen.getByRole("dialog",{name:"ADD Change Set"});
 expect(await within(dialog).findByText("cannot publish this table")).toBeVisible();
 expect(within(dialog).getByRole("button",{name:"返回修改"})).toBeEnabled();
 expect(within(dialog).queryByText("草稿保存结果待确认。原请求已保留，刷新后仍可找回。")).not.toBeInTheDocument();
 await user.click(within(dialog).getByRole("button",{name:"返回修改"}));
 expect(screen.getByLabelText("template_key 值")).toHaveValue("kept-after-rejection");
 expect(writes).toHaveLength(1);
});

it("AC-019 配置下拉草稿结果未知后仍以原正文和键恢复",async()=>{
 const releaseID="99999999aaaabbbbccccddddeeeeeeee";let attempts=0;const writes:RequestInit[]=[];
 const saved={id:releaseID,title:"notification_templates 配置变更",applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-08T00:00:00Z",updated_at:"2026-09-08T00:00:00Z",history:[],allowed_actions:["edit","cancel"],items:[{table_name:"notification_templates",operation:"ADD",id:null,expected_record_version:"",content:{template_key:"same-release-intent"},before:null,fields:[]}]};
 const metadata=defaultFieldPolicies("notification_templates",columns);
 const template=metadata.fields.find(field=>field.field_name==="template_key")!;
 template.effective={...template.effective,ui_type:"select",ui_options:{options:[{label:"原目录",value:"catalog"}]}};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).includes("/table-field-policies/"))return json(metadata);
  if(String(input)==="/api/v1/release-orders"&&init?.method==="POST"){
   writes.push(init);attempts++;
   if(attempts===1)throw new TypeError("response lost");
   if(attempts===2)return json({error:{code:"publication_unsupported",message:"known after unknown",request_id:"known-after-unknown"}},422);
   return json(saved,201);
  }
  if(String(input)===`/api/v1/release-orders/${releaseID}`)return json(saved);
  return readFetch(input,init);
 })));
 const user=userEvent.setup();renderPage();
 await user.click(await screen.findByRole("button",{name:"新增记录"}));
 await user.click(screen.getByLabelText("包含 template_key"));
 await user.selectOptions(screen.getByRole("combobox",{name:"template_key 值"}),"custom");
 await user.type(screen.getByRole("textbox",{name:"template_key 值 自定义值"}),"same-release-intent");
 await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
 await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));
 await screen.findByText("原请求已保留；再次保存将提交同一份草稿。");
 expect(screen.getByRole("button",{name:"返回修改"})).toBeEnabled();
 await user.click(screen.getByRole("button",{name:"返回修改"}));
 expect(screen.getByRole("textbox",{name:"template_key 值 自定义值"})).toHaveValue("same-release-intent");
 await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
 await user.click(await screen.findByRole("button",{name:"确认并保存草稿"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 const retry=await screen.findByRole("button",{name:"确认并保存草稿"});
 await waitFor(()=>expect(retry).toBeEnabled());
 await user.click(retry);
 expect(await screen.findByRole("heading",{name:"notification_templates 配置变更"})).toBeVisible();
 expect(writes).toHaveLength(3);
 for(const retry of writes.slice(1)){
  expect(retry.body).toBe(writes[0]!.body);
  expect(new Headers(retry.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 }
 expect(JSON.parse(String(writes[0]!.body)).title).toBe("notification_templates 配置变更");
});

it("明确勾选两行后加入本人跨表已有草稿，保留原明细和各行版本",async()=>{
 const existingID="bbbbbbbbccccccccddddddddeeeeeeee";
 const existing={id:existingID,title:"继续整理消息模板",applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"4",created_at:"2026-09-08T00:00:00Z",updated_at:"2026-09-08T00:00:00Z",history:[],allowed_actions:["edit"],items:[{table_name:"other_config",operation:"ADD",id:null,expected_record_version:"",content:{template_key:"kept",body:"kept"},before:null,fields:[]}]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path===`/api/v1/release-orders/${existingID}`){if(init?.method==="PUT")writes.push(init);return json(existing)}
  if(path.startsWith("/api/v1/release-orders?")){expect(new URL(path,"http://localhost").searchParams.has("table_name")).toBe(false);return json({orders:[{...existing,item_count:1,operation_counts:{ADD:1}}],next_cursor:""})};
  if(path.endsWith("/tables/notification_templates/query"))return json({columns,rows:[row,{...row,id:"42"}],record_versions:["7","8"],page:{page_number:1,page_size:20,total_count:2,total_pages:1}});
  return readFetch(input,init,{...mutationPolicy,allow_delete:true});
 })));
 const user=userEvent.setup();renderPage();
 await user.click(await screen.findByRole("checkbox",{name:"选择记录 41"}));
 await user.click(screen.getByRole("checkbox",{name:"选择记录 42"}));
 await user.click(screen.getByRole("button",{name:"删除已选 2 项"}));
 await user.click(screen.getByRole("button",{name:"选择已有草稿"}));
 await user.selectOptions(await screen.findByLabelText("保存到草稿"),existingID);
 await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body))).toEqual({title:"继续整理消息模板",expected_version:"4",changes:{upserts:[{table_name:"notification_templates",operation:"DELETE",id:"41",expected_record_version:"7",content:{}},{table_name:"notification_templates",operation:"DELETE",id:"42",expected_record_version:"8",content:{}}]}});
});

it("已有草稿去向只列出当前可编辑的草稿",async()=>{
 const editableID="11111111222222223333333344444444",readonlyID="55555555666666667777777788888888";
 const summary=(id:string,extra:object={})=>({notification:{sequence:"0",unread:false,pending:false},id,title:"消息模板草稿",table_names:["notification_templates"],applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-08T00:00:00Z",updated_at:"2026-09-08T00:00:00Z",allowed_actions:["edit"],item_count:1,operation_counts:{MODIFY:1},approvals:[],approval_context:{revision:"draft",tables:[],approvable_tables:[]},...extra});
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).startsWith("/api/v1/release-orders?"))return json({orders:[summary(editableID),summary(readonlyID,{allowed_actions:["submit","cancel"]})],next_cursor:""});
  return readFetch(input,init,{...mutationPolicy,allow_modify:true});
 })));
 const user=userEvent.setup();renderPage();
 await user.click(await screen.findByRole("button",{name:"修改记录 41"}));await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
 await user.click(screen.getByRole("button",{name:"选择已有草稿"}));
 expect(await screen.findByRole("option",{name:`消息模板草稿 · ${editableID} · 版本 1`})).toBeVisible();expect(screen.queryByRole("option",{name:`消息模板草稿 · ${readonlyID} · 版本 1`})).not.toBeInTheDocument();
});

it("批量选择把原型属性名当作普通字符串记录 id",async()=>{
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/tables/notification_templates/query"))return json({columns:columns.map(column=>column.name==="id"?{...column,type:"string"}:column),rows:[{...row,id:"__proto__"},{...row,id:"constructor"}],record_versions:["7","8"],page:{page_number:1,page_size:20,total_count:2,total_pages:1}});
  if(path==="/api/v1/release-orders"&&init?.method==="POST"){writes.push(init);return json({error:{code:"release_invalid",message:"retained"}},422)}
  return readFetch(input,init,{...mutationPolicy,allow_delete:true});
 })));
 const user=userEvent.setup();renderPage();
 const first=await screen.findByRole("checkbox",{name:"选择记录 __proto__"});
 const second=screen.getByRole("checkbox",{name:"选择记录 constructor"});
 expect(first).not.toBeChecked();expect(second).not.toBeChecked();
 await user.click(first);await user.click(second);
 await user.click(screen.getByRole("button",{name:"删除已选 2 项"}));
 await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body)).items).toEqual([{table_name:"notification_templates",operation:"DELETE",id:"__proto__",expected_record_version:"7",content:{}},{table_name:"notification_templates",operation:"DELETE",id:"constructor",expected_record_version:"8",content:{}}]);
});

it("批量删除核对每页20项，末页定位后仍保存全部选择",async()=>{
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/tables/notification_templates/query"))return json({columns,rows:Array.from({length:21},(_,index)=>({...row,id:String(index+1)})),record_versions:Array(21).fill("7"),page:{page_number:1,page_size:100,total_count:21,total_pages:1}});
  if(path==="/api/v1/release-orders"&&init?.method==="POST"){writes.push(init);return json({error:{code:"release_invalid",message:"retained"}},422)}
  return readFetch(input,init,{...mutationPolicy,allow_delete:true});
 })));
 const user=userEvent.setup();renderPage();
 await screen.findByRole("checkbox",{name:"选择记录 21"});
 for(let id=1;id<=21;id++)await user.click(screen.getByRole("checkbox",{name:`选择记录 ${id}`}));
 await user.click(screen.getByRole("button",{name:"删除已选 21 项"}));
 const dialog=screen.getByRole("dialog",{name:"删除所选记录"});
 expect(within(dialog).getAllByRole("listitem")).toHaveLength(20);
 await user.type(within(dialog).getByLabelText("定位待删除明细"),"21");
 expect(within(dialog).getAllByRole("listitem")).toHaveLength(1);
 expect(within(dialog).getByText("明细 21 · 记录 21 · 记录基线 7")).toBeVisible();
 await user.click(within(dialog).getByRole("button",{name:"确认并保存草稿"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body)).items.map((item:{id:string})=>item.id)).toEqual(Array.from({length:21},(_,index)=>String(index+1)));
});

it("AC-012 freezes configured query/editor input through same-account recovery and reopens with latest rules", async () => {
  const metadata = defaultFieldPolicies("notification_templates", columns);
  const template = metadata.fields.find(field => field.field_name === "template_key")!;
  template.state = "active";
  template.effective = {...template.effective,display_name:"模板",ui_type:"select",ui_options:{options:[{label:"欢迎",value:"welcome"}]},is_required:true};
  let latest = metadata;
  let metadataReads = 0;
  let signedIn = true;
  let queryReads = 0;
  let schemaChanged = false;
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/table-field-policies/")) { metadataReads++; return json(latest); }
    if (url.endsWith("/auth/session") || url.endsWith("/auth/activity")) return signedIn ? json(testAdminIdentity) : json({error:{code:"session_invalid",message:"expired"}},401);
    if (url.endsWith("/auth/csrf")) return json({csrf_token:"preauth"});
    if (url.endsWith("/auth/login")) { signedIn=true; return json(testAdminIdentity); }
    if (url.endsWith("/query")) {
      queryReads++;
      return json({columns:schemaChanged?columns.filter(column=>column.name!=="body"):columns,rows:[schemaChanged?Object.fromEntries(Object.entries(row).filter(([name])=>name!=="body")):row],page:{page_number:1,page_size:20,total_count:1,total_pages:1}});
    }
    if (url.endsWith("/query-policies")) return json({policies:[queryPolicy]});
    return readFetch(input,init,{...mutationPolicy,allow_modify:true});
  });
  vi.stubGlobal("fetch",fetchMock);
  const user=userEvent.setup(); renderPage();
  const queryInput = await screen.findByRole("combobox",{name:"筛选 模板 值"});
  await user.selectOptions(queryInput,"custom");
  const queryCustom = screen.getByRole("textbox",{name:"筛选 模板 值 自定义值"});
  await user.type(queryCustom,"query-kept");
  await user.click(screen.getByRole("button",{name:"新增记录"}));
  await user.selectOptions(await screen.findByRole("combobox",{name:"template_key 值"}),"custom");
  const editorInput = screen.getByRole("textbox",{name:"template_key 值 自定义值"});
  await user.type(editorInput,"editor-kept");
  const originalBodyInput = screen.getByRole("textbox",{name:"body 值"});
  schemaChanged = true;
  latest = structuredClone(metadata);
  latest.fields = latest.fields.filter(field=>field.field_name!=="body");
  latest.fields.find(field=>field.field_name==="template_key")!.effective = {...template.effective,display_name:"最新模板",ui_type:"textarea",ui_options:{options:[]},default_value:"latest-prefill"};
  signedIn=false;
  act(()=>window.dispatchEvent(new CustomEvent(businessSessionInvalid,{detail:{code:"session_invalid"}})));
  expect(editorInput).not.toBeVisible();
  await user.type(await screen.findByLabelText("用户名"),"test.user");
  await user.type(screen.getByLabelText("密码"),"correct horse battery staple");
  await user.click(screen.getByRole("button",{name:"登录"}));
  await waitFor(()=>expect(editorInput).toBeVisible());
  expect(screen.getByRole("textbox",{name:"template_key 值 自定义值"})).toBe(editorInput);
  expect(editorInput).toHaveValue("editor-kept");
  expect(screen.getByRole("textbox",{name:"body 值"})).toBe(originalBodyInput);
  expect(queryCustom).toHaveValue("query-kept");
  expect(metadataReads).toBe(3);
  expect(queryReads).toBeGreaterThanOrEqual(2);
  expect(fetchMock.mock.calls.filter(([url,init])=>String(url).endsWith("/release-orders")&&init?.method==="POST")).toHaveLength(0);
  await user.click(screen.getByRole("button",{name:"取消"}));
  await user.click(screen.getByRole("button",{name:"放弃修改并离开"}));
  await user.click(screen.getByRole("button",{name:"新增记录"}));
  const reopened=await screen.findByRole("textbox",{name:"template_key 值"});
  expect(reopened.tagName).toBe("TEXTAREA");
  expect(reopened).toHaveValue("latest-prefill");
  await user.click(screen.getByRole("button",{name:"取消"}));
  await user.click(screen.getByRole("link",{name:"查询规则定义"}));
  await screen.findByRole("heading",{name:"查询规则定义"});
  await user.click(screen.getByRole("link",{name:"统一变更入口"}));
  expect((await screen.findByRole("textbox",{name:"筛选 最新模板 值"})).tagName).toBe("TEXTAREA");
});

it("重新进入时等待本次字段配置读取再冻结，并且后续实时刷新不重绘查询输入",async()=>{
 const named=(name:string)=>{
  const metadata=defaultFieldPolicies("notification_templates",columns);
  const template=metadata.fields.find(field=>field.field_name==="template_key")!;
  template.state="active";
  template.effective={...template.effective,display_name:name,enabled:true};
  return metadata;
 };
 let metadataReads=0,resolveFresh:((response:Response)=>void)|undefined;
 const fetchMock=vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
  if(String(input).includes("/table-field-policies/")){
   metadataReads++;
   if(metadataReads===1)return json(named("旧模板"));
   if(metadataReads===2)return new Promise<Response>(resolve=>{resolveFresh=resolve});
   return json(named("刷新后模板"));
  }
  return readFetch(input,init);
 });
 vi.stubGlobal("fetch",withAdminSession(fetchMock));
 const user=userEvent.setup();const {client}=renderPage();
 await screen.findByRole("textbox",{name:"筛选 旧模板 值"});
 await user.click(screen.getByRole("link",{name:"查询规则定义"}));
 await screen.findByRole("heading",{name:"查询规则定义"});
 await user.click(screen.getByRole("link",{name:"统一变更入口"}));
 await waitFor(()=>expect(metadataReads).toBe(2));
 expect(screen.queryByRole("textbox",{name:"筛选 旧模板 值"})).not.toBeInTheDocument();
 expect(screen.getByText("正在读取字段查询配置…")).toBeVisible();
 resolveFresh!(json(named("新模板")));
 const input=await screen.findByRole("textbox",{name:"筛选 新模板 值"});
 await user.type(input,"kept query");
 await act(async()=>{await client.refetchQueries({queryKey:["current-field-display"]})});
 expect(screen.getByRole("textbox",{name:"筛选 新模板 值"})).toBe(input);
 expect(input).toHaveValue("kept query");
 expect(screen.queryByRole("textbox",{name:"筛选 刷新后模板 值"})).not.toBeInTheDocument();
 expect(await screen.findByRole("columnheader",{name:/刷新后模板template_key/})).toBeVisible();
});

it("列表首次成功后的配置刷新失败独立报错重试，并保留已冻结筛选输入",async()=>{
 const named=(name:string)=>{
  const metadata=defaultFieldPolicies("notification_templates",columns);
  const template=metadata.fields.find(field=>field.field_name==="template_key")!;
  template.state="active";
  template.effective={...template.effective,display_name:name,enabled:true};
  return metadata;
 };
 let metadataReads=0;
 const fetchMock=vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
  if(String(input).includes("/table-field-policies/")){
   metadataReads++;
   if(metadataReads===2)return json({error:{code:"field_policy_unavailable",message:"later display failure",request_id:"display-refresh-1"}},503,"display-refresh-1");
   return json(named(metadataReads===1?"打开时模板":"恢复后模板"));
  }
  return readFetch(input,init);
 });
 vi.stubGlobal("fetch",withAdminSession(fetchMock));
 const user=userEvent.setup();const {client}=renderPage();
 const input=await screen.findByRole("textbox",{name:"筛选 打开时模板 值"});
 await user.type(input,"retained query");
 await act(async()=>{await client.refetchQueries({queryKey:["current-field-display"]})});
 const result=screen.getByRole("region",{name:"Managed Data 查询结果"});
 const alert=await within(result).findByRole("alert");
 expect(alert).toHaveTextContent("字段配置暂时不可用");
 expect(alert).toHaveTextContent("display-refresh-1");
 expect(screen.getAllByRole("alert").filter(element=>element.textContent?.includes("字段配置暂时不可用"))).toHaveLength(1);
 expect(input).toHaveValue("retained query");
 await user.click(within(alert).getByRole("button",{name:"重试"}));
 expect(await screen.findByRole("columnheader",{name:/恢复后模板template_key/})).toBeVisible();
 expect(screen.getByRole("textbox",{name:"筛选 打开时模板 值"})).toBe(input);
 expect(input).toHaveValue("retained query");
});

it("AC-012 isolates configured query and editor drafts when the account changes", async () => {
  let identity = testAdminIdentity;
  let reads = 0;
  vi.stubGlobal("fetch",vi.fn(async(input: RequestInfo | URL,init?:RequestInit)=>{
    if(String(input).endsWith("/auth/session")||String(input).endsWith("/auth/activity")) return json(identity);
    if(String(input).includes("/table-field-policies/")){reads++;return json(defaultFieldPolicies("notification_templates",columns));}
    return readFetch(input,init);
  }));
  const user=userEvent.setup();renderPage();
  await user.type(await screen.findByRole("textbox",{name:"筛选 template_key 值"}),"alice-query");
  await user.click(screen.getByRole("button",{name:"新增记录"}));
  await user.click(await screen.findByRole("checkbox",{name:"包含 template_key"}));
  await user.type(screen.getByRole("textbox",{name:"template_key 值"}),"alice-editor");
  identity={...testAdminIdentity,account:{...testAdminIdentity.account,id:"9e5e2b50-6aaa-4eaa-83fb-f56d84240214",username:"bob"},csrf_token:"bob-csrf"};
  act(()=>window.dispatchEvent(new CustomEvent(businessSessionInvalid,{detail:{code:"account_changed"}})));
  const query=await screen.findByRole("textbox",{name:"筛选 template_key 值"});
  await waitFor(()=>expect(query).toHaveValue(""));
  expect(screen.queryByRole("dialog",{name:"新增 notification_templates 记录"})).not.toBeInTheDocument();
  await user.click(screen.getByRole("button",{name:"新增记录"}));
  expect(await screen.findByRole("textbox",{name:"template_key 值"})).toHaveValue("");
  expect(reads).toBe(4);
});

it("AC-015 independently retries field metadata without replacing already entered query input", async () => {
  let reads=0;
  vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input: RequestInfo | URL,init?:RequestInit)=>{
    if(String(input).includes("/table-field-policies/")) {
      reads++;
      return reads===1||reads===3 ? json({error:{code:"field_policy_unavailable",message:"unavailable",request_id:`metadata-${reads}`}},503) : json(defaultFieldPolicies("notification_templates",columns));
    }
    return readFetch(input,init);
  })));
  const user=userEvent.setup();renderPage();
  expect(await screen.findByRole("alert")).toHaveTextContent("metadata-1");
  expect(screen.queryByRole("textbox",{name:"筛选 template_key 值"})).not.toBeInTheDocument();
  expect(screen.getByRole("button",{name:"查询"})).toBeDisabled();
  await user.click(screen.getByRole("button",{name:"重试"}));
  const query=await screen.findByRole("textbox",{name:"筛选 template_key 值"});
  await user.type(query,"query-survives-editor-retry");
  await user.click(screen.getByRole("button",{name:"新增记录"}));
  const dialog=screen.getByRole("dialog",{name:"新增 notification_templates 记录"});
  expect(await within(dialog).findByRole("alert")).toHaveTextContent("metadata-3");
  expect(within(dialog).queryByRole("textbox",{name:"template_key 值"})).not.toBeInTheDocument();
  await user.click(within(dialog).getByRole("button",{name:"重试"}));
  await screen.findByRole("textbox",{name:"template_key 值"});
  expect(query).toHaveValue("query-survives-editor-retry");
  expect(reads).toBe(4);
});

it("AC-015 passes newly discovered real fields through the editor into Change Set", async()=>{
  vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input: RequestInfo | URL,init?:RequestInit)=>{
    if(String(input).includes("/table-field-policies/")) return json(defaultFieldPolicies("notification_templates",[...columns,{name:"new_field",type:"string",nullable:true}]));
    return readFetch(input,init);
  })));
  const user=userEvent.setup();renderPage();
  await user.click(await screen.findByRole("button",{name:"新增记录"}));
  await user.click(await screen.findByRole("checkbox",{name:"包含 new_field"}));
  await user.type(screen.getByRole("textbox",{name:"new_field 值"}),"fresh schema value");
  await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
  const changeSet=screen.getByRole("dialog",{name:"ADD Change Set"});
  expect(within(changeSet).getByText("new_field",{exact:true})).toBeVisible();
  expect(within(changeSet).getByText("fresh schema value",{exact:true})).toBeVisible();
});

it("AC-015/019 shows fresh MODIFY defaults in Change Set while retaining the original record version", async()=>{
  const freshColumns=[...columns,{name:"new_default",type:"string" as const,nullable:true},{name:"new_null",type:"string" as const,nullable:true}];
  const freshRow={...row,new_default:"database default",new_null:null};
  const writes: unknown[]=[];
  vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input: RequestInfo | URL,init?:RequestInit)=>{
    const url=String(input);
    if(url.includes("/table-field-policies/"))return json(defaultFieldPolicies("notification_templates",freshColumns));
    if(url.endsWith("/query")&&JSON.parse(String(init?.body)).conditions?.[0]?.field==="id")return json({columns:freshColumns,rows:[freshRow],record_versions:["1"],page:{page_number:1,page_size:1,total_count:1,total_pages:1}});
    if(url.endsWith("/release-orders")&&init?.method==="POST"){
      writes.push(JSON.parse(String(init.body)));
      return json({error:{code:"record_version_conflict",message:"stale original version",request_id:"drift-conflict"}},409);
    }
    return readFetch(input,init,{...mutationPolicy,allow_modify:true});
  })));
  const user=userEvent.setup();renderPage();
  const modify=await screen.findByRole("button",{name:"修改记录 41"});
  await waitFor(()=>expect(modify).toBeEnabled());await user.click(modify);
  expect(await screen.findByRole("textbox",{name:"new_default 值"})).toHaveValue("database default");
  await user.type(screen.getByRole("textbox",{name:"body 值"}),"my change");
  await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
  const review=screen.getByRole("dialog",{name:"MODIFY Change Set"});
  expect(within(review).getAllByText("database default")).toHaveLength(2);
  await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));
  await screen.findByText("drift-conflict",{exact:false});
  expect(writes).toEqual([{title:"notification_templates 配置变更",items:[{table_name:"notification_templates",operation:"MODIFY",id:"41",expected_record_version:"0",content:{template_key:"welcome",subject:null,body:"my change",new_default:"database default",new_null:null}}]}]);
  expect(screen.getByRole("button",{name:"确认并保存草稿"})).toBeDisabled();
  expect(within(review).getByText("my change")).toBeVisible();
});


it.each(["single","batch"] as const)("另一窗口原请求重推后保留配置页%s未保存输入",async(kind)=>{
 const releaseID="88888888aaaabbbbccccddddeeeeeeee",writes:RequestInit[]=[];
 const body=JSON.stringify({title:"other window",items:[{table_name:"notification_templates",operation:"ADD",content:{template_key:"other-value"}}]});
 const saved={id:releaseID,title:"other window",applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-08T00:00:00Z",updated_at:"2026-09-08T00:00:00Z",history:[],allowed_actions:["edit"],items:[{table_name:"notification_templates",operation:"ADD",id:null,expected_record_version:"",content:{template_key:"other-value"},before:null,fields:[]}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input)==="/api/v1/release-orders"&&init?.method==="POST"){writes.push(init);return json(saved,201)}
  if(String(input)===`/api/v1/release-orders/${releaseID}`)return json(saved);
  return readFetch(input,init,{...mutationPolicy,allow_delete:true});
 })));
 const user=userEvent.setup();renderPage();
 if(kind==="single"){
  await user.click(await screen.findByRole("button",{name:"新增记录"}));await user.click(screen.getByLabelText("包含 template_key"));await user.type(screen.getByLabelText("template_key 值"),"this-window-value");await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
 }else{await user.click(await screen.findByRole("checkbox",{name:"选择记录 41"}));await user.click(screen.getByRole("button",{name:"删除已选 1 项"}));}
 await act(async()=>{await rememberReleaseRequest(testAdminIdentity.account.id,{scope:"create",path:"/api/v1/release-orders",method:"POST",body,key:"other-create-key",label:"另一窗口原申请"})});
 await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));await waitFor(()=>expect(writes).toHaveLength(1));
 await waitFor(()=>expect(pendingReleaseRequests(testAdminIdentity.account.id)).toHaveLength(0));
 expect(writes[0]!.body).toBe(body);expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBe("other-create-key");
 if(kind==="single"){
  expect(screen.getByRole("dialog",{name:"ADD Change Set"})).toBeVisible();await user.click(screen.getByRole("button",{name:"返回修改"}));expect(screen.getByLabelText("template_key 值")).toHaveValue("this-window-value");
 }else{expect(screen.getByRole("dialog",{name:"删除所选记录"})).toBeVisible();await user.click(screen.getByRole("button",{name:"取消删除"}));expect(screen.getByRole("checkbox",{name:"选择记录 41"})).toBeChecked();}
});


it.each(["single","batch"] as const)("配置页%s未知结果后可以退出且不清除原请求",async(kind)=>{
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input)==="/api/v1/release-orders"&&init?.method==="POST"){writes.push(init);throw new TypeError("response lost")}
  return readFetch(input,init,{...mutationPolicy,allow_delete:true});
 })));
 const user=userEvent.setup();renderPage();
 if(kind==="single"){
  await user.click(await screen.findByRole("button",{name:"新增记录"}));await user.click(screen.getByLabelText("包含 template_key"));await user.type(screen.getByLabelText("template_key 值"),"kept-after-close");await user.click(screen.getByRole("button",{name:"查看 Change Set"}));
 }else{await user.click(await screen.findByRole("checkbox",{name:"选择记录 41"}));await user.click(screen.getByRole("button",{name:"删除已选 1 项"}));}
 await user.click(screen.getByRole("button",{name:"确认并保存草稿"}));await screen.findByText("Admin 连接或响应传输中断。");
 expect(screen.getByRole("button",{name:kind==="single"?"放弃本次编辑":"取消删除"})).toBeEnabled();
 await user.keyboard("{Escape}");await user.click(await screen.findByRole("button",{name:"放弃修改并离开"}));
 await waitFor(()=>expect(screen.queryByRole("dialog")).not.toBeInTheDocument());expect(writes).toHaveLength(1);
 const retained=pendingReleaseRequests(testAdminIdentity.account.id);expect(retained).toHaveLength(1);expect(retained[0].body).toBe(writes[0]!.body);expect(retained[0].key).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});
