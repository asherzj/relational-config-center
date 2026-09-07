import { testIdentity, withAccountSession } from "../../test/account-session";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";
import { businessSessionInvalid } from "../../api/business-session";

const activePolicy = {
  code: "standard_mutation_v1",
  name: "标准单表变更",
  description: "允许新增和修改，并维护标准审计字段",
  type_code: "single_table_mutation",
  allow_add: true,
  allow_modify: true,
  allow_delete: false,
  create_operator_field: "creator",
  create_time_field: "created_at",
  modify_operator_field: "modifier",
  modify_time_field: "updated_at",
  status: "ACTIVE",
  creator: "admin",
  modifier: "admin",
  gmt_created: "2026-08-22T09:12:08Z",
  gmt_modified: "2026-08-23T14:26:11Z",
};

const draftPolicy = {
  ...activePolicy,
  code: "editable_mutation_v2",
  name: "可编辑变更草稿",
  status: "DRAFT",
};

const unknownPolicy = {
  ...activePolicy,
  code: "future_mutation_v1",
  type_code: "future_mutation",
};

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
}

function renderPage(initialEntry = "/platform/mutation-policies") {
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

describe("变更规则页面", () => {
  it("在空目录中使用变更规则的领域文案", async () => {
    vi.stubGlobal("fetch", withAccountSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) {
        return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      }
      if (url.endsWith("/mutation-policies")) return json({ policies: [] });
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage();
    expect(await screen.findByText("还没有变更规则")).toBeVisible();
    expect(screen.queryByText("还没有查询规则")).not.toBeInTheDocument();
  });

  it("is a formal route that renders Type operations and the real catalog", async () => {
    vi.stubGlobal("fetch", withAccountSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) {
        return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      }
      if (url.endsWith("/mutation-policies")) return json({ policies: [activePolicy] });
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage();
    expect(await screen.findByRole("heading", { name: "变更规则定义" })).toBeVisible();
    expect(await screen.findByRole("link", { name: "变更规则定义" })).toHaveClass("active");
    await waitFor(() => expect(screen.getAllByText("single_table_mutation")).toHaveLength(2));
    expect(screen.getByText("ADD · MODIFY · DELETE")).toBeVisible();
    expect(await screen.findByText("standard_mutation_v1")).toBeVisible();
    expect(screen.getByText("已激活")).toBeVisible();
  });

  it("loads a copyable detail URL from the dedicated endpoint", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/standard_mutation_v1")) return json(activePolicy);
      if (url.endsWith("/mutation-policies")) return json({ policies: [activePolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));

    renderPage("/platform/mutation-policies/standard_mutation_v1");
    expect(await screen.findByRole("heading", { name: "变更规则详情" })).toBeVisible();
    expect(await screen.findByDisplayValue("标准单表变更")).toBeDisabled();
    expect(screen.getByDisplayValue("creator")).toBeDisabled();
    expect(screen.getByDisplayValue("updated_at")).toBeDisabled();
    const effect = screen.getByRole("region", { name: "实际变更效果" });
    expect(effect).toHaveTextContent("新增：规则允许");
    expect(effect).toHaveTextContent("creator（当前账号的永久 Account ID）");
    expect(effect).toHaveTextContent("修改：规则允许");
    expect(effect).toHaveTextContent("删除：规则禁止");
    expect(effect).toHaveTextContent("操作人字段填写当前登录账号的永久 Account ID；历史值保持原样。");
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/mutation-policies/standard_mutation_v1", expect.any(Object));
  });

  it("creates a Draft with all capabilities and nullable Auto Fill fields in the Admin DTO", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies") && init?.method === "POST") {
        return json({ ...draftPolicy, ...JSON.parse(String(init.body)), status: "DRAFT" }, 201);
      }
      if (url.endsWith("/mutation-policies")) return json({ policies: [] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/mutation-policies/new");
    expect(await screen.findByRole("heading", { name: "新建变更规则草稿" })).toBeVisible();
    const createButton = screen.getByRole("button", { name: "创建草稿" });
    await waitFor(() => expect(createButton).toBeEnabled());
    expect(screen.getByRole("region", { name: "执行效果预览" })).toBeVisible();
    await user.type(screen.getByLabelText(/规则编码/), "audit_mutation_v2");
    await user.type(screen.getByLabelText("显示名称"), "审计字段变更");
    await user.click(screen.getByRole("checkbox", { name: /ADD/ }));
    await user.click(screen.getByRole("checkbox", { name: /MODIFY/ }));
    await user.type(screen.getByLabelText("新增时填写 Operator 的列"), "creator");
    await user.type(screen.getByLabelText("新增和修改时填写更新时间的列"), "updated_at");
    await user.click(createButton);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/mutation-policies",
      expect.objectContaining({ method: "POST" }),
    ));
    const createCall = fetchMock.mock.calls.find(([url, init]) => String(url).endsWith("/mutation-policies") && init?.method === "POST");
    expect(JSON.parse(String(createCall?.[1]?.body))).toEqual({
      code: "audit_mutation_v2",
      name: "审计字段变更",
      description: "",
      type_code: "single_table_mutation",
      allow_add: true,
      allow_modify: true,
      allow_delete: false,
      create_operator_field: "creator",
      create_time_field: null,
      modify_operator_field: null,
      modify_time_field: "updated_at",
    });
  });

  it("rejects unsafe, primary-key, duplicate and permission-dependent Auto Fill targets before fetch", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies")) return json({ policies: [] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/mutation-policies/new");
    await user.type(await screen.findByLabelText(/规则编码/), "invalid_mutation_v1");
    await user.type(screen.getByLabelText("显示名称"), "非法草稿");
    await user.type(screen.getByLabelText("新增时填写 Operator 的列"), "id");
    await user.type(screen.getByLabelText("新增时填写创建时间的列"), "unsafe;field");
    await user.type(screen.getByLabelText("新增和修改时填写 Operator 的列"), "duplicate_target");
    await user.type(screen.getByLabelText("新增和修改时填写更新时间的列"), "duplicate_target");
    const createButton = screen.getByRole("button", { name: "创建草稿" });
    await waitFor(() => expect(createButton).toBeEnabled());
    await user.click(createButton);

    expect(await screen.findByText(/id 是主键/)).toBeVisible();
    expect(screen.getByText(/请输入安全的字段名/)).toBeVisible();
    expect(screen.getAllByText(/Auto Fill 目标不能重复/)).toHaveLength(2);
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false);
  });

  it("does not replay a mutation-rule write whose response was lost and offers a read-only check", async () => {
    let writes = 0;
    let reads = 0;
    let failedCheckStatus = 0;
    let failedReads = 0;
    let finishCheck: ((response: Response) => void) | undefined;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/editable_mutation_v2") && init?.method === "PUT") {
        writes += 1;
        throw new TypeError("response lost after commit");
      }
      if (url.endsWith("/mutation-policies/editable_mutation_v2")) {
        reads += 1;
        if (failedCheckStatus) {
          failedReads += 1;
          if (!finishCheck) return new Promise<Response>((resolve) => { finishCheck = resolve; });
          return json({ error: { code: "policy_catalog_unavailable", message: "read unavailable", request_id: "req-check" } }, failedCheckStatus);
        }
        return json(draftPolicy);
      }
      if (url.endsWith("/mutation-policies")) { reads += 1; return json({ policies: [draftPolicy] }); }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();
    renderPage("/platform/mutation-policies/editable_mutation_v2?mode=edit");
    await user.clear(await screen.findByLabelText("显示名称"));
    await user.type(screen.getByLabelText("显示名称"), "可能已提交");
    await user.click(screen.getByRole("button", { name: "保存执行规则" }));
    expect(await screen.findByLabelText("提交结果尚未确认")).toHaveTextContent("提交结果尚未确认");
    expect(screen.getByRole("button", { name: "保存执行规则" })).toBeDisabled();
    expect(writes).toBe(1);
    for (const status of [503, 504]) {
      failedCheckStatus = status;
      failedReads = 0;
      finishCheck = undefined;
      await user.click(screen.getByRole("button", { name: "只读核对当前状态" }));
      await waitFor(() => expect(finishCheck).toBeTypeOf("function"));
      await act(async () => { finishCheck!(json({ error: { code: "policy_catalog_unavailable", message: "read unavailable", request_id: "req-check" } }, status)); });
      await waitFor(() => expect(failedReads).toBe(1));
      expect(await screen.findByLabelText("提交结果尚未确认")).toHaveTextContent("提交结果尚未确认");
      expect(screen.getByRole("button", { name: "保存执行规则" })).toBeDisabled();
      expect(writes).toBe(1);
    }
    // Closing requires explicit discard, but reopening must retain the write
    // outcome block even though the editing session itself is newly keyed.
    await user.click(screen.getByRole("button", { name: "关闭抽屉" }));
    await user.click(screen.getByRole("button", { name: "放弃修改并离开" }));
    await user.click(screen.getByRole("button", { name: "修改执行规则" }));
    expect(await screen.findByRole("button", { name: "保存执行规则" })).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("提交结果尚未确认");
    expect(writes).toBe(1);
    failedCheckStatus = 0;
    const readsBeforeCheck = reads;
    await user.click(screen.getByRole("button", { name: "只读核对当前状态" }));
    await waitFor(() => expect(reads).toBeGreaterThan(readsBeforeCheck));
    expect(await screen.findByText("当前查询结果（仅供核对）")).toBeVisible();
    expect(screen.getByRole("button", { name: "保存执行规则" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "我已核对，返回修改" }));
    await waitFor(() => expect(screen.queryByLabelText("提交结果尚未确认")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "保存执行规则" })).toBeEnabled();
    expect(writes).toBe(1);
  });

  it("blocks a mutation-rule lifecycle command after its response is lost until a read-only check", async () => {
    let writes = 0;
    let reads = 0;
    let failedCheckStatus = 0;
    let failedReads = 0;
    let finishCheck: ((response: Response) => void) | undefined;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/editable_mutation_v2/activate") && init?.method === "POST") {
        writes += 1;
        return json({ error: { code: "invalid_policy_transition", message: "activation refused", request_id: "req-conflict" } }, 409);
      }
      if (url.endsWith("/mutation-policies/editable_mutation_v2") && init?.method === "DELETE") {
        writes += 1;
        throw new TypeError("response lost after commit");
      }
      if (url.endsWith("/mutation-policies/editable_mutation_v2")) {
        reads += 1;
        if (failedCheckStatus) {
          failedReads += 1;
          if (!finishCheck) return new Promise<Response>((resolve) => { finishCheck = resolve; });
          return json({ error: { code: "policy_catalog_unavailable", message: "read unavailable", request_id: "req-check" } }, failedCheckStatus);
        }
        return json(draftPolicy);
      }
      if (url.endsWith("/mutation-policies")) {
        reads += 1;
        return json({ policies: [draftPolicy] });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();
    renderPage("/platform/mutation-policies/editable_mutation_v2");
    await user.click((await screen.findAllByRole("button", { name: "激活" })).at(-1)!);
    await user.click(screen.getByRole("button", { name: "确认激活" }));
    expect(await screen.findByText(/当前状态不允许/)).toBeVisible();
    await user.click(screen.getAllByRole("button", { name: "删除" }).at(-1)!);
    await user.click(screen.getByRole("button", { name: "确认删除" }));
    expect(await screen.findByLabelText("提交结果尚未确认")).toHaveTextContent("提交结果尚未确认");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "变更规则详情" })).not.toBeInTheDocument());
    // Read-only details remain available; reopening them must not provide a
    // second lifecycle command before the uncertain result has been checked.
    await user.click(screen.getByRole("button", { name: "查看" }));
    expect(await screen.findByRole("dialog", { name: "变更规则详情" })).toBeInTheDocument();
    const repeatedCommand = screen.queryByRole("button", { name: "删除" });
    if (repeatedCommand) {
      await user.click(repeatedCommand);
      const confirmation = screen.queryByRole("button", { name: "确认删除" });
      if (confirmation) await user.click(confirmation);
    }
    expect(writes).toBe(2);
    expect(screen.queryByRole("button", { name: "激活" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "关闭抽屉" }));
    for (const status of [503, 504]) {
      failedCheckStatus = status;
      failedReads = 0;
      finishCheck = undefined;
      await user.click(screen.getByRole("button", { name: "只读核对当前状态" }));
      await waitFor(() => expect(finishCheck).toBeTypeOf("function"));
      await act(async () => { finishCheck!(json({ error: { code: "policy_catalog_unavailable", message: "read unavailable", request_id: "req-check" } }, status)); });
      await waitFor(() => expect(failedReads).toBe(1));
      expect(await screen.findByLabelText("提交结果尚未确认")).toHaveTextContent("提交结果尚未确认");
      expect(screen.queryByRole("button", { name: "激活" })).not.toBeInTheDocument();
      expect(writes).toBe(2);
    }
    failedCheckStatus = 0;
    const readsBeforeCheck = reads;
    await user.click(screen.getByRole("button", { name: "只读核对当前状态" }));
    await waitFor(() => expect(reads).toBeGreaterThan(readsBeforeCheck));
    expect(await screen.findByText("当前查询结果（仅供核对）")).toBeVisible();
    expect(screen.queryByRole("button", { name: "激活" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "我已核对，结束本次核对" }));
    await waitFor(() => expect(screen.queryByLabelText("提交结果尚未确认")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "激活" })).toBeEnabled();
    expect(writes).toBe(2);
  });

  it("keeps a mutation-rule draft across same-account login and refetches before allowing save", async () => {
    let signedIn = true;
    let writes = 0;
    let detailReads = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/auth/session")) return signedIn ? json(testIdentity) : json({ error: { code: "session_invalid", message: "expired", request_id: "req-session" } }, 401);
      if (url.endsWith("/auth/csrf")) return json({ csrf_token: "preauth-csrf" });
      if (url.endsWith("/auth/login")) { signedIn = true; return json(testIdentity); }
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/editable_mutation_v2") && init?.method === "PUT") { writes += 1; return json(draftPolicy); }
      if (url.endsWith("/mutation-policies/editable_mutation_v2")) { detailReads += 1; return json(draftPolicy); }
      if (url.endsWith("/mutation-policies")) return json({ policies: [draftPolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    renderPage("/platform/mutation-policies/editable_mutation_v2?mode=edit");
    await user.clear(await screen.findByLabelText("显示名称"));
    await user.type(screen.getByLabelText("显示名称"), "恢复后的变更意图");
    signedIn = false;
    act(() => window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: "session_invalid" } })));
    await user.type(await screen.findByLabelText("用户名"), "test.user");
    await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
    await user.click(screen.getByRole("button", { name: "登录" }));
    await waitFor(() => expect(screen.queryByRole("heading", { name: "登录本地账号" })).not.toBeInTheDocument());
    expect(screen.getByLabelText("显示名称")).toHaveValue("恢复后的变更意图");
    expect(detailReads).toBeGreaterThanOrEqual(2);
    expect(writes).toBe(0);
  });

  it("fails an unknown Type closed while allowing only safe metadata updates", async () => {
    vi.stubGlobal("fetch", withAccountSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/future_mutation_v1")) return json(unknownPolicy);
      if (url.endsWith("/mutation-policies")) return json({ policies: [unknownPolicy] });
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage("/platform/mutation-policies/future_mutation_v1?mode=metadata");
    expect(await screen.findByText("仅可修改名称和描述")).toBeVisible();
    expect(screen.queryByText("仅可查看")).not.toBeInTheDocument();
    expect(await screen.findByText(/无法确认执行规则/)).toBeVisible();
    expect(screen.getByRole("region", { name: "规则效果无法确认" })).not.toHaveTextContent("规则允许");
    expect(await screen.findByDisplayValue("标准单表变更")).toBeEnabled();
    expect(screen.getByDisplayValue("creator")).toBeDisabled();
    expect(screen.getByRole("button", { name: "保存名称和描述" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "弃用" })).not.toBeInTheDocument();
  });

  it("fails a known Type closed when its registry capabilities are incomplete", async () => {
    vi.stubGlobal("fetch", withAccountSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD"] }] });
      if (url.endsWith("/mutation-policies/editable_mutation_v2")) return json(draftPolicy);
      if (url.endsWith("/mutation-policies")) return json({ policies: [draftPolicy] });
      throw new Error(`unexpected request ${url}`);
    })));

    renderPage("/platform/mutation-policies/editable_mutation_v2");
    expect(await screen.findByText("无法确认执行规则", { exact: false })).toBeVisible();
    expect(screen.getByText("当前只能安全查看。", { exact: false })).toBeVisible();
    expect(screen.queryByRole("button", { name: "修改执行规则" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "激活" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "删除" })).not.toBeInTheDocument();
  });

  it("activates a Draft only after explicit confirmation", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/editable_mutation_v2/activate") && init?.method === "POST") return json({ ...draftPolicy, status: "ACTIVE" });
      if (url.endsWith("/mutation-policies/editable_mutation_v2")) return json(draftPolicy);
      if (url.endsWith("/mutation-policies")) return json({ policies: [draftPolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/mutation-policies/editable_mutation_v2");
    await user.click((await screen.findAllByRole("button", { name: "激活" })).at(-1)!);
    expect(screen.getByRole("alertdialog", { name: "激活变更规则？" })).toBeVisible();
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith("/activate"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认激活" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/mutation-policies/editable_mutation_v2/activate",
      expect.objectContaining({ method: "POST" }),
    ));
    expect(await screen.findByText("变更规则已激活")).toBeVisible();
  });

  it("replaces a Draft with the complete execution DTO", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/editable_mutation_v2") && init?.method === "PUT") return json({ ...draftPolicy, ...JSON.parse(String(init.body)) });
      if (url.endsWith("/mutation-policies/editable_mutation_v2")) return json(draftPolicy);
      if (url.endsWith("/mutation-policies")) return json({ policies: [draftPolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/mutation-policies/editable_mutation_v2?mode=edit");
    expect(await screen.findByRole("region", { name: "执行效果预览" })).toBeVisible();
    await user.click(screen.getByRole("checkbox", { name: /DELETE/ }));
    await user.click(screen.getByRole("button", { name: "保存执行规则" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/mutation-policies/editable_mutation_v2",
      expect.objectContaining({ method: "PUT" }),
    ));
    const replaceCall = fetchMock.mock.calls.find(([url, init]) => String(url).endsWith("editable_mutation_v2") && init?.method === "PUT");
    expect(JSON.parse(String(replaceCall?.[1]?.body))).toMatchObject({
      code: "editable_mutation_v2",
      allow_add: true,
      allow_modify: true,
      allow_delete: true,
      create_operator_field: "creator",
      create_time_field: "created_at",
      modify_operator_field: "modifier",
      modify_time_field: "updated_at",
    });
  });

  it("updates only display metadata on an Active Policy", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (url.endsWith("/mutation-policies/standard_mutation_v1/metadata") && init?.method === "PATCH") return json({ ...activePolicy, name: "新显示名称" });
      if (url.endsWith("/mutation-policies/standard_mutation_v1")) return json(activePolicy);
      if (url.endsWith("/mutation-policies")) return json({ policies: [activePolicy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();

    renderPage("/platform/mutation-policies/standard_mutation_v1?mode=metadata");
    const name = await screen.findByLabelText("显示名称");
    await user.clear(name);
    await user.type(name, "新显示名称");
    await user.click(screen.getByRole("button", { name: "保存名称和描述" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/mutation-policies/standard_mutation_v1/metadata",
      expect.objectContaining({ method: "PATCH", body: JSON.stringify({ name: "新显示名称", description: activePolicy.description }) }),
    ));
  });

  it.each([
    ["弃用", "弃用变更规则？", "确认弃用", "deprecate", activePolicy],
    ["删除", "删除变更规则草稿？", "确认删除", "", draftPolicy],
  ])("confirms %s before issuing its lifecycle command", async (action, dialogTitle, confirmLabel, suffix, policy) => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      if (suffix && url.endsWith(`/mutation-policies/${policy.code}/${suffix}`) && init?.method === "POST") return json({ ...policy, status: "DEPRECATED" });
      if (!suffix && url.endsWith(`/mutation-policies/${policy.code}`) && init?.method === "DELETE") return new Response(null, { status: 204 });
      if (url.endsWith(`/mutation-policies/${policy.code}`)) return json(policy);
      if (url.endsWith("/mutation-policies")) return json({ policies: [policy] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAccountSession(fetchMock));
    const user = userEvent.setup();

    renderPage(`/platform/mutation-policies/${policy.code}`);
    await user.click((await screen.findAllByRole("button", { name: action })).at(-1)!);
    expect(screen.getByRole("alertdialog", { name: dialogTitle })).toBeVisible();
    await user.click(screen.getByRole("button", { name: confirmLabel }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([url, init]) => {
      const endpoint = suffix ? `/mutation-policies/${policy.code}/${suffix}` : `/mutation-policies/${policy.code}`;
      return String(url).endsWith(endpoint) && init?.method === (suffix ? "POST" : "DELETE");
    })).toBe(true));
    expect(await screen.findByText(action === "弃用" ? "变更规则已弃用" : "变更规则草稿已删除")).toBeVisible();
    await waitFor(() => expect(fetchMock.mock.calls.filter(([url, init]) => String(url).endsWith("/mutation-policies") && !init?.method).length).toBeGreaterThanOrEqual(2));
  });

});
