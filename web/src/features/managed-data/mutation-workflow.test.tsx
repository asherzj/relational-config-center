import { withDefaultRecordVersions } from "../../test/managed-data-fixture";
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
  value = withDefaultRecordVersions(value);
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
  it("pins reviewed table and field intent for draft saving without a row write", async () => {
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
          created_at: "2026-08-28T00:00:00Z", updated_at: "2026-08-28T00:00:00Z",
        });
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

    expect(hook.result.current.view.draftInput).toEqual({items:[{table_name:"managed_items",operation:"ADD",content:{name:"created"}}]});
    expect(fetchMock.mock.calls.some(([url])=>String(url).includes("/tables/"))).toBe(false);
  });
});
