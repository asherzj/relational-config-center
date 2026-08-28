import { describe, expect, it } from "vitest";
import { validateDraft, type MutationPolicyDraft } from "./model";

const valid: MutationPolicyDraft = {
  code: "standard_mutation_v1",
  name: "标准变更",
  description: "",
  typeCode: "single_table_mutation",
  allowAdd: true,
  allowModify: true,
  allowDelete: false,
  createOperatorField: "creator",
  createTimeField: "created_at",
  modifyOperatorField: "modifier",
  modifyTimeField: "updated_at",
};

describe("Mutation Policy Draft validation", () => {
  it("accepts the supported relational rule shape", () => {
    expect(validateDraft(valid)).toEqual({});
  });

  it("rejects technology-bound codes, id, unsafe and duplicate targets", () => {
    expect(validateDraft({
      ...valid,
      code: "mysql_mutation_v1",
      createOperatorField: "id",
      createTimeField: "unsafe;field",
      modifyOperatorField: "same_field",
      modifyTimeField: "same_field",
    })).toMatchObject({
      code: expect.any(String),
      createOperatorField: expect.stringContaining("id"),
      createTimeField: expect.stringContaining("安全"),
      modifyOperatorField: expect.stringContaining("重复"),
      modifyTimeField: expect.stringContaining("重复"),
    });
  });

  it("enforces Auto Fill permission dependencies", () => {
    expect(validateDraft({ ...valid, allowAdd: false })).toMatchObject({
      createOperatorField: expect.stringContaining("ADD"),
      createTimeField: expect.stringContaining("ADD"),
    });
    expect(validateDraft({
      ...valid,
      allowAdd: false,
      allowModify: false,
      createOperatorField: null,
      createTimeField: null,
    })).toMatchObject({
      modifyOperatorField: expect.stringContaining("ADD 或 MODIFY"),
      modifyTimeField: expect.stringContaining("ADD 或 MODIFY"),
    });
  });
});
