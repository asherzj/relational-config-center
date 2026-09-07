import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TestRouter } from "../../test/TestRouter";
import { afterEach, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
}

afterEach(() => vi.unstubAllGlobals());

it("keeps form input and presents stable Admin errors with Request ID", async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
    if (url.endsWith("/mutation-policies") && init?.method === "POST") return json({ error: { code: "mutation_policy_exists", message: "already exists", request_id: "req-mutation-42" } }, 409);
    if (url.endsWith("/mutation-policies")) return json({ policies: [] });
    throw new Error(`unexpected request ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  const user = userEvent.setup();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><TestRouter initialEntries={["/platform/mutation-policies/new"]}><ToastProvider><AppRoutes /></ToastProvider></TestRouter></QueryClientProvider>);

  await user.type(await screen.findByLabelText(/规则编码/), "duplicate_mutation_v1");
  await user.type(screen.getByLabelText("显示名称"), "重复规则");
  const createButton = screen.getByRole("button", { name: "创建草稿" });
  await waitFor(() => expect(createButton).toBeEnabled());
  await user.click(createButton);

  expect(await screen.findByRole("alert")).toHaveTextContent("变更规则编码已存在");
  expect(screen.getByRole("alert")).toHaveTextContent("req-mutation-42");
  expect(screen.getByLabelText(/规则编码/)).toHaveValue("duplicate_mutation_v1");
});
