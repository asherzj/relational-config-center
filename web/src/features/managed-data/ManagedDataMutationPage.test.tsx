import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";

const tablePolicy = {
  table_name: "notification_templates",
  query_policy_code: "notification_page_query_v1",
  mutation_policy_code: "notification_full_mutation_v1",
  enabled: true,
  creator: "fixture",
  modifier: "fixture",
  gmt_created: "2026-08-25T09:00:00Z",
  gmt_modified: "2026-08-25T09:00:00Z",
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
  gmt_created: "2026-08-25T09:00:00Z",
  gmt_modified: "2026-08-25T09:00:00Z",
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
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json", "X-Request-ID": requestId },
  });
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TestRouter initialEntries={["/configuration/managed-data"]}>
        <ToastProvider><AppRoutes /></ToastProvider>
      </TestRouter>
    </QueryClientProvider>,
  );
}

function readFetch(input: RequestInfo | URL, init: RequestInit | undefined, policy = mutationPolicy) {
  const url = String(input);
  if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
  if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
  if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(policy);
  if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
    return json({ columns, rows: [row], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
  }
  throw new Error(`unexpected request ${url}`);
}

afterEach(() => vi.unstubAllGlobals());

describe("Managed Data mutation capability", () => {
  it("fails closed by capability and excludes id plus every server-managed Auto Fill field from ADD", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => Promise.resolve(readFetch(input, init))));
    const user = userEvent.setup();

    renderPage();

    const add = await screen.findByRole("button", { name: "新增记录" });
    await vi.waitFor(() => expect(add).toBeEnabled());
    expect(screen.getByRole("button", { name: "修改记录 41" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "删除记录 41" })).toBeDisabled();
    expect(screen.getByText("MODIFY 未由当前变更规则授权")).toBeVisible();
    expect(screen.getByText("DELETE 未由当前变更规则授权")).toBeVisible();

    await user.click(add);
    const editor = screen.getByRole("dialog", { name: "新增 notification_templates 记录" });
    expect(within(editor).getByRole("checkbox", { name: "包含 template_key" })).toBeVisible();
    expect(within(editor).getByRole("checkbox", { name: "包含 subject" })).toBeVisible();
    expect(within(editor).queryByRole("checkbox", { name: "包含 id" })).not.toBeInTheDocument();
    expect(within(editor).queryByRole("checkbox", { name: "包含 creator" })).not.toBeInTheDocument();
    expect(within(editor).queryByRole("checkbox", { name: "包含 gmt_created" })).not.toBeInTheDocument();
    expect(within(editor).queryByRole("checkbox", { name: "包含 modifier" })).not.toBeInTheDocument();
    expect(within(editor).queryByRole("checkbox", { name: "包含 gmt_modified" })).not.toBeInTheDocument();
  });

  it("fails closed when an applicable Auto Fill target is absent from the live dynamic Schema", async () => {
    const invalidPolicy = { ...mutationPolicy, create_operator_field: "missing_creator", allow_modify: true, allow_delete: true };
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => Promise.resolve(readFetch(input, init, invalidPolicy))));

    renderPage();

    const add = await screen.findByRole("button", { name: "新增记录" });
    await vi.waitFor(() => expect(screen.getByText("ADD Auto Fill 字段 missing_creator 不存在于实时 Schema")).toBeVisible());
    expect(add).toBeDisabled();
    expect(screen.getByRole("button", { name: "修改记录 41" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "删除记录 41" })).toBeEnabled();
  });

  it("ADD uses the shared all-field Change Set once, then exact-id refetches the database row", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    const createdColumns = [...columns, { name: "server_default", type: "string", nullable: false }] as const;
    const created = {
      ...row,
      id: "42",
      template_key: "new-template",
      subject: null,
      body: "",
      creator: "server-operator",
      modifier: "server-operator",
      server_default: "filled-after-write",
    };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/tables/notification_templates/rows") && init?.method === "POST") return json({ id: "42" }, 201);
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        const spec = JSON.parse(String(init.body));
        return json({
          columns: spec.conditions?.[0]?.field === "id" ? createdColumns : columns,
          rows: spec.conditions?.[0]?.field === "id" ? [created] : [row],
          page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 },
        });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();

    const add = await screen.findByRole("button", { name: "新增记录" });
    await vi.waitFor(() => expect(add).toBeEnabled());
    await user.click(add);
    await user.click(screen.getByRole("checkbox", { name: "包含 template_key" }));
    await user.type(screen.getByRole("textbox", { name: "template_key 值" }), "new-template");
    await user.click(screen.getByRole("checkbox", { name: "包含 subject" }));
    await user.click(screen.getByRole("checkbox", { name: "subject 使用 NULL" }));
    await user.click(screen.getByRole("checkbox", { name: "包含 body" }));
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));

    const changeSet = screen.getByRole("dialog", { name: "ADD Change Set" });
    expect(within(changeSet).getAllByRole("row")).toHaveLength(columns.length + 1);
    const idRow = within(changeSet).getByRole("row", { name: /id/ });
    expect(within(idRow).getByText("不存在")).toHaveClass("cell-missing");
    expect(within(idRow).getByText("未提交")).toHaveClass("cell-unsubmitted");
    const subjectRow = within(changeSet).getByRole("row", { name: /subject/ });
    expect(within(subjectRow).getByText("NULL")).toBeVisible();
    const bodyRow = within(changeSet).getByRole("row", { name: /body/ });
    expect(within(bodyRow).getByText('""')).toBeVisible();
    expect(within(changeSet).getAllByText("Auto Fill").length).toBeGreaterThan(0);
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(1);

    await user.click(within(changeSet).getByRole("button", { name: "确认并执行" }));

    expect(await screen.findByRole("heading", { name: "ADD 已完成" })).toBeVisible();
    expect(screen.getAllByText("server-operator")).toHaveLength(2);
    expect(screen.getByText("server_default")).toBeVisible();
    expect(screen.getByText("filled-after-write")).toBeVisible();
    const writeCall = fetchMock.mock.calls.find(([url]) => String(url).endsWith("/rows"));
    expect(JSON.parse(String(writeCall?.[1]?.body))).toEqual({ content: { template_key: "new-template", subject: null, body: "" } });
    const exactQuery = fetchMock.mock.calls.find(([, init]) => init?.method === "POST" && String(init.body).includes('"field":"id"'));
    expect(JSON.parse(String(exactQuery?.[1]?.body))).toEqual({ conditions: [{ field: "id", operator: "exact", value: "42" }], page_number: 1, page_size: 1 });
  });

  it("MODIFY highlights only changed fields, preserves raw typed values, and exact-id refetches", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    const richColumns = [
      ...columns.slice(0, 4),
      { name: "price", type: "decimal", nullable: false },
      { name: "scheduled_on", type: "date", nullable: false },
      { name: "metadata", type: "json", nullable: true },
      ...columns.slice(4),
    ] as const;
    const richRow = { ...row, price: "10.2300", scheduled_on: "2026-08-26", metadata: "{\"enabled\":true}" };
    const modified = { ...richRow, body: "changed", modifier: "server-operator" };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/tables/notification_templates/rows/41") && init?.method === "PATCH") return json({ affected: 1 });
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        const exact = String(init.body).includes('"field":"id"');
        return json({ columns: richColumns, rows: [exact ? modified : richRow], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();

    const modify = await screen.findByRole("button", { name: "修改记录 41" });
    await vi.waitFor(() => expect(modify).toBeEnabled());
    await user.click(modify);
    await user.click(screen.getByRole("checkbox", { name: "包含 body" }));
    await user.clear(screen.getByRole("textbox", { name: "body 值" }));
    await user.type(screen.getByRole("textbox", { name: "body 值" }), "changed");
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));

    const changeSet = screen.getByRole("dialog", { name: "MODIFY Change Set" });
    expect(within(changeSet).getAllByRole("row")).toHaveLength(richColumns.length + 1);
    const bodyRow = within(changeSet).getByRole("row", { name: /body/ });
    expect(within(bodyRow).getByText('""').closest("td")).toHaveClass("change-original");
    expect(within(bodyRow).getByText("changed").closest("td")).toHaveClass("change-next");
    const priceRow = within(changeSet).getByRole("row", { name: /price/ });
    expect(within(priceRow).getAllByText("10.2300")).toHaveLength(2);
    expect(within(priceRow).getAllByRole("cell")[0]).not.toHaveClass("change-original");
    expect(within(changeSet).getAllByText("2026-08-26")).toHaveLength(2);
    expect(within(changeSet).getAllByText('{"enabled":true}')).toHaveLength(2);
    const creatorRow = within(changeSet).getByRole("row", { name: /^creator/ });
    expect(within(creatorRow).getAllByText("fixture")).toHaveLength(2);
    expect(within(creatorRow).queryByText("未提交")).not.toBeInTheDocument();
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(1);

    await user.click(within(changeSet).getByRole("button", { name: "确认并执行" }));

    expect(await screen.findByRole("heading", { name: "MODIFY 已完成" })).toBeVisible();
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST" && String(init.body).includes('"field":"id"'))).toBe(true);
    const patchCall = fetchMock.mock.calls.find(([, init]) => init?.method === "PATCH");
    expect(JSON.parse(String(patchCall?.[1]?.body))).toEqual({ content: { body: "changed" } });
  });

  it("DELETE shows every original value struck out against a missing record and finishes with a summary", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    let deleted = false;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/tables/notification_templates/rows/41") && init?.method === "DELETE") { deleted = true; return json({ affected: 1 }); }
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        return json({ columns, rows: deleted ? [] : [row], page: { page_number: 1, page_size: 20, total_count: deleted ? 0 : 1, total_pages: deleted ? 0 : 1 } });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();

    const remove = await screen.findByRole("button", { name: "删除记录 41" });
    await vi.waitFor(() => expect(remove).toBeEnabled());
    await user.click(remove);
    const changeSet = screen.getByRole("dialog", { name: "DELETE Change Set" });
    const rows = within(changeSet).getAllByRole("row").slice(1);
    expect(rows).toHaveLength(columns.length);
    for (const changeRow of rows) {
      expect(within(changeRow).getAllByRole("cell")[0]).toHaveClass("change-original");
      expect(within(changeRow).getByText("不存在").closest("td")).toHaveClass("change-next");
    }
    expect(fetchMock.mock.calls.filter(([url]) => String(url).endsWith("/query"))).toHaveLength(1);

    await user.click(within(changeSet).getByRole("button", { name: "确认并执行" }));

    expect(await screen.findByRole("heading", { name: "DELETE 已完成" })).toBeVisible();
    expect(screen.getByText(/已删除记录 id/)).toHaveTextContent("41");
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "DELETE")).toHaveLength(1);
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST" && String(init.body).includes('"field":"id"'))).toBe(false);
  });

  it("preserves an empty JSON String row id when executing DELETE", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    const emptyIDRow = { ...row, id: "" };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/tables/notification_templates/rows/") && init?.method === "DELETE") return json({ affected: 1 });
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        return json({ columns, rows: [emptyIDRow], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();

    const remove = await screen.findByRole("button", { name: /删除记录/ });
    await vi.waitFor(() => expect(remove).toBeEnabled());
    await user.click(remove);
    await user.click(screen.getByRole("button", { name: "确认并执行" }));

    expect(await screen.findByRole("heading", { name: "DELETE 已完成" })).toBeVisible();
    expect(fetchMock.mock.calls.filter(([url, init]) => String(url).endsWith("/rows/") && init?.method === "DELETE")).toHaveLength(1);
  });

  it("keeps the Change Set and editor values after a stable Admin rejection with Request ID", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/tables/notification_templates/rows") && init?.method === "POST") {
        return json({ error: { code: "duplicate_key", message: "duplicate", request_id: "req-duplicate-26" } }, 409, "req-duplicate-26");
      }
      if (url.endsWith("/tables/notification_templates/query")) return json({ columns, rows: [row], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();

    const add = await screen.findByRole("button", { name: "新增记录" });
    await vi.waitFor(() => expect(add).toBeEnabled());
    await user.click(add);
    await user.click(screen.getByRole("checkbox", { name: "包含 template_key" }));
    await user.type(screen.getByRole("textbox", { name: "template_key 值" }), "duplicate-template");
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
    const changeSet = screen.getByRole("dialog", { name: "ADD Change Set" });
    await user.click(within(changeSet).getByRole("button", { name: "确认并执行" }));

    const alert = await within(changeSet).findByRole("alert");
    expect(alert).toHaveTextContent("唯一键已存在，请修改字段值后重试");
    expect(alert).toHaveTextContent("req-duplicate-26");
    expect(within(changeSet).getByText("duplicate-template")).toBeVisible();
    expect(within(changeSet).getByRole("button", { name: "确认并执行" })).toBeEnabled();
    await user.click(within(changeSet).getByRole("button", { name: "返回修改" }));
    expect(screen.getByRole("textbox", { name: "template_key 值" })).toHaveValue("duplicate-template");
    expect(screen.getByRole("checkbox", { name: "包含 template_key" })).toBeChecked();
  });

  it("pins the table identity and never repeats a completed ADD when exact-id readback fails", async () => {
    const fullPolicy = { ...mutationPolicy, allow_modify: true, allow_delete: true };
    const secondPolicy = { ...tablePolicy, table_name: "audit_events" };
    let exactAttempts = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/table-policies")) return json({ policies: [tablePolicy, secondPolicy] });
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/notification_full_mutation_v1")) return json(fullPolicy);
      if (url.endsWith("/tables/notification_templates/rows") && init?.method === "POST") return json({ id: "42" }, 201);
      if (url.endsWith("/tables/notification_templates/query") && init?.method === "POST") {
        if (String(init.body).includes('"field":"id"')) {
          exactAttempts += 1;
          if (exactAttempts === 1) return json({ error: { code: "query_unavailable", message: "unavailable", request_id: "req-readback-26" } }, 503, "req-readback-26");
          return json({ columns, rows: [{ ...row, id: "42", template_key: "readback" }], page: { page_number: 1, page_size: 1, total_count: 1, total_pages: 1 } });
        }
        return json({ columns, rows: [row], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
      }
      if (url.endsWith("/tables/audit_events/query") && init?.method === "POST") {
        return json({ columns, rows: [], page: { page_number: 1, page_size: 20, total_count: 0, total_pages: 0 } });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage();

    const add = await screen.findByRole("button", { name: "新增记录" });
    await vi.waitFor(() => expect(add).toBeEnabled());
    await user.click(add);
    await user.click(screen.getByRole("checkbox", { name: "包含 template_key" }));
    await user.type(screen.getByRole("textbox", { name: "template_key 值" }), "readback");
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Managed Table" }), "audit_events");
    await user.click(screen.getByRole("button", { name: "继续编辑" }));
    expect(screen.getByRole("combobox", { name: "Managed Table" })).toHaveValue("notification_templates");
    await user.click(screen.getByRole("button", { name: "确认并执行" }));

    expect(await screen.findByRole("heading", { name: "ADD 已执行，回查未完成" })).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent("req-readback-26");
    expect(screen.queryByRole("button", { name: "确认并执行" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重新回查" }));

    expect(await screen.findByRole("heading", { name: "ADD 已完成" })).toBeVisible();
    expect(fetchMock.mock.calls.filter(([url, init]) => String(url).endsWith("/rows") && init?.method === "POST")).toHaveLength(1);
    expect(exactAttempts).toBe(2);
  });
});
