import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppRoutes } from "../app";
import { ToastProvider } from "../components/ui/Toast";

const audit = { creator: "fixture", modifier: "fixture", gmt_created: "2026-09-07T00:00:00Z", gmt_modified: "2026-09-07T00:00:00Z" };
const query = { ...audit, code: "query_v1", name: "查询基线", description: "说明", type_code: "page_query", default_order_field: "id", default_order_direction: "DESC", default_page_size: 20, max_page_size: 200, status: "DRAFT" };
const mutation = { ...audit, code: "mutation_v1", name: "变更基线", description: "说明", type_code: "single_table_mutation", allow_add: true, allow_modify: true, allow_delete: true, create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null, status: "DRAFT" };
const assignment = { ...audit, table_name: "items", query_policy_code: "query_v1", mutation_policy_code: "mutation_v1", enabled: true };
const columns = [{ name: "id", type: "uint64", nullable: false }, { name: "name", type: "string", nullable: false }, { name: "note", type: "string", nullable: true }];
const row = { id: "1", name: "original", note: null };
function json(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } }); }
const rejection = () => json({ error: { code: "invalid_mutation_content", message: "invalid", request_id: "req-preserved" } }, 400);

function backend({ active = false, write }: { active?: boolean; write?: (url: string, init: RequestInit) => Promise<Response> } = {}) {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (init?.method && init.method !== "GET" && !url.endsWith("/query")) {
      if (write) return write(url, init);
      throw new Error(`Unexpected write ${url}`);
    }
    if (url.endsWith("/query-policy-types")) return json({ types: [{ code: "page_query" }] });
    if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
    if (url.endsWith("/query-policies")) return json({ policies: [{ ...query, status: "ACTIVE" }, { ...query, code: "query_v2", default_page_size: 12, max_page_size: 50, status: "ACTIVE" }] });
    if (url.endsWith("/mutation-policies")) return json({ policies: [{ ...mutation, status: "ACTIVE" }] });
    if (url.includes("/query-policies/")) return json({ ...query, ...(active ? { status: "ACTIVE" } : {}) });
    if (url.includes("/mutation-policies/")) return json({ ...mutation, ...(active ? { status: "ACTIVE" } : {}) });
    if (url.endsWith("/table-policies")) return json({ policies: [assignment, { ...assignment, table_name: "other_items" }] });
    if (url.includes("/table-policies/")) return json(assignment);
    if (url.endsWith("/database-tables")) return json({ tables: [{ table_name: "new_items", table_comment: "", policy_exists: false, policy_enabled: false, compatible: true, incompatibility_reason: null }] });
    if (url.endsWith("/query")) return json({ columns, rows: [row], page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 } });
    throw new Error(`Unexpected read ${url}`);
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
function mount(path: string, entries = [path], index = entries.length - 1) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const router = createMemoryRouter([{ path: "*", element: <ToastProvider><AppRoutes /></ToastProvider> }], { initialEntries: entries, initialIndex: index });
  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>);
  return { client, router };
}
function unloadPrevented() {
  const event = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}
const confirmLeave = () => screen.getByRole("alertdialog", { name: "放弃未保存的修改？" });
const variants = (["query", "mutation"] as const).flatMap((kind) => (["create", "edit", "metadata"] as const).map((mode) => ({ kind, mode })));
afterEach(() => vi.unstubAllGlobals());

