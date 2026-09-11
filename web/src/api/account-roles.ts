import { z } from "zod";
import { request } from "./client";

export const accountRoleSchema = z.enum(["VIEWER", "EDITOR", "PUBLISHER", "ADMIN"]);
const historicalAccountRoleSchema = z.enum(["VIEWER", "EDITOR", "APPROVER", "PUBLISHER", "ADMIN"]);
export type AccountRole = z.infer<typeof accountRoleSchema>;
const version = z.string().regex(/^[1-9][0-9]*$/);
const roleAccountSchema = z.object({
 id: z.uuid(), username: z.string(), display_name: z.string(), enabled: z.boolean(),
 roles: z.array(accountRoleSchema).min(1).max(4), version,
});
export type RoleAccount = z.infer<typeof roleAccountSchema>;
const roleEventSchema = z.object({
 id: version, actor_kind: z.enum(["account", "maintenance"]), actor_id: z.string(), account_id: z.uuid(),
 before_roles: z.array(historicalAccountRoleSchema), after_roles: z.array(historicalAccountRoleSchema), version,
 created_at: z.iso.datetime({ offset: true }),
});
export const accountRoles = {
 list: (query: string, after = "") => request(`/api/v1/account-roles?${new URLSearchParams({ q:query, after })}`, { schema:z.object({accounts:z.array(roleAccountSchema),next_cursor:z.string()}) }),
 change: (id: string, roles: AccountRole[], expectedVersion: string, key: string) => request(`/api/v1/account-roles/${encodeURIComponent(id)}`, {
  method:"PUT", headers:{"Idempotency-Key":key}, body:JSON.stringify({roles,expected_version:expectedVersion}), schema:roleAccountSchema,
 }),
 history: (id: string, before = "") => request(`/api/v1/account-roles/${encodeURIComponent(id)}/history?${new URLSearchParams({before})}`, {schema:z.object({events:z.array(roleEventSchema),next_cursor:z.string()})}),
};
