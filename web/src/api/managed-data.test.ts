import { afterEach, describe, expect, it, vi } from "vitest";
import { addManagedRow, deleteManagedRow, modifyManagedRow, queryManagedTable } from "./managed-data";
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
      rows: [{ id: "1" }],
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
      rows: [{ id: "1", unexpected: "leaked" }],
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
      rows: [],
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

describe("Managed Data mutation API contract", () => {
  it("preserves omitted, NULL, and empty-string Mutation Content across ADD, MODIFY, and DELETE", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === "POST") return json({ id: "9007199254740993" }, 201);
      return json({ affected: 1 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(addManagedRow("notification_templates", { subject: null, body: "" })).resolves.toEqual({ id: "9007199254740993" });
    await expect(modifyManagedRow("notification_templates", "9007199254740993", { subject: "ready", body: null })).resolves.toEqual({ affected: 1 });
    await expect(deleteManagedRow("notification_templates", "9007199254740993")).resolves.toEqual({ affected: 1 });

    expect(fetchMock.mock.calls.map(([url, init]) => [String(url), init?.method, init?.body ? JSON.parse(String(init.body)) : undefined])).toEqual([
      ["/api/v1/tables/notification_templates/rows", "POST", { content: { subject: null, body: "" } }],
      ["/api/v1/tables/notification_templates/rows/9007199254740993", "PATCH", { content: { subject: "ready", body: null } }],
      ["/api/v1/tables/notification_templates/rows/9007199254740993", "DELETE", undefined],
    ]);
  });

  it.each([
    ["ADD id must remain a JSON String", "POST", { id: 42 }],
    ["MODIFY affected must be positive", "PATCH", { affected: 0 }],
    ["MODIFY must affect exactly one row", "PATCH", { affected: 2 }],
    ["DELETE affected must be lossless", "DELETE", { affected: 9_007_199_254_740_992 }],
  ])("fails closed when %s", async (_name, method, response) => {
    vi.stubGlobal("fetch", vi.fn(async () => json(response, method === "POST" ? 201 : 200)));

    const action = method === "POST"
      ? addManagedRow("notification_templates", {})
      : method === "PATCH"
        ? modifyManagedRow("notification_templates", "1", {})
        : deleteManagedRow("notification_templates", "1");

    await expect(action).rejects.toMatchObject({ code: "contract_mismatch", requestId: "req-managed-data-contract" });
  });
});