describe("rule drafts and navigation protection", () => {
  it.each(variants)("$kind $mode: protects all drawer exits and treats exact restoration as clean", async ({ kind, mode }) => {
    backend({ active: mode === "metadata" });
    const user = userEvent.setup();
    const path = `/platform/${kind}-policies/${mode === "create" ? "new" : `${kind}_v1?mode=${mode}`}`;
    const { router } = mount(path);
    const name = await screen.findByRole("textbox", { name: "显示名称" });
    const baseline = (name as HTMLInputElement).value;
    expect(unloadPrevented()).toBe(false);
    await user.type(name, " ");
    expect(unloadPrevented()).toBe(false);
    await user.clear(name);
    if (baseline) await user.type(name, baseline);
    if (kind === "mutation" && mode !== "metadata") {
      const optional = screen.getByRole("textbox", { name: "新增时填写 Operator 的列" });
      await user.type(optional, "operator");
      expect(unloadPrevented()).toBe(true);
      await user.clear(optional);
      expect(unloadPrevented()).toBe(false);
    }
    await user.type(name, "未保存");
    expect(unloadPrevented()).toBe(true);
    const triggers = [
      () => user.click(screen.getByRole("button", { name: "关闭" })),
      () => user.click(screen.getByRole("button", { name: "取消" })),
      () => user.keyboard("{Escape}"),
      () => user.click(screen.getByRole("button", { name: "关闭抽屉" })),
    ];
    for (const trigger of triggers) {
      await trigger();
      expect(within(confirmLeave()).getByRole("button", { name: "继续编辑" })).toHaveFocus();
      await user.keyboard("{Escape}");
      expect(name).toHaveValue(`${baseline}未保存`);
      expect(router.state.location.pathname).toBe(path.split("?")[0]);
    }
    await user.clear(name);
    if (baseline) await user.type(name, baseline);
    expect(unloadPrevented()).toBe(false);
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/platform/${kind}-policies`);
  });

  it("cancels and confirms history back/forward, resource/mode changes, and main navigation", async () => {
    backend();
    const user = userEvent.setup();
    const path = "/platform/query-policies/query_v1?mode=edit";
    const { router } = mount(path, ["/platform/query-policies", path, "/platform/mutation-policies"], 1);
    const name = await screen.findByRole("textbox", { name: "显示名称" });
    await user.type(name, "draft");
    for (const destination of [-1, 1, "/platform/query-policies/query_v1", "/platform/query-policies/new"] as const) {
      await act(() => typeof destination === "number" ? router.navigate(destination) : router.navigate(destination));
      expect(confirmLeave()).toBeVisible();
      await user.click(screen.getByRole("button", { name: "继续编辑" }));
      expect(name).toHaveValue("查询基线draft");
      expect(router.state.location.search).toBe("?mode=edit");
    }
    await user.click(screen.getByRole("link", { name: "变更规则定义" }));
    await user.click(screen.getByRole("button", { name: "放弃修改并离开" }));
    expect(router.state.location.pathname).toBe("/platform/mutation-policies");
    await act(() => router.navigate(-1));
    const restored = await screen.findByRole("textbox", { name: "显示名称" });
    expect(restored).toHaveValue("查询基线");
    await user.type(restored, "again");
    await act(() => router.navigate(1));
    await user.click(screen.getByRole("button", { name: "放弃修改并离开" }));
    expect(router.state.location.pathname).toBe("/platform/mutation-policies");
    await act(() => router.navigate(-1));
    await user.type(await screen.findByRole("textbox", { name: "显示名称" }), "back");
    await act(() => router.navigate(-1));
    await user.click(screen.getByRole("button", { name: "放弃修改并离开" }));
    expect(router.state.location.pathname).toBe("/platform/query-policies");
  });

  it.each(variants)("$kind $mode: pending blocks repeat/leave, failure retains draft, success clears protection", async ({ kind, mode }) => {
    let resolve!: (response: Response) => void;
    const fetch = backend({ active: mode === "metadata", write: () => new Promise((done) => { resolve = done; }) });
    const user = userEvent.setup();
    const path = `/platform/${kind}-policies/${mode === "create" ? "new" : `${kind}_v1?mode=${mode}`}`;
    const { router } = mount(path);
    const name = await screen.findByRole("textbox", { name: "显示名称" });
    if (mode === "create") await user.type(screen.getByRole("textbox", { name: /规则编码/ }), `${kind}_v2`);
    await user.clear(name);
    await user.type(name, "保留输入");
    const form = name.closest("form")!;
    fireEvent.submit(form);
    fireEvent.submit(form);
    await waitFor(() => expect(name).toBeDisabled());
    expect(fetch.mock.calls.filter(([, init]) => ["POST", "PUT", "PATCH"].includes(init?.method ?? ""))).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "关闭" }));
    expect(await screen.findByRole("alertdialog", { name: "正在提交，请稍候" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "放弃修改并离开" })).not.toBeInTheDocument();
    await act(async () => resolve(rejection()));
    expect(await screen.findByRole("alert")).toHaveTextContent("req-preserved");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(name).toHaveValue("保留输入");
    expect(name).toBeEnabled();
    expect(unloadPrevented()).toBe(true);
    fireEvent.submit(form);
    await waitFor(() => expect(name).toBeDisabled());
    await act(() => router.navigate("/platform/table-policies"));
    expect(await screen.findByRole("alertdialog", { name: "正在提交，请稍候" })).toBeVisible();
    const value = { ...(kind === "query" ? query : mutation), name: "保留输入", code: `${kind}_${mode === "create" ? "v2" : "v1"}`, status: mode === "metadata" ? "ACTIVE" : "DRAFT" };
    await act(async () => resolve(json(value, mode === "create" ? 201 : 200)));
    await waitFor(() => expect(router.state.location.search).toBe(""));
    expect(router.state.location.pathname).toBe(`/platform/${kind}-policies/${value.code}`);
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(unloadPrevented()).toBe(false);
    expect(await screen.findByRole("textbox", { name: "显示名称" })).toBeDisabled();
  });

  it.each(["query", "mutation"] as const)("%s draft survives refreshed details and changed server status", async (kind) => {
    backend();
    const user = userEvent.setup();
    const { client } = mount(`/platform/${kind}-policies/${kind}_v1?mode=edit`);
    const name = await screen.findByRole("textbox", { name: "显示名称" });
    await user.type(name, "local");
    await act(async () => {
      client.setQueryData([`${kind}-policies`, "detail", `${kind}_v1`], (data: Record<string, unknown>) => ({ ...data, name: "remote", status: "ACTIVE" }));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    if (kind === "mutation") await act(async () => {
      client.setQueryData(["mutation-policy-types"], []);
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(name).toHaveValue(`${kind === "query" ? "查询" : "变更"}基线local`);
    expect(name).toBeEnabled();
    await user.keyboard("{Escape}");
    expect(confirmLeave()).toBeVisible();
  });
});

describe("table assignments and managed row drafts", () => {
  it("table read-only effects follow refreshed references", async () => {
    backend({ active: true });
    const { client } = mount("/platform/table-policies/items");
    const effects = await screen.findByRole("region", { name: "当前已选规则效果" });
    expect(effects).toHaveTextContent("默认每页数量为 20");
    act(() => client.setQueryData(["table-policies", "detail", "items"], (data: Record<string, unknown>) => ({ ...data, queryPolicyCode: "query_v2" })));
    await waitFor(() => expect(effects).toHaveTextContent("默认每页数量为 12"));
  });

  it.each(["query", "mutation"] as const)("%s read-only detail reflects refreshed values", async (kind) => {
    backend();
    const { client } = mount(`/platform/${kind}-policies/${kind}_v1`);
    const name = await screen.findByRole("textbox", { name: "显示名称" });
    act(() => client.setQueryData([`${kind}-policies`, "detail", `${kind}_v1`], (data: Record<string, unknown>) => ({ ...data, name: "latest" })));
    await waitFor(() => expect(name).toHaveValue("latest"));
    expect(unloadPrevented()).toBe(false);
  });

  it.each(["create", "replace"])("table %s preserves selections on cancel/refresh/failure and clears protection on success", async (mode) => {
    let resolve!: (response: Response) => void;
    backend({ active: true, write: () => new Promise((done) => { resolve = done; }) });
    const user = userEvent.setup();
    const path = mode === "create" ? "/platform/table-policies?mode=create" : "/platform/table-policies/items?mode=replace";
    const { client, router } = mount(path);
    const querySelect = await screen.findByRole("combobox", { name: "Active 查询规则" });
    expect(unloadPrevented()).toBe(false);
    if (mode === "create") {
      await user.selectOptions(screen.getByRole("combobox", { name: "真实数据库表" }), "new_items");
      await user.selectOptions(screen.getByRole("combobox", { name: "Active 变更规则" }), "mutation_v1");
    }
    await user.selectOptions(querySelect, "query_v2");
    expect(screen.getByRole("region", { name: "所选规则效果预览" })).toHaveTextContent("默认每页数量为 12");
    await user.keyboard("{Escape}");
    expect(confirmLeave()).toBeVisible();
    await user.click(screen.getByRole("button", { name: "继续编辑" }));
    expect(querySelect).toHaveValue("query_v2");
    if (mode === "replace") {
      act(() => client.setQueryData(["table-policies", "detail", "items"], (data: Record<string, unknown>) => ({ ...data, queryPolicyCode: "query_v2" })));
      expect(querySelect).toHaveValue("query_v2");
      expect(screen.getByRole("region", { name: "所选规则效果预览" })).toHaveTextContent("默认每页数量为 12");
      await user.selectOptions(querySelect, "query_v1");
      expect(unloadPrevented()).toBe(false);
      await user.selectOptions(querySelect, "query_v2");
    }
    const submit = async () => {
      fireEvent.submit(querySelect.closest("form")!);
      if (mode === "replace") await user.click(await screen.findByRole("button", { name: "确认替换" }));
      await waitFor(() => expect(querySelect).toBeDisabled());
    };
    await submit();
    if (mode === "replace") {
      expect(screen.getByRole("button", { name: "取消操作" })).toBeDisabled();
      await user.keyboard("{Escape}");
      expect(screen.getByRole("alertdialog", { name: "替换已启用的表规则？" })).toBeVisible();
    }
    await act(() => router.navigate("/platform/query-policies"));
    expect(await screen.findByRole("alertdialog", { name: "正在提交，请稍候" })).toBeVisible();
    await act(async () => resolve(rejection()));
    expect(await screen.findByRole("alert")).toHaveTextContent("req-preserved");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(querySelect).toHaveValue("query_v2");
    await submit();
    await act(async () => resolve(json({ ...assignment, table_name: mode === "create" ? "new_items" : "items", query_policy_code: "query_v2" })));
    await waitFor(() => expect(router.state.location.search).toBe(""));
    expect(unloadPrevented()).toBe(false);
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });

  it("keeps the same row draft across Change Set, canceled exits and refreshed table catalog", async () => {
    backend({ active: true });
    const user = userEvent.setup();
    const { client, router } = mount("/configuration/managed-data");
    const add = await screen.findByRole("button", { name: "新增记录" });
    await waitFor(() => expect(add).toBeEnabled());
    await user.click(add);
    expect(unloadPrevented()).toBe(false);
    await user.click(screen.getByRole("checkbox", { name: "包含 name" }));
    await user.type(screen.getByRole("textbox", { name: "name 值" }), "typed then omitted");
    await user.click(screen.getByRole("checkbox", { name: "包含 name" }));
    await user.click(screen.getByRole("checkbox", { name: "包含 note" }));
    await user.click(screen.getByRole("checkbox", { name: "note 使用 NULL" }));
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(confirmLeave()).toBeVisible();
    await user.click(screen.getByRole("button", { name: "继续编辑" }));
    expect(screen.getByRole("dialog", { name: "ADD Change Set" })).toContainElement(document.activeElement as HTMLElement);
    await user.click(screen.getByRole("button", { name: "返回修改" }));
    expect(screen.getByRole("textbox", { name: "name 值" })).toHaveValue("typed then omitted");
    expect(screen.getByRole("checkbox", { name: "包含 name" })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "note 使用 NULL" })).toBeChecked();
    await user.selectOptions(screen.getByRole("combobox", { name: "Managed Table" }), "other_items");
    await user.click(screen.getByRole("button", { name: "继续编辑" }));
    expect(screen.getByRole("combobox", { name: "Managed Table" })).toHaveValue("items");
    act(() => client.setQueryData(["table-policies", "list"], []));
    expect(screen.getByRole("textbox", { name: "name 值" })).toHaveValue("typed then omitted");
    await act(() => router.navigate("/platform/query-policies"));
    await user.click(screen.getByRole("button", { name: "放弃修改并离开" }));
    expect(router.state.location.pathname).toBe("/platform/query-policies");
    expect(unloadPrevented()).toBe(false);
  });

  it("row execution cannot be canceled or repeated in flight, then exposes failure and success without stale prompts", async () => {
    let resolve!: (response: Response) => void;
    const fetch = backend({ active: true, write: () => new Promise((done) => { resolve = done; }) });
    const user = userEvent.setup();
    const { router } = mount("/configuration/managed-data");
    const edit = await screen.findByRole("button", { name: "修改记录 1" });
    await waitFor(() => expect(edit).toBeEnabled());
    await user.click(edit);
    await user.click(screen.getByRole("checkbox", { name: "包含 name" }));
    await user.type(screen.getByRole("textbox", { name: "name 值" }), "changed");
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
    const execute = screen.getByRole("button", { name: "确认并执行" });
    fireEvent.click(execute);
    fireEvent.click(execute);
    await waitFor(() => expect(execute).toBeDisabled());
    expect(fetch.mock.calls.filter(([, init]) => init?.method === "PATCH")).toHaveLength(1);
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: "MODIFY Change Set" })).toBeVisible();
    expect(screen.getByRole("button", { name: "取消 Change Set" })).toBeDisabled();
    await act(() => router.navigate("/platform/query-policies"));
    expect(await screen.findByRole("alertdialog", { name: "正在提交，请稍候" })).toBeVisible();
    await act(async () => resolve(rejection()));
    expect(await screen.findByRole("alert")).toHaveTextContent("req-preserved");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "返回修改" }));
    expect(screen.getByRole("textbox", { name: "name 值" })).toHaveValue("originalchanged");
    expect(screen.getByRole("alert")).toHaveTextContent("req-preserved");
    await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
    await user.click(screen.getByRole("button", { name: "确认并执行" }));
    await act(() => router.navigate("/platform/query-policies"));
    expect(await screen.findByRole("alertdialog", { name: "正在提交，请稍候" })).toBeVisible();
    await act(async () => resolve(json({ affected: 1 })));
    expect(await screen.findByRole("heading", { name: "MODIFY 已完成" })).toBeVisible();
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/configuration/managed-data");
    expect(unloadPrevented()).toBe(false);
  });

  it("restoring row controls is clean and DELETE preview explicitly cancels without writing", async () => {
    const fetch = backend({ active: true });
    const user = userEvent.setup();
    mount("/configuration/managed-data");
    const edit = await screen.findByRole("button", { name: "修改记录 1" });
    await waitFor(() => expect(edit).toBeEnabled());
    await user.click(edit);
    await user.click(screen.getByRole("checkbox", { name: "包含 name" }));
    await user.type(screen.getByRole("textbox", { name: "name 值" }), "x");
    await user.keyboard("{Backspace}");
    await user.click(screen.getByRole("checkbox", { name: "包含 name" }));
    expect(unloadPrevented()).toBe(false);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "删除记录 1" }));
    expect(screen.getByText(/尚未执行删除/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "取消删除" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(fetch.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
  });
});
