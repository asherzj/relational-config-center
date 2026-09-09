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
  created_at: z.string(),
  updated_at: z.string(),
});

export const queryPolicyListDtoSchema = z.object({
  policies: z.array(queryPolicyDtoSchema),
});

export const queryPolicyTypeListDtoSchema = z.object({
  types: z.array(z.object({ code: z.string() })),
});

export const mutationOperationSchema = z.enum(["ADD", "MODIFY", "DELETE"]);

export const mutationPolicyTypeListDtoSchema = z.object({
  types: z.array(z.object({
    code: z.string(),
    operations: z.array(mutationOperationSchema).length(3).refine(
      (operations) => new Set(operations).size === operations.length,
      { message: "变更规则类型的操作不得重复" },
    ),
  })),
});

export const mutationPolicyDtoSchema = z.object({
  code: z.string(),
  name: z.string(),
  description: z.string(),
  type_code: z.string(),
  allow_add: z.boolean(),
  allow_modify: z.boolean(),
  allow_delete: z.boolean(),
  create_operator_field: z.string().nullable(),
  create_time_field: z.string().nullable(),
  modify_operator_field: z.string().nullable(),
  modify_time_field: z.string().nullable(),
  status: policyStatusSchema,
  creator: z.string(),
  modifier: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
});

export const mutationPolicyListDtoSchema = z.object({
  policies: z.array(mutationPolicyDtoSchema),
});

export const databaseTableDtoSchema = z.object({
  table_name: z.string(),
  table_comment: z.string(),
  policy_exists: z.boolean(),
  policy_enabled: z.boolean(),
  compatible: z.boolean(),
  incompatibility_reason: z.enum([
    "missing_primary_key",
    "composite_primary_key",
    "primary_key_must_be_id",
  ]).nullable(),
}).refine(
  (table) => table.compatible === (table.incompatibility_reason === null),
  { message: "compatible tables must not have an incompatibility reason" },
);

export const databaseTableListDtoSchema = z.object({
  tables: z.array(databaseTableDtoSchema),
});

export const tablePolicyDtoSchema = z.object({
  concurrency_key: z.array(z.string()).optional().default([]),
  table_name: z.string(),
  query_policy_code: z.string(),
  mutation_policy_code: z.string(),
  enabled: z.boolean(),
  creator: z.string(),
  modifier: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
});

export const tablePolicyListDtoSchema = z.object({
  policies: z.array(tablePolicyDtoSchema),
});

export const managedDataColumnTypeSchema = z.enum([
  "uint64",
  "int64",
  "decimal",
  "float64",
  "string",
  "boolean",
  "date",
  "time",
  "datetime",
  "timestamp",
  "json",
]);

export const managedDataQueryResponseDtoSchema = z.object({
  columns: z.array(z.object({
    name: z.string().min(1),
    type: managedDataColumnTypeSchema,
    nullable: z.boolean(),
  })),
  rows: z.array(z.record(z.string(), z.string().nullable())),
  record_versions: z.array(z.string().max(20).regex(/^(0|[1-9][0-9]*)$/).pipe(z.string().refine((v) => BigInt(v) <= 18446744073709551615n))),
  page: z.object({
    page_number: z.number().int().positive().refine(Number.isSafeInteger, "page_number must be a safe integer"),
    page_size: z.number().int().positive().refine(Number.isSafeInteger, "page_size must be a safe integer"),
    total_count: z.number().int().nonnegative().refine(Number.isSafeInteger, "total_count exceeds Web's lossless range"),
    total_pages: z.number().int().nonnegative().refine(Number.isSafeInteger, "total_pages exceeds Web's lossless range"),
  }),
}).superRefine((result, context) => {
  if (result.record_versions.length !== result.rows.length) context.addIssue({ code: "custom", message: "Every row must have a record version", path: ["record_versions"] });
  const names = result.columns.map((column) => column.name);
  if (new Set(names).size !== names.length) {
    context.addIssue({ code: "custom", message: "Managed Data columns must be unique", path: ["columns"] });
    return;
  }
  const expected = new Set(names);
  result.rows.forEach((row, index) => {
    const actual = Object.keys(row);
    if (actual.length !== expected.size || actual.some((name) => !expected.has(name))) {
      context.addIssue({ code: "custom", message: "Managed Data row does not match columns", path: ["rows", index] });
    }
  });
});


export const adminErrorDtoSchema = z.object({
  error: z.object({
    item_index: z.number().int().nonnegative().optional(),
    table_name: z.string().optional(), order_id: z.string().optional(), applicant_id: z.string().optional(),
    code: z.string(),
    message: z.string(),
    request_id: z.string(),
  }),
});

export type QueryPolicyDto = z.infer<typeof queryPolicyDtoSchema>;
export type PolicyStatusDto = z.infer<typeof policyStatusSchema>;
export type MutationPolicyDto = z.infer<typeof mutationPolicyDtoSchema>;
export type MutationPolicyTypeDto = z.infer<typeof mutationPolicyTypeListDtoSchema>["types"][number];
export type DatabaseTableDto = z.infer<typeof databaseTableDtoSchema>;
export type TablePolicyDto = z.infer<typeof tablePolicyDtoSchema>;
export type ManagedDataQueryResponseDto = z.infer<typeof managedDataQueryResponseDtoSchema>;

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

export type PutMutationPolicyDto = {
  code: string;
  name: string;
  description: string;
  type_code: string;
  allow_add: boolean;
  allow_modify: boolean;
  allow_delete: boolean;
  create_operator_field: string | null;
  create_time_field: string | null;
  modify_operator_field: string | null;
  modify_time_field: string | null;
};
