import { describe, expect, it } from "vitest";
import { validateDraft } from "./model";

describe("query policy draft validation", () => {
  const valid = {
    code: "standard_page_query_v1",
    name: "标准分页查询",
    description: "",
    typeCode: "page_query",
    defaultOrderField: "id",
    defaultOrderDirection: "DESC" as const,
    defaultPageSize: 20,
    maxPageSize: 200,
  };

  it("accepts the supported policy shape", () => {
    expect(validateDraft(valid)).toEqual({});
  });

  it("rejects technology-bound codes and unsafe paging", () => {
    expect(validateDraft({ ...valid, code: "mysql_page_query_v1", defaultPageSize: 201 })).toMatchObject({
      code: expect.any(String),
      defaultPageSize: expect.any(String),
    });
  });
});
