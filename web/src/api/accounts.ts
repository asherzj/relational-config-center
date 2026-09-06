import { z } from "zod";
import { request } from "./client";

export const currentIdentitySchema = z.object({
  account: z.object({
    id: z.uuid(), username: z.string(), display_name: z.string(),
    email: z.string(), email_verified: z.literal(false), status: z.literal("enabled"),
  }),
  csrf_token: z.string().min(1),
  idle_expires_at: z.iso.datetime({ offset: true }),
  expires_at: z.iso.datetime({ offset: true }),
});
export type CurrentIdentity = z.infer<typeof currentIdentitySchema>;
export type Registration = { username: string; email: string; password: string; display_name?: string };

const prepareRequest = () => request("/api/v1/auth/csrf", { schema: z.object({ csrf_token: z.string().min(1) }), credentials: "same-origin" });
let pendingPreparation: ReturnType<typeof prepareRequest> | undefined;
function prepare() {
  // A still-running preparation may outlive a route. Share its result so a
  // later response cannot replace the Cookie behind a different CSRF token.
  pendingPreparation ??= prepareRequest().finally(() => { pendingPreparation = undefined; });
  return pendingPreparation;
}

// The browser keeps credential cookies HttpOnly; callers consume current identity.
// CSRF tokens and account data remain in memory and are never persisted.
export const accounts = {
  current: () => request("/api/v1/auth/session", { schema: currentIdentitySchema, credentials: "same-origin" }),
  prepare,
  register: (input: Registration, csrf: string) => request("/api/v1/auth/register", {
    method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": csrf }, body: JSON.stringify(input), schema: currentIdentitySchema,
  }),
  login: (input: { username: string; password: string }, csrf: string) => request("/api/v1/auth/login", {
    method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": csrf }, body: JSON.stringify(input), schema: currentIdentitySchema,
  }),
  logout: (csrf: string) => request<void>("/api/v1/auth/logout", { method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": csrf } }),
};
