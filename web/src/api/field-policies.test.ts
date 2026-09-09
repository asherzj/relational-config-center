import { describe, expect, it } from "vitest";
import { fieldPoliciesSchema } from "./field-policies";
import { ApiError, isUncertainWriteError } from "./client";

const effective = { field_name: "priority", display_name: "priority", description: "", display_order: 0, is_visible: true, is_queryable: true, query_operators: ["exact"], ui_type: "text", ui_options: { options: [] }, editable_on_add: true, editable_on_modify: true, is_required: false, enabled: false };
const field = { field_name: "priority", column_type: "string", nullable: true, generated: false, auto_increment: false, has_default: false, policy: null, effective, audit: null, state: "missing", warning: "" };
describe("field policy public transport", () => {
  it("preserves unknown stored controls for management while requiring safe effective controls", () => {
    const data = { table_name: "items", fields: [{ ...field, state: "incompatible", policy: { ...effective, ui_type: "future-control", query_operators: ["future-operator"] } }] };
    const result = fieldPoliciesSchema.parse(data);
    expect(result.fields[0].policy?.ui_type).toBe("future-control");
    expect(result.fields[0].effective.ui_type).toBe("text");
    expect(fieldPoliciesSchema.safeParse({ ...data, fields: [{ ...data.fields[0], effective: { ...effective, ui_type: "unknown" } }] }).success).toBe(false);
  });
  it("keeps absent, null and empty prefills distinct", () => {
    const result = fieldPoliciesSchema.parse({ table_name: "items", fields: [
      { ...field, policy: effective }, { ...field, policy: { ...effective, default_value: null } }, { ...field, policy: { ...effective, default_value: "" } },
    ] });
    expect(Object.hasOwn(result.fields[0].policy!, "default_value")).toBe(false);
    expect(result.fields[1].policy!.default_value).toBeNull();
    expect(result.fields[2].policy!.default_value).toBe("");
  });
  it("configuration rejection can be corrected directly; unavailable writes require readback", () => {
    expect(isUncertainWriteError(new ApiError("invalid_field_policy", "名称不能为空", 422))).toBe(false);
    expect(isUncertainWriteError(new ApiError("field_policy_unavailable", "unavailable", 503))).toBe(true);
    expect(isUncertainWriteError(new ApiError("field_policy_timeout", "timeout", 504))).toBe(true);
  });
});
