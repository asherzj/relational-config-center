import { z } from "zod";
import { request } from "./client";

export const approvalRoleIdentitySchema = z.object({ id: z.string(), name: z.string() });
const assignmentSchema = z.object({ table_name: z.string(), version: z.string().regex(/^(0|[1-9][0-9]*)$/), role_ids: z.array(z.string()), roles: z.array(approvalRoleIdentitySchema) });
export type TableApprovalAssignment = z.infer<typeof assignmentSchema>;
export type ApprovalRoleIdentity = z.infer<typeof approvalRoleIdentitySchema>;
export type TableApprovalInput = { expected_version: string; role_ids: string[] };
export const tableApprovals = {
  get: (table: string) => request(`/api/v1/table-policies/${encodeURIComponent(table)}/approval-roles`, { schema: assignmentSchema }),
  save: (table: string, input: TableApprovalInput, key: string) => request(`/api/v1/table-policies/${encodeURIComponent(table)}/approval-roles`, { method: "PUT", body: JSON.stringify(input), headers: { "Idempotency-Key": key }, schema: assignmentSchema }),
};
