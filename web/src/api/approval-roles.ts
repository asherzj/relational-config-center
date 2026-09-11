import { z } from "zod";
import { request } from "./client";

const memberSchema = z.object({ id: z.uuid(), username: z.string(), display_name: z.string(), enabled: z.boolean() });
const roleSchema = z.object({
  id: z.uuid(), name: z.string(), description: z.string(), enabled: z.boolean(),
  version: z.string().regex(/^[1-9][0-9]*$/), members: z.array(memberSchema), referenced: z.boolean(), deleted: z.boolean(),
  creator: z.uuid(), modifier: z.uuid(), created_at: z.iso.datetime({ offset: true }), updated_at: z.iso.datetime({ offset: true }),
});
export type ApprovalMember = z.infer<typeof memberSchema>;
export type ApprovalRole = z.infer<typeof roleSchema>;
export type ApprovalRoleInput = { name: string; description: string; enabled: boolean; member_ids: string[]; expected_version?: string };
export const approvalRoles = {
  list: (q: string, after = "") => request(`/api/v1/approval-roles?${new URLSearchParams({ q, after })}`, { schema: z.object({ roles: z.array(roleSchema), next_cursor: z.string() }) }),
  get: (id: string) => request(`/api/v1/approval-roles/${encodeURIComponent(id)}`, { schema: roleSchema }),
  save: (id: string | undefined, input: ApprovalRoleInput, key: string) => request(`/api/v1/approval-roles${id ? `/${encodeURIComponent(id)}` : ""}`, {
    method: id ? "PUT" : "POST", headers: { "Idempotency-Key": key }, body: JSON.stringify(input), schema: roleSchema,
  }),
  remove: (id: string, version: string, key: string) => request(`/api/v1/approval-roles/${encodeURIComponent(id)}`, {
    method: "DELETE", headers: { "Idempotency-Key": key }, body: JSON.stringify({ expected_version: version }), schema: roleSchema,
  }),
};
