import { describe, expect, it } from "vitest";
import { createConditionDraft, validateQueryDraft, type ManagedDataColumn } from "./model";

function validationFor(type: ManagedDataColumn["type"], value: string) {
  const column: ManagedDataColumn = { name: "value", type, nullable: false };
  return validateQueryDraft([column], [{ ...createConditionDraft(column.name), value }], "");
}

describe("Query Spec value validation", () => {
  it.each([
    ["float64", "0x10"],
    ["float64", " 1.5"],
    ["float64", "1e309"],
    ["float64", "0x1.fffffffffffff8p1023"],
    ["date", "2023-02-29"],
    ["datetime", "2026-13-01 12:00:00"],
    ["timestamp", "2026-02-29T12:00:00Z"],
  ] satisfies [ManagedDataColumn["type"], string][]) (
    "rejects %s value %s when Admin ParseColumnValue would reject it",
    (type, value) => expect(validationFor(type, value)).toContain("不符合"),
  );

  it.each([
    ["float64", "1.25e-3"],
    ["float64", "0x1.fp2"],
    ["float64", "0x1.fffffffffffffp1023"],
    ["date", "2024-02-29"],
    ["datetime", "2026-08-26 12:34:56.123456"],
    ["timestamp", "2026-08-26T12:34:56.123456Z"],
  ] satisfies [ManagedDataColumn["type"], string][]) (
    "accepts %s value %s when Admin ParseColumnValue accepts it",
    (type, value) => expect(validationFor(type, value)).toBeNull(),
  );
});
