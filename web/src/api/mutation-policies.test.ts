import { afterEach, describe, expect, it, vi } from "vitest";
import { listMutationPolicies, listMutationPolicyTypes } from "./mutation-policies";

function json(value: unknown) {
  return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json", "X-Request-ID": "req-contract" } });
}

afterEach(() => vi.unstubAllGlobals());

describe("Mutation Policy API contract", () => {
  it("maps registered operations and nullable Auto Fill fields into the Web model", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith("/mutation-policy-types")) return json({ types: [{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }] });
      return json({ policies: [{
        code: "readonly_mutation_v1", name: "只读", description: "", type_code: "single_table_mutation",
        allow_add: false, allow_modify: false, allow_delete: false,
        create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null,
        status: "DRAFT", creator: "admin", modifier: "admin",
        gmt_created: "2026-08-22T09:12:08Z", gmt_modified: "2026-08-22T09:12:08Z",
      }] });
    }));

    await expect(listMutationPolicyTypes()).resolves.toEqual([{ code: "single_table_mutation", operations: ["ADD", "MODIFY", "DELETE"] }]);
    await expect(listMutationPolicies()).resolves.toMatchObject([{ code: "readonly_mutation_v1", createOperatorField: null, allowAdd: false }]);
  });

  it("fails closed when Type operations violate the Admin contract", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ types: [{ code: "single_table_mutation", operations: ["UPSERT"] }] })));

    await expect(listMutationPolicyTypes()).rejects.toMatchObject({ code: "contract_mismatch", requestId: "req-contract" });
  });

  it.each([
    [["ADD", "MODIFY"]],
    [["ADD", "MODIFY", "MODIFY"]],
  ])("fails closed when registered operations are incomplete or repeated: %j", async (operations) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ types: [{ code: "single_table_mutation", operations }] })));

    await expect(listMutationPolicyTypes()).rejects.toMatchObject({ code: "contract_mismatch", requestId: "req-contract" });
  });
});
