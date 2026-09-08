import { afterEach, describe, expect, it, vi } from "vitest";
import { queryManagedTable } from "./managed-data";
import type { QueryCondition } from "../features/managed-data/model";

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json", "X-Request-ID": "req-managed-data-contract" },
  });
}

afterEach(() => vi.unstubAllGlobals());

describe("Managed Data query API contract", () => {
  it.each<[string, QueryCondition, object]>([
    ["exact preserves an empty JSON String", { field: "subject", operator: "exact", value: "" }, { field: "subject", operator: "exact", value: "" }],
    ["contains", { field: "body", operator: "contains", value: "ready" }, { field: "body", operator: "contains", value: "ready" }],
    ["open_range", { field: "priority", operator: "open_range", from: "10", to: "30" }, { field: "priority", operator: "open_range", from: "10", to: "30" }],
    ["closed_range with one boundary", { field: "active_from", operator: "closed_range", from: "2026-01-01" }, { field: "active_from", operator: "closed_range", from: "2026-01-01" }],
    ["in preserves an empty member", { field: "channel", operator: "in", values: ["EMAIL", ""] }, { field: "channel", operator: "in", values: ["EMAIL", ""] }],
    ["not_in", { field: "channel", operator: "not_in", values: ["SMS"] }, { field: "channel", operator: "not_in", values: ["SMS"] }],
    ["is_null has no value key", { field: "subject", operator: "is_null" }, { field: "subject", operator: "is_null" }],
    ["is_not_null has no value key", { field: "metadata", operator: "is_not_null" }, { field: "metadata", operator: "is_not_null" }],
  ])("serializes %s without inventing nullable values", async (_name, condition, expected) => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => json({
      columns: [{ name: "id", type: "uint64", nullable: false }],
      rows: [{ id: "1" }], record_versions: ["0"],
      page: { page_number: 2, page_size: 10, total_count: 21, total_pages: 3 },
    }));
    vi.stubGlobal("fetch", fetchMock);

    await queryManagedTable("notification_templates", {
      conditions: [condition],
      order: { field: "id", direction: "ASC" },
      pageNumber: 2,
      pageSize: 10,
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/tables/notification_templates/query",
      expect.objectContaining({ method: "POST" }),
    );
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      conditions: [expected],
      order: { field: "id", direction: "ASC" },
      page_number: 2,
      page_size: 10,
    });
  });

  it("fails closed when dynamic row keys do not exactly match the declared columns", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => json({
      columns: [
        { name: "id", type: "uint64", nullable: false },
        { name: "subject", type: "string", nullable: true },
      ],
      rows: [{ id: "1", unexpected: "leaked" }], record_versions: ["0"],
      page: { page_number: 1, page_size: 20, total_count: 1, total_pages: 1 },
    })));

    await expect(queryManagedTable("notification_templates", { conditions: [] })).rejects.toMatchObject({
      code: "contract_mismatch",
      requestId: "req-managed-data-contract",
    });
  });

  it("fails closed instead of rounding 64-bit page counts beyond JavaScript's safe range", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => json({
      columns: [{ name: "id", type: "uint64", nullable: false }],
      rows: [], record_versions: [],
      page: {
        page_number: 1,
        page_size: 20,
        total_count: 9_007_199_254_740_992,
        total_pages: 450_359_962_737_050,
      },
    })));

    await expect(queryManagedTable("notification_templates", { conditions: [] })).rejects.toMatchObject({
      code: "contract_mismatch",
      requestId: "req-managed-data-contract",
    });
  });
});

// Row writes were removed in T5. Serialization now preserves intent at the
// release-order boundary, where confirmation cannot change configuration.
