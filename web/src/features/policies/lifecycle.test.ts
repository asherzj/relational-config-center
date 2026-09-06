import { describe, expect, it } from "vitest";
import { policyActionAvailability, requestedPolicyFormMode, resolvePolicyFormMode } from "./lifecycle";

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
});
