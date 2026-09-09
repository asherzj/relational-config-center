import { z } from "zod";
import { request } from "./client";

export const uiTypes = ["text", "textarea", "number", "boolean", "date", "datetime", "select", "radio"] as const;
export const operators = ["exact", "contains", "open_range", "closed_range", "in", "not_in", "is_null", "is_not_null"] as const;
const policySchema = z.object({
  field_name: z.string(), display_name: z.string(), description: z.string(), display_order: z.number().int(),
  is_visible: z.boolean(), is_queryable: z.boolean(), query_operators: z.array(z.enum(operators)),
  ui_type: z.enum(uiTypes), ui_options: z.object({ options: z.array(z.object({ label: z.string(), value: z.string() })), min: z.string().optional(), max: z.string().optional(), step: z.string().optional() }),
  editable_on_add: z.boolean(), editable_on_modify: z.boolean(), is_required: z.boolean(),
  default_value: z.string().nullable().optional(), enabled: z.boolean(),
});
const rawPolicySchema = policySchema.extend({ ui_type: z.string(), query_operators: z.array(z.string()) });
const fieldSchema = z.object({
  field_name: z.string(), column_type: z.string(), nullable: z.boolean(), generated: z.boolean(), auto_increment: z.boolean(), has_default: z.boolean(),
  state: z.enum(["missing", "disabled", "active", "incompatible"]), warning: z.string(),
  policy: rawPolicySchema.nullable(), effective: policySchema,
  audit: z.object({ creator: z.string(), modifier: z.string(), created_at: z.string(), updated_at: z.string() }).nullable(),
});
export const fieldPoliciesSchema = z.object({ table_name: z.string(), fields: z.array(fieldSchema), query_capacity: z.object({ max_conditions: z.number().int().positive(), max_values_per_condition: z.number().int().positive(), queryable_fields: z.number().int().nonnegative(), supported: z.boolean() }) });
export type FieldPolicy = z.infer<typeof rawPolicySchema>;
export type FieldPolicyField = z.infer<typeof fieldSchema>;
export type FieldPolicies = z.infer<typeof fieldPoliciesSchema>;
const path = (table: string) => `/api/v1/table-field-policies/${encodeURIComponent(table)}`;
export const getFieldPolicies = (table: string) => request(path(table), { schema: fieldPoliciesSchema });
export const saveFieldPolicies = (table: string, policies: FieldPolicy[]) => request(path(table), { method: "PUT", body: JSON.stringify({ policies }), schema: fieldPoliciesSchema });
