import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ManagedDataColumn } from "./model";
import { useManagedDataMutationWorkflow } from "./mutation-workflow";

const columns: ManagedDataColumn[] = [
  { name: "id", type: "uint64", nullable: false },
  { name: "name", type: "string", nullable: false },
];

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
}

function createWrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

afterEach(() => vi.unstubAllGlobals());

describe("Managed Data mutation workflow", () => {
  it("pins table identity, writes once, and retries only exact-id readback", async () => {
    let readbacks = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/mutation-policy-types")) {
        return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      }
      if (url.endsWith("/mutation-policies/full_mutation_v1")) {
        return json({
          code: "full_mutation_v1", name: "Full mutation", description: "", type_code: "single_table_mutation",
          allow_add: true, allow_modify: true, allow_delete: true,
          create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
          status: "ACTIVE", creator: "fixture", modifier: "fixture",
          gmt_created: "2026-08-28T00:00:00Z", gmt_modified: "2026-08-28T00:00:00Z",
        });
      }
      if (url.endsWith("/tables/managed_items/rows") && init?.method === "POST") return json({ id: "41" }, 201);
      if (url.endsWith("/tables/managed_items/query") && init?.method === "POST") {
        readbacks += 1;
        if (readbacks === 1) return json({ error: { code: "query_unavailable", message: "暂时无法回查", request_id: "req-1" } }, 503);
        return json({ columns, rows: [{ id: "41", name: "created" }], page: { page_number: 1, page_size: 1, total_count: 1, total_pages: 1 } });
      }
      throw new Error(`unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const hook = renderHook(
      ({ tableName }) => useManagedDataMutationWorkflow({ canEdit: true, tableName, mutationPolicyCode: "full_mutation_v1", columns }),
      { wrapper: createWrapper(), initialProps: { tableName: "managed_items" } },
    );
    await waitFor(() => expect(hook.result.current.view.capabilityReasons.ADD).toBeUndefined());

    act(() => hook.result.current.send({ type: "open-editor", operation: "ADD" }));
    expect(hook.result.current.view.editor?.tableName).toBe("managed_items");
    hook.rerender({ tableName: "another_table" });
    act(() => hook.result.current.send({ type: "review-content", content: { name: "created" } }));
    expect(hook.result.current.view.changeSet?.operation).toBe("ADD");

    act(() => {
      hook.result.current.send({ type: "confirm-pending" });
      hook.result.current.send({ type: "confirm-pending" });
    });
    await waitFor(() => expect(hook.result.current.view.outcome?.retrievalError).toBeDefined());
    expect(fetchMock.mock.calls.filter(([url, init]) => String(url).endsWith("/tables/managed_items/rows") && init?.method === "POST")).toHaveLength(1);

    act(() => hook.result.current.send({ type: "retry-readback" }));
    await waitFor(() => expect(hook.result.current.view.outcome?.row).toEqual({ id: "41", name: "created" }));
    expect(fetchMock.mock.calls.filter(([url, init]) => String(url).endsWith("/tables/managed_items/rows") && init?.method === "POST")).toHaveLength(1);
    expect(readbacks).toBe(2);
  });
});
