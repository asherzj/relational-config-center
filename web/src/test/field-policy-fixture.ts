import type { FieldPolicies } from "../api/field-policies";

// A successful metadata read contains every real field, including unconfigured ones.
export function defaultFieldPolicies(tableName: string, columns: readonly { name: string; type: FieldPolicies["fields"][number]["column_type"]; nullable: boolean; generated?: boolean }[]): FieldPolicies {
  return {
    table_name: tableName,
    query_capacity: { max_conditions: 256, max_values_per_condition: 100, queryable_fields: columns.length, supported: true },
    fields: columns.map(column => ({
      field_name: column.name, column_type: column.type, nullable: column.nullable, generated: column.generated ?? false,
      auto_increment: false, has_default: false, state: "missing", warning: "", policy: null, audit: null,
      effective: { field_name: column.name, display_name: column.name, description: "", display_order: 0, is_visible: true,
        is_queryable: true, query_operators: ["exact"], ui_type: "text", ui_options: { options: [] },
        editable_on_add: true, editable_on_modify: true, is_required: false, enabled: false },
    })),
  };
}
