import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";
import { TestRouter } from "../../test/TestRouter";
import { withAdminSession } from "../../test/account-session";

const standard = {
  code: "ordinary_release_v1", name: "常规发布", description: "标准流程", type: "STANDARD",
  node_list: [
    { code: "approval", type: "APPROVAL", name: "按表审批", required_role: "TABLE_APPROVER" },
    { code: "publication", type: "PUBLICATION", name: "发布", required_role: "PUBLISHER" },
    { code: "completion", type: "COMPLETION", name: "完结", required_role: "PUBLISHER" },
  ],
  monitor_list: [], enabled: true, version: "1", creator: "admin-id", modifier: "admin-id",
  created_at: "2026-09-11T00:00:00Z", updated_at: "2026-09-11T00:00:00Z",
};
const emergency = { ...standard, code: "urgent_release_v1", name: "应急发布", type: "EMERGENCY", node_list: standard.node_list.slice(1) };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
function renderPage(path = "/platform/release-templates") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><TestRouter initialEntries={[path]}><ToastProvider><AppRoutes /></ToastProvider></TestRouter></QueryClientProvider>);
}
afterEach(() => vi.unstubAllGlobals());

describe("发布流程模板页面", () => {
  it("shows the formal ADMIN catalog and protects emergency lifecycle actions", async () => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/release-templates")) return json({ templates: [standard, emergency] });
      if (url.endsWith("/release-templates/urgent_release_v1")) return json(emergency);
      throw new Error(`unexpected request ${url}`);
    })));
    const user = userEvent.setup();
    renderPage();
    expect(await screen.findByRole("heading", { name: "发布流程模板" })).toBeVisible();
    expect(screen.getByRole("link", { name: "发布流程模板" })).toHaveClass("active");
    await user.click((await screen.findAllByRole("button", { name: "查看" }))[1]);
    expect(await screen.findByRole("button", { name: "停用" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "删除" })).toBeDisabled();
  });

  it("creates an emergency template with a constrained node list and a request identity", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/release-templates") && init?.method === "POST") return json(emergency, 201);
      if (url.endsWith("/release-templates")) return json({ templates: [] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();
    renderPage("/platform/release-templates/new");
    await user.type(await screen.findByLabelText(/模板编码/), "urgent_release_v1");
    await user.type(screen.getByLabelText("模板名称"), "应急发布");
    await user.selectOptions(screen.getByLabelText("发布类型"), "EMERGENCY");
    await user.click(screen.getByRole("button", { name: "保存模板" }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(true));
    const [, request] = fetchMock.mock.calls.find(([, init]) => init?.method === "POST")!;
    expect(new Headers(request?.headers).get("Idempotency-Key")).toMatch(/^release-template-/);
    const body = JSON.parse(String(request?.body));
    expect(body.node_list.map((node: { type: string }) => node.type)).toEqual(["PUBLICATION", "COMPLETION"]);
    expect(body.monitor_list).toBeUndefined();
  });

  it("keeps stale edit input and submits the version observed when editing began", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/release-templates/ordinary_release_v1") && init?.method === "PUT") return json({ error: { code: "release_template_conflict", message: "模板已被其他管理员修改", request_id: "template-conflict" } }, 409);
      if (url.endsWith("/release-templates/ordinary_release_v1")) return json(standard);
      if (url.endsWith("/release-templates")) return json({ templates: [standard] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();
    renderPage("/platform/release-templates/ordinary_release_v1?mode=edit");
    const name = await screen.findByLabelText("模板名称");
    await user.clear(name);
    await user.type(name, "我的未保存版本");
    await user.click(screen.getByRole("button", { name: "保存模板" }));
    expect(await screen.findByText(/模板已被其他管理员修改/)).toBeVisible();
    expect(name).toHaveValue("我的未保存版本");
    const [, request] = fetchMock.mock.calls.find(([, init]) => init?.method === "PUT")!;
    expect(JSON.parse(String(request?.body)).expected_version).toBe("1");
  });

  it("uses the shared confirmation dialog and focuses cancel before disabling a standard template", async () => {
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/release-templates/ordinary_release_v1")) return json(standard);
      if (url.endsWith("/release-templates")) return json({ templates: [standard] });
      throw new Error(`unexpected request ${url}`);
    })));
    const user = userEvent.setup();
    renderPage("/platform/release-templates/ordinary_release_v1");
    await user.click(await screen.findByRole("button", { name: "停用" }));
    expect(screen.getByRole("alertdialog", { name: /停用发布流程模板/ })).toBeVisible();
    expect(screen.getByRole("button", { name: "取消" })).toHaveFocus();
  });

  it("retries an unknown edit with the original body, version and request identity", async () => {
    let attempts = 0;
    const updated = { ...standard, name: "已保存名称", version: "2" };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/release-templates/ordinary_release_v1") && init?.method === "PUT") {
        attempts += 1;
        if (attempts === 1) throw new TypeError("connection closed after request");
        return json(updated);
      }
      if (url.endsWith("/release-templates/ordinary_release_v1")) return json(standard);
      if (url.endsWith("/release-templates")) return json({ templates: [standard] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();
    renderPage("/platform/release-templates/ordinary_release_v1?mode=edit");
    const name = await screen.findByLabelText("模板名称");
    await user.clear(name);
    await user.type(name, "已保存名称");
    await user.click(screen.getByRole("button", { name: "保存模板" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("保存结果未知；请保持当前页面，再次点击保存将原样重推同一请求。");
    expect(name).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "重推原请求" }));
    await waitFor(() => expect(attempts).toBe(2));
    const writes = fetchMock.mock.calls.filter(([, init]) => init?.method === "PUT");
    expect(writes[0][1]?.body).toBe(writes[1][1]?.body);
    expect(new Headers(writes[0][1]?.headers).get("Idempotency-Key")).toBe(new Headers(writes[1][1]?.headers).get("Idempotency-Key"));
  });

  it("keeps an unknown lifecycle request bound to its original action and target", async () => {
    let attempts = 0;
    const disabled = { ...standard, enabled: false, version: "2" };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/release-templates/ordinary_release_v1/disable") && init?.method === "POST") {
        attempts += 1;
        if (attempts === 1) return json({ error: { code: "release_template_unavailable", message: "response lost after commit", request_id: "lifecycle-unknown" } }, 503);
        return json(disabled);
      }
      if (url.endsWith("/release-templates/ordinary_release_v1")) return json(standard);
      if (url.endsWith("/release-templates")) return json({ templates: [standard] });
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", withAdminSession(fetchMock));
    const user = userEvent.setup();
    renderPage("/platform/release-templates/ordinary_release_v1");
    await user.click(await screen.findByRole("button", { name: "停用" }));
    await user.click(screen.getByRole("button", { name: "确认停用" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("操作结果未知；再次确认将原样重推同一请求。");
    expect(screen.getByRole("alert")).toHaveTextContent("请求编号：lifecycle-unknown");
    await user.click(screen.getByRole("button", { name: /^取消$/ }));
    expect(screen.getByRole("alert")).toHaveTextContent("请求编号：lifecycle-unknown");
    expect(screen.getAllByRole("button", { name: "编辑" }).some(button => button.hasAttribute("disabled"))).toBe(true);
    expect(screen.getByRole("button", { name: "删除" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "重推停用请求" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "重推停用请求" }));
    expect(screen.getByRole("alertdialog", { name: /ordinary_release_v1/ })).toHaveTextContent("停用");
    await user.click(screen.getByRole("button", { name: "重推原请求" }));
    await waitFor(() => expect(attempts).toBe(2));
    const writes = fetchMock.mock.calls.filter(([, init]) => init?.method === "POST");
    expect(String(writes[0][0])).toBe(String(writes[1][0]));
    expect(writes[0][1]?.body).toBe(writes[1][1]?.body);
    expect(new Headers(writes[0][1]?.headers).get("Idempotency-Key")).toBe(new Headers(writes[1][1]?.headers).get("Idempotency-Key"));
  });

  it("does not carry a discarded template error into another target", async () => {
    const second = { ...standard, code: "second_standard_v1", name: "第二模板" };
    vi.stubGlobal("fetch", withAdminSession(vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/release-templates/ordinary_release_v1") && init?.method === "PUT") return json({ error: { code: "release_template_conflict", message: "conflict", request_id: "old-target" } }, 409);
      if (url.endsWith("/release-templates/ordinary_release_v1")) return json(standard);
      if (url.endsWith("/release-templates/second_standard_v1")) return json(second);
      if (url.endsWith("/release-templates")) return json({ templates: [standard, second] });
      throw new Error(`unexpected request ${url}`);
    })));
    const user = userEvent.setup();
    renderPage("/platform/release-templates/ordinary_release_v1?mode=edit");
    const name = await screen.findByLabelText("模板名称");
    await user.clear(name);
    await user.type(name, "冲突输入");
    await user.click(screen.getByRole("button", { name: "保存模板" }));
    expect(await screen.findByText(/模板已被其他管理员修改/)).toBeVisible();
    await user.click(screen.getAllByRole("button", { name: /^关闭$/ }).at(-1)!);
    await user.click(await screen.findByRole("button", { name: "放弃修改并离开" }));
    await user.click((await screen.findAllByRole("button", { name: "编辑" }))[1]);
    expect(await screen.findByDisplayValue("第二模板")).toBeVisible();
    expect(screen.queryByText(/模板已被其他管理员修改/)).not.toBeInTheDocument();
    expect(screen.queryByText("old-target")).not.toBeInTheDocument();
  });
});
