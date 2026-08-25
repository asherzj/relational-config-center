import { z } from "zod";

export const policyStatusSchema = z.enum(["DRAFT", "ACTIVE", "DEPRECATED"]);

export const queryPolicyDtoSchema = z.object({
  code: z.string(),
  name: z.string(),
  description: z.string(),
  type_code: z.string(),
  default_order_field: z.string(),
  default_order_direction: z.enum(["ASC", "DESC"]),
  default_page_size: z.number().int(),
  max_page_size: z.number().int(),
  status: policyStatusSchema,
  creator: z.string(),
  modifier: z.string(),
  gmt_created: z.string(),
  gmt_modified: z.string(),
});

export const queryPolicyListDtoSchema = z.object({
  policies: z.array(queryPolicyDtoSchema),
});

export const queryPolicyTypeListDtoSchema = z.object({
  types: z.array(z.object({ code: z.string() })),
});

export const adminErrorDtoSchema = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
    request_id: z.string(),
  }),
});

export type QueryPolicyDto = z.infer<typeof queryPolicyDtoSchema>;
export type PolicyStatusDto = z.infer<typeof policyStatusSchema>;

export type PutQueryPolicyDto = {
  code: string;
  name: string;
  description: string;
  type_code: string;
  default_order_field: string;
  default_order_direction: "ASC" | "DESC";
  default_page_size: number;
  max_page_size: number;
};

export type UpdateQueryPolicyMetadataDto = Pick<PutQueryPolicyDto, "name" | "description">;
