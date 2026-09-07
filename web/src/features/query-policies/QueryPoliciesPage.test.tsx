import { testIdentity, withAccountSession } from "../../test/account-session";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";
import { businessSessionInvalid } from "../../api/business-session";

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
      <MemoryRouter initialEntries={[initialEntry]}>
        <ToastProvider><AppRoutes /></ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("查询规则页面", () => {
  it("renders localized policies from the real Admin contract shape", async () => {
    vi.stubGlobal("fetch", withAccountSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies")) return json({ policies: [activePolicy] });
      throw new Error(`unexpected request ${url}`);
    })));

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
    vi.stubGlobal("fetch", withAccountSession(fetchMock));

    renderPage("/platform/query-policies/standard_page_query_v1");
    expect(await screen.findByRole("heading", { name: "查询规则详情" })).toBeVisible();
    expect(await screen.findByDisplayValue("标准分页查询")).toBeDisabled();
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/query-policies/standard_page_query_v1", expect.any(Object));
  });

  it("keeps an unknown Policy Type read-only even through a direct edit URL", async () => {
    vi.stubGlobal("fetch", withAccountSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/future_query_v1")) return json(unknownPolicy);
      if (url.endsWith("/query-policies")) return json({ policies: [unknownPolicy] });
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage("/platform/query-policies/future_query_v1?mode=edit");
    expect(await screen.findByText(/Web 尚不支持类型 future_page_query，当前仅可查看/)).toBeVisible();
    expect(await screen.findByDisplayValue("标准分页查询")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "保存" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "更新元数据" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "弃用" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "激活" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "删除" })).not.toBeInTheDocument();
  });

  it.each([
    ["ACTIVE", "edit", activePolicy],
    ["DEPRECATED", "edit", deprecatedPolicy],
    ["DRAFT", "metadata", draftPolicy],
  ])("downgrades a disallowed %s direct mode to read-only", async (_status, requestedMode, policy) => {
    vi.stubGlobal("fetch", withAccountSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith(`/query-policies/${policy.code}`)) return json(policy);
      if (url.endsWith("/query-policies")) return json({ policies: [policy] });
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage(`/platform/query-policies/${policy.code}?mode=${requestedMode}`);
    expect(await screen.findByRole("heading", { name: "查询规则详情" })).toBeVisible();
    expect(await screen.findByDisplayValue(policy.name)).toBeDisabled();
    expect(screen.queryByRole("button", { name: "保存" })).not.toBeInTheDocument();
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
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/query-policies/compact_page_query_v1");
    const activateButtons = await screen.findAllByRole("button", { name: "激活" });
    await user.click(activateButtons.at(-1)!);
    expect(screen.getByRole("alertdialog", { name: "激活查询规则？" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "确认激活" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/query-policies/compact_page_query_v1/activate",
      expect.objectContaining({ method: "POST", credentials: "same-origin", headers: expect.objectContaining({ "X-CSRF-Token": "test-session-csrf" }) }),
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
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
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
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
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

  it("does not replay a query-rule write whose response was lost and offers a read-only check", async () => {
    let writes = 0;
    let reads = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/compact_page_query_v1") && init?.method === "PUT") {
        writes += 1;
        throw new TypeError("response lost after commit");
      }
      if (url.endsWith("/query-policies/compact_page_query_v1")) { reads += 1; return json(draftPolicy); }
      if (url.endsWith("/query-policies")) { reads += 1; return json({ policies: [draftPolicy] }); }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();
    renderPage("/platform/query-policies/compact_page_query_v1?mode=edit");
    await user.clear(await screen.findByLabelText("显示名称"));
    await user.type(screen.getByLabelText("显示名称"), "可能已提交");
    await user.click(screen.getByRole("button", { name: "保存" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("提交结果尚未确认");
    expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
    expect(writes).toBe(1);
    const readsBeforeCheck = reads;
    await user.click(screen.getByRole("button", { name: "只读查询当前状态" }));
    await waitFor(() => expect(reads).toBeGreaterThan(readsBeforeCheck));
    expect(writes).toBe(1);
  });

  it("blocks a query-rule lifecycle command after its response is lost until a read-only check", async () => {
    let writes = 0;
    let reads = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/compact_page_query_v1/activate") && init?.method === "POST") {
        writes += 1;
        throw new TypeError("response lost after commit");
      }
      if (url.endsWith("/query-policies/compact_page_query_v1")) { reads += 1; return json(draftPolicy); }
      if (url.endsWith("/query-policies")) { reads += 1; return json({ policies: [draftPolicy] }); }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();
    renderPage("/platform/query-policies/compact_page_query_v1");
    await user.click((await screen.findAllByRole("button", { name: "激活" })).at(-1)!);
    await user.click(screen.getByRole("button", { name: "确认激活" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("提交结果尚未确认");
    expect(screen.queryByRole("button", { name: "激活" })).not.toBeInTheDocument();
    expect(writes).toBe(1);
    const readsBeforeCheck = reads;
    await user.click(screen.getByRole("button", { name: "只读查询当前状态" }));
    await waitFor(() => expect(reads).toBeGreaterThan(readsBeforeCheck));
    expect(writes).toBe(1);
  });

  it("keeps the mounted rule draft hidden when recovery refetch fails and reveals it after retry", async () => {
    let signedIn = true;
    let detailUnavailable = false;
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/auth/session")) return signedIn ? json(testIdentity) : json({ error: { code: "session_invalid", message: "expired", request_id: "req-session" } }, 401);
      if (url.endsWith("/auth/csrf")) return json({ csrf_token: "preauth-csrf" });
      if (url.endsWith("/auth/login")) { signedIn = true; detailUnavailable = true; return json(testIdentity); }
      if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
      if (url.endsWith("/query-policies/compact_page_query_v1")) return detailUnavailable
        ? json({ error: { code: "policy_catalog_unavailable", message: "down", request_id: "req-recovery" } }, 503)
        : json(draftPolicy);
      if (url.endsWith("/query-policies")) return json({ policies: [draftPolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage("/platform/query-policies/compact_page_query_v1?mode=edit");
    await user.clear(await screen.findByLabelText("显示名称"));
    await user.type(screen.getByLabelText("显示名称"), "重读失败仍保留");
    signedIn = false;
    act(() => window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: "session_invalid" } })));
    await user.type(await screen.findByLabelText("用户名"), "test.user");
    await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
    await user.click(screen.getByRole("button", { name: "登录" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("账号服务暂时不可用");
    expect(screen.getByLabelText("显示名称")).toHaveValue("重读失败仍保留");
    detailUnavailable = false;
    await user.click(screen.getByRole("button", { name: "重新检查登录状态" }));
    await waitFor(() => expect(screen.queryByText("账号服务暂时不可用")).not.toBeInTheDocument());
    expect(screen.getByLabelText("显示名称")).toHaveValue("重读失败仍保留");
  });
});
