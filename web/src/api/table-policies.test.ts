import { afterEach, describe, expect, it, vi } from "vitest";
import { createTablePolicy, listDatabaseTables, listTablePolicies } from "./table-policies";

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json", "X-Request-ID": "req-table-contract" } });
}

afterEach(() => vi.unstubAllGlobals());

describe("Table Policy API contract", () => {
  it("maps Discovery and assignment audit DTOs into Web models", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith("/database-tables")) return json({ tables: [{
        table_name: "notification_templates", table_comment: "通知模板", policy_exists: true,
        policy_enabled: false, compatible: true, incompatibility_reason: null,
      }] });
      return json({ policies: [{
        table_name: "notification_templates", query_policy_code: "standard_query_v1",
        mutation_policy_code: "standard_mutation_v1", version:"1", enabled: false, creator: "admin", modifier: "admin",
        created_at: "2026-08-24T09:00:00Z", updated_at: "2026-08-24T10:00:00Z",
      }] });
    }));

    await expect(listDatabaseTables()).resolves.toMatchObject([{ tableName: "notification_templates", tableComment: "通知模板", compatible: true }]);
    await expect(listTablePolicies()).resolves.toMatchObject([{ tableName: "notification_templates", queryPolicyCode: "standard_query_v1", enabled: false }]);
  });

  it("serializes all three immutable assignment identifiers on create", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => json({
      ...JSON.parse(String(init?.body)), version:"1", enabled: false, creator: "admin", modifier: "admin",
      created_at: "2026-08-24T09:00:00Z", updated_at: "2026-08-24T09:00:00Z",
    }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await createTablePolicy({ key:"test-create-key", assignment:{ tableName: "notification_templates", queryPolicyCode: "standard_query_v1", mutationPolicyCode: "standard_mutation_v1" } });

    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      table_name: "notification_templates",
      query_policy_code: "standard_query_v1",
      mutation_policy_code: "standard_mutation_v1",
      concurrency_key: [],
    });
  });

  it("fails closed when Discovery compatibility and its stable reason contradict", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ tables: [{
      table_name: "broken", table_comment: "", policy_exists: false, policy_enabled: false,
      compatible: true, incompatibility_reason: "missing_primary_key",
    }] })));

    await expect(listDatabaseTables()).rejects.toMatchObject({ code: "contract_mismatch", requestId: "req-table-contract" });
  });
});
