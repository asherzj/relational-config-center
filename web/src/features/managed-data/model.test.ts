import { describe, expect, it } from "vitest";
import { buildChangeSet, createConditionDraft, validateQueryDraft, type ManagedDataColumn } from "./model";

function validationFor(type: ManagedDataColumn["type"], value: string) {
  const column: ManagedDataColumn = { name: "value", type, nullable: false };
  return validateQueryDraft([column], [{ ...createConditionDraft(column.name), value }], "", { max_conditions: 256, max_values_per_condition: 100 });
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

describe("Change Set", () => {
  const cellValue = (cell: ReturnType<typeof buildChangeSet>["rows"][number]["next"]) => cell.state === "value" ? cell.value : undefined;
  const columns: ManagedDataColumn[] = [
    { name: "id", type: "uint64", nullable: false },
    { name: "subject", type: "string", nullable: true },
    { name: "body", type: "string", nullable: false },
    { name: "price", type: "decimal", nullable: false },
    { name: "active_at", type: "timestamp", nullable: true },
    { name: "metadata", type: "json", nullable: true },
    { name: "modifier", type: "string", nullable: false },
  ];
  const original = {
    id: "9007199254740993",
    subject: null,
    body: "",
    price: "10.2300",
    active_at: "2026-08-26T09:10:11.123456Z",
    metadata: "{\"enabled\":true}",
    modifier: "alice",
  };

  it("ADD compares every field with a missing original and distinguishes submitted states", () => {
    const changeSet = buildChangeSet("ADD", columns, undefined, {
      subject: null,
      body: "",
      price: "10.2300",
      metadata: "{\"enabled\":true}",
    }, new Set(["modifier"]));

    expect(changeSet.rows.map((row) => [row.field, row.original.state, row.next.state, cellValue(row.next), row.autoFill])).toEqual([
      ["id", "missing", "unsubmitted", undefined, false],
      ["subject", "missing", "null", undefined, false],
      ["body", "missing", "empty", undefined, false],
      ["price", "missing", "value", "10.2300", false],
      ["active_at", "missing", "unsubmitted", undefined, false],
      ["metadata", "missing", "value", "{\"enabled\":true}", false],
      ["modifier", "missing", "unsubmitted", undefined, true],
    ]);
  });

  it("MODIFY compares every field, preserving unchanged values and marking Auto Fill as pending", () => {
    const changeSet = buildChangeSet("MODIFY", columns, original, { subject: "", body: "changed" }, new Set(["modifier"]));

    expect(changeSet.rows.map((row) => [row.field, row.original.state, row.next.state, row.changed, cellValue(row.next)])).toEqual([
      ["id", "value", "value", false, "9007199254740993"],
      ["subject", "null", "empty", true, undefined],
      ["body", "empty", "value", true, "changed"],
      ["price", "value", "value", false, "10.2300"],
      ["active_at", "value", "value", false, "2026-08-26T09:10:11.123456Z"],
      ["metadata", "value", "value", false, "{\"enabled\":true}"],
      ["modifier", "value", "unsubmitted", true, undefined],
    ]);
  });

  it("DELETE compares every original field with a missing new record", () => {
    const changeSet = buildChangeSet("DELETE", columns, original, {}, new Set());

    expect(changeSet.rows).toHaveLength(columns.length);
    expect(changeSet.rows.every((row) => row.original.state !== "missing" && row.next.state === "missing" && row.changed)).toBe(true);
  });
});
