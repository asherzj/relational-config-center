import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";

const activePolicy = {
  code: "standard_page_query_v1",
  name: "标准分页查询",
  description: "适用于多数配置表的默认分页规则",
  type_code: "page_query",
  default_order_field: "id",
  default_order_direction: "DESC",
  default_page_size: 20,
  max_page_size: 200,
  status: "ACTIVE",
  creator: "admin",
  modifier: "admin",
  gmt_created: "2026-08-22T09:12:08Z",
  gmt_modified: "2026-08-23T14:26:11Z",
};

const draftPolicy = {
  ...activePolicy,
  code: "compact_page_query_v1",
  name: "紧凑分页查询",
  status: "DRAFT",
  modifier: "local-admin",
};

const deprecatedPolicy = {
  ...activePolicy,
  code: "legacy_page_query_v1",
  name: "旧版分页查询",
  status: "DEPRECATED",
};

const unknownPolicy = {
  ...activePolicy,
  code: "future_query_v1",
  type_code: "future_page_query",
};

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
}

function renderPage(initialEntry = "/platform/query-policies") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TestRouter initialEntries={[initialEntry]}>
        <ToastProvider><AppRoutes /></ToastProvider>
      </TestRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("查询规则页面", () => {
  it("renders localized policies from the real Admin contract shape", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies")) return json({ policies: [activePolicy] });
      throw new Error(`unexpected request ${url}`);
    }));

    renderPage();
    expect(await screen.findByRole("heading", { name: "查询规则定义" })).toBeVisible();
    expect(await screen.findByText("standard_page_query_v1")).toBeVisible();
    expect(screen.getByText("已激活")).toBeVisible();
    expect(screen.queryByText("Query Policy 定义")).not.toBeInTheDocument();
  });

  it("loads details from the dedicated detail endpoint", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/standard_page_query_v1")) return json(activePolicy);
      if (url.endsWith("/query-policies")) return json({ policies: [activePolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    renderPage("/platform/query-policies/standard_page_query_v1");
    expect(await screen.findByRole("heading", { name: "查询规则详情" })).toBeVisible();
    expect(await screen.findByDisplayValue("标准分页查询")).toBeDisabled();
    expect(screen.getByRole("region", { name: "实际查询效果" })).toHaveTextContent("按 id 降序排列");
    expect(screen.getByRole("region", { name: "实际查询效果" })).toHaveTextContent("默认每页数量为 20");
    expect(screen.getByRole("region", { name: "实际查询效果" })).toHaveTextContent("不能超过 200");
    expect(screen.getByRole("region", { name: "实际查询效果" })).toHaveTextContent("没有配置字段白名单");
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/query-policies/standard_page_query_v1", expect.any(Object));
  });

  it("blocks execution editing for an unknown Type but still offers safe metadata editing", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/future_query_v1")) return json(unknownPolicy);
      if (url.endsWith("/query-policies")) return json({ policies: [unknownPolicy] });
      throw new Error(`unexpected request ${url}`);
    }));

    renderPage("/platform/query-policies/future_query_v1?mode=edit");
    expect(await screen.findByText("仅可修改名称和描述")).toBeVisible();
    expect(screen.queryByText("仅可查看")).not.toBeInTheDocument();
    expect(await screen.findByText(/无法确认执行规则，仍可安全查看或修改名称和描述/)).toBeVisible();
    expect(await screen.findByDisplayValue("标准分页查询")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "保存执行规则" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "修改名称和描述" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "弃用" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "激活" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "删除" })).not.toBeInTheDocument();
  });

  it("keeps a known Active row's hint consistent when the Type registry is unavailable", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ error: { code: "unavailable", message: "down" } }, 503);
      if (url.endsWith("/query-policies")) return json({ policies: [activePolicy] });
      throw new Error(`unexpected request ${url}`);
    }));

    renderPage();
    expect(await screen.findByText("仅可修改名称和描述")).toBeVisible();
    expect(screen.getByRole("button", { name: "名称和描述" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "弃用" })).not.toBeInTheDocument();
  });

  it("allows an unknown active Type to update only its name and description", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/future_query_v1")) return json(unknownPolicy);
      if (url.endsWith("/query-policies")) return json({ policies: [unknownPolicy] });
      throw new Error(`unexpected request ${url}`);
    }));

    renderPage("/platform/query-policies/future_query_v1?mode=metadata");
    expect(await screen.findByDisplayValue("标准分页查询")).toBeEnabled();
    expect(screen.getByDisplayValue("id")).toBeDisabled();
    expect(screen.getByRole("button", { name: "保存名称和描述" })).toBeVisible();
  });

  it.each([
    ["ACTIVE", "edit", activePolicy],
    ["DEPRECATED", "edit", deprecatedPolicy],
    ["DRAFT", "metadata", draftPolicy],
  ])("downgrades a disallowed %s direct mode to read-only", async (_status, requestedMode, policy) => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith(`/query-policies/${policy.code}`)) return json(policy);
      if (url.endsWith("/query-policies")) return json({ policies: [policy] });
      throw new Error(`unexpected request ${url}`);
    }));

    renderPage(`/platform/query-policies/${policy.code}?mode=${requestedMode}`);
    expect(await screen.findByRole("heading", { name: "查询规则详情" })).toBeVisible();
    expect(await screen.findByDisplayValue(policy.name)).toBeDisabled();
    expect(screen.queryByRole("button", { name: /保存/ })).not.toBeInTheDocument();
  });

  it("executes lifecycle commands only after confirmation", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/compact_page_query_v1/activate") && init?.method === "POST") {
        return json({ ...draftPolicy, status: "ACTIVE" });
      }
      if (url.endsWith("/query-policies/compact_page_query_v1")) return json(draftPolicy);
      if (url.endsWith("/query-policies")) return json({ policies: [draftPolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    renderPage("/platform/query-policies/compact_page_query_v1");
    const activateButtons = await screen.findAllByRole("button", { name: "激活" });
    await user.click(activateButtons.at(-1)!);
    expect(screen.getByRole("alertdialog", { name: "激活查询规则？" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "确认激活" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/query-policies/compact_page_query_v1/activate",
      expect.objectContaining({ method: "POST" }),
    ));
    expect(await screen.findByText("查询规则已激活")).toBeVisible();
  });

  it("deprecates an active Policy only after confirmation", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/standard_page_query_v1/deprecate") && init?.method === "POST") return json({ ...activePolicy, status: "DEPRECATED" });
      if (url.endsWith("/query-policies/standard_page_query_v1")) return json(activePolicy);
      if (url.endsWith("/query-policies")) return json({ policies: [activePolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    renderPage("/platform/query-policies/standard_page_query_v1");
    await user.click((await screen.findAllByRole("button", { name: "弃用" })).at(-1)!);
    expect(screen.getByRole("alertdialog", { name: "弃用查询规则？" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "确认弃用" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/query-policies/standard_page_query_v1/deprecate",
      expect.objectContaining({ method: "POST" }),
    ));
  });

  it("cancels and then confirms Draft deletion", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/compact_page_query_v1") && init?.method === "DELETE") return new Response(null, { status: 204 });
      if (url.endsWith("/query-policies/compact_page_query_v1")) return json(draftPolicy);
      if (url.endsWith("/query-policies")) return json({ policies: [draftPolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();

    renderPage("/platform/query-policies/compact_page_query_v1");
    await user.click((await screen.findAllByRole("button", { name: "删除" })).at(-1)!);
    expect(screen.getByRole("alertdialog", { name: "删除查询规则草稿？" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(fetchMock.mock.calls.some(([url, init]) => String(url).endsWith("compact_page_query_v1") && (init as RequestInit | undefined)?.method === "DELETE")).toBe(false);

    await user.click((await screen.findAllByRole("button", { name: "删除" })).at(-1)!);
    await user.click(screen.getByRole("button", { name: "确认删除" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/query-policies/compact_page_query_v1",
      expect.objectContaining({ method: "DELETE" }),
    ));
  });
});


it("ends an uncertain lifecycle check in the refreshed directory instead of leaving stale detail actions", async () => {
  const user = userEvent.setup();
  let storedPolicy = { ...draftPolicy };
  let writes = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
    if (url.endsWith(`/query-policies/${draftPolicy.code}/activate`) && init?.method === "POST") {
      writes++; storedPolicy = { ...storedPolicy, status: "ACTIVE" };
      throw new TypeError("response lost after activation");
    }
    if (url.endsWith(`/query-policies/${draftPolicy.code}`)) return json(storedPolicy);
    if (url.endsWith("/query-policies")) return json({ policies: [storedPolicy] });
    throw new Error(`unexpected request ${url}`);
  }));
  renderPage(`/platform/query-policies/${draftPolicy.code}`);
  const drawer = await screen.findByRole("dialog", { name: "查询规则详情" });
  await user.click(await within(drawer).findByRole("button", { name: "激活" }));
  await user.click(screen.getByRole("button", { name: "确认激活" }));
  await screen.findByRole("alert", { name: "提交结果尚未确认" });
  await user.click(screen.getByRole("button", { name: "只读核对当前状态" }));
  await screen.findByText("ACTIVE");
  await user.click(screen.getByRole("button", { name: "我已核对，结束本次核对" }));
  await waitFor(() => expect(screen.queryByRole("dialog", { name: "查询规则详情" })).not.toBeInTheDocument());
  expect(await screen.findByText("已激活")).toBeVisible();
  expect(writes).toBe(1);
});
