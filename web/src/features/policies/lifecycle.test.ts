import { describe, expect, it } from "vitest";
import { policyActionAvailability, policyTypeAvailabilityHint, requestedPolicyFormMode, resolvePolicyFormMode } from "./lifecycle";

describe("policy lifecycle rules", () => {
  it("keeps safe display metadata editable when execution support is unknown", () => {
    expect(policyActionAvailability("ACTIVE", false)).toEqual({
      replace: false,
      activate: false,
      delete: false,
      deprecate: false,
      metadata: true,
    });
    expect(resolvePolicyFormMode("metadata", "ACTIVE", false)).toBe("metadata");
    expect(resolvePolicyFormMode("replace", "DRAFT", false)).toBe("view");
  });

  it("derives form modes from the same lifecycle rules for direct URLs", () => {
    expect(requestedPolicyFormMode(false, "edit")).toBe("replace");
    expect(resolvePolicyFormMode("replace", "ACTIVE", true)).toBe("view");
    expect(resolvePolicyFormMode("metadata", "DRAFT", true)).toBe("view");
    expect(resolvePolicyFormMode("metadata", "DEPRECATED", true)).toBe("metadata");
  });

  it("describes the same safe fallback that the row actions expose", () => {
    expect(policyTypeAvailabilityHint("DRAFT", false)).toBe("仅可查看");
    expect(policyTypeAvailabilityHint("ACTIVE", false)).toBe("仅可修改名称和描述");
    expect(policyTypeAvailabilityHint("DEPRECATED", false)).toBe("仅可修改名称和描述");
    expect(policyTypeAvailabilityHint("ACTIVE", true)).toBeNull();
  });
});
