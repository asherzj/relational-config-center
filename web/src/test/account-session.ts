import { beforeEach, vi } from "vitest";
beforeEach(() => {
  vi.stubGlobal("navigator", Object.defineProperty(Object.create(navigator), "locks", { configurable: true, value: { request: (_name: string, callback: () => Promise<unknown>) => callback() } }));
});
export const testIdentity = {
  account: { id: "ab09850e-ef9a-4317-a000-d67465416b5b", username: "test.user", display_name: "测试账号", email: "test@example.com", email_verified: false, status: "enabled", roles: ["VIEWER"] },
  csrf_token: "test-session-csrf", expires_at: "2099-09-07T08:00:00Z", idle_expires_at: "2099-09-07T00:30:00Z",
};

// Web tests mock the public HTTP boundary, including the real session contract.
// Unrelated page fixtures start with no personal reminders. Notification tests
// own their counts route directly so dedicated failure cases are not masked.
export function withAccountSession(implementation: typeof fetch): typeof fetch {
  return (input, init) => ["/api/v1/auth/session", "/api/v1/auth/activity"].includes(String(input))
    ? Promise.resolve(new Response(JSON.stringify(testIdentity), {status:200,headers:{"Content-Type":"application/json"}}))
    : String(input) === "/api/v1/approval-notifications" ? Promise.resolve(Response.json({unread_count:0,pending_count:0})) : implementation(input, init);
}

// Catalog/mutation regression fixtures explicitly start with an administrator grant.
export const testAdminIdentity = {...testIdentity, account:{...testIdentity.account,roles:["ADMIN"]}};
export function withAdminSession(implementation: typeof fetch): typeof fetch {
 return (input,init) => ["/api/v1/auth/session","/api/v1/auth/activity"].includes(String(input))
  ? Promise.resolve(new Response(JSON.stringify(testAdminIdentity),{status:200,headers:{"Content-Type":"application/json"}}))
  : String(input) === "/api/v1/approval-notifications" ? Promise.resolve(Response.json({unread_count:0,pending_count:0})) : implementation(input,init);
}
