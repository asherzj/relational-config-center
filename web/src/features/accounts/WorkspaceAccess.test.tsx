import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";

const identity = { account: { id: "ab09850e-ef9a-4317-a000-d67465416b5b", username: "alice", display_name: "小爱", email: "alice@example.com", email_verified: false, status: "enabled" }, csrf_token: "session-csrf", expires_at: "2099-09-07T08:00:00Z", idle_expires_at: "2099-09-07T00:30:00Z" };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
const failure = (code: string, status: number) => json({ error: { code, message: "safe failure", request_id: "request-1" } }, status);
function Location() { const location = useLocation(); return <output aria-label="current path">{location.pathname}{location.search}{location.hash}</output>; }
function renderWorkspace(path: string) {
 const client = new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 return render(<QueryClientProvider client={client}><MemoryRouter initialEntries={[path]}><ToastProvider><AppRoutes /><Location /></ToastProvider></MemoryRouter></QueryClientProvider>);
}
afterEach(()=>{vi.unstubAllGlobals();localStorage.clear();});

it("requires login before showing the workspace and remembers its internal destination", async () => {
 let businessReads=0;
 vi.stubGlobal("fetch",vi.fn(async (input:RequestInfo|URL)=>{
  if(String(input).endsWith("/session")) return failure("session_invalid",401);
  businessReads++;
  return json({policies:[]});
 }));
 renderWorkspace("/platform/query-policies?view=drafts#selected");
 expect(await screen.findByRole("heading",{name:"登录本地账号"})).toBeVisible();
 expect(screen.getByRole("link",{name:"注册新账号"})).toBeVisible();
 expect(screen.queryByLabelText("主导航")).not.toBeInTheDocument();
 expect(screen.getByLabelText("current path")).toHaveTextContent("/login?returnTo=%2Fplatform%2Fquery-policies%3Fview%3Ddrafts%23selected");
 expect(businessReads).toBe(0);
});

it.each([
 ["/configuration/managed-data?table=items#row", "/configuration/managed-data?table=items#row", "login"],
 ["/platform/mutation-policies", "/platform/mutation-policies", "register"],
 ["https://evil.example/steal", "/platform/query-policies", "login"],
 ["//evil.example/steal", "/platform/query-policies", "register"],
 ["/\\evil.example/steal", "/platform/query-policies", "login"],
])("returns from %s to an allowed workspace destination after %s", async (destination, expected, mode) => {
 let signedIn=false;
 vi.stubGlobal("navigator",Object.assign(Object.create(navigator),{locks:{request:(_name:string,callback:()=>Promise<unknown>)=>callback()}}));
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input);
  if(path.endsWith("/session"))return signedIn?json(identity):failure("session_invalid",401);
  if(path.endsWith("/csrf"))return json({csrf_token:"preauth-csrf"});
  if(path.endsWith("/login")||path.endsWith("/register")){signedIn=true;return json(identity,path.endsWith("/register")?201:200);}
  if(path.endsWith("-types"))return json({types:[]});
  if(path.endsWith("database-tables"))return json({tables:[]});
  return json({policies:[]});
 }));
 renderWorkspace(`/${mode}?returnTo=${encodeURIComponent(destination)}`);
 const user=userEvent.setup();
 await screen.findByRole("button",{name:mode==="register"?"注册并登录":"登录"});
 await user.type(screen.getByLabelText("用户名"),"alice");
 if(mode==="register")await user.type(screen.getByLabelText("邮箱"),"alice@example.com");
 await user.type(screen.getByLabelText("密码"),"correct horse battery staple");
 await user.click(screen.getByRole("button",{name:mode==="register"?"注册并登录":"登录"}));
 await waitFor(()=>expect(screen.getByLabelText("current path")).toHaveTextContent(expected));
 expect(await screen.findByLabelText("主导航")).toBeVisible();
 expect(screen.getByText("小爱")).toBeVisible();
});

it("keeps an authenticated rule rejection on the workspace instead of redirecting to login", async () => {
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  if(String(input).endsWith("/session"))return json(identity);
  if(String(input).endsWith("-types"))return json({types:[]});
  return failure("mutation_not_allowed",403);
 }));
 renderWorkspace("/platform/query-policies");
 expect(await screen.findByRole("alert")).toHaveTextContent("当前变更规则不允许该写入操作");
 expect(screen.getByLabelText("current path")).toHaveTextContent("/platform/query-policies");
 expect(screen.getByLabelText("主导航")).toBeVisible();
 expect(screen.queryByRole("heading",{name:"登录本地账号"})).not.toBeInTheDocument();
});

it("preserves a service failure as a retryable session check", async () => {
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>String(input).endsWith("/session")?failure("auth_unavailable",503):json({policies:[]})));
 renderWorkspace("/platform/query-policies");
 expect(await screen.findByRole("alert")).toHaveTextContent("原登录凭据已保留");
 expect(screen.getByLabelText("current path")).toHaveTextContent("/platform/query-policies");
 expect(screen.queryByLabelText("主导航")).not.toBeInTheDocument();
});

it("handles a business session 401 by hiding the workspace and returning to login", async () => {
 let revoked=false;
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  if(String(input).endsWith("/session"))return revoked?failure("session_invalid",401):json(identity);
  if(String(input).endsWith("-types"))return json({types:[]});
  revoked=true;
  return failure("session_invalid",401);
 }));
 renderWorkspace("/platform/query-policies");
 expect(await screen.findByRole("heading",{name:"登录本地账号"})).toBeVisible();
 expect(screen.queryByLabelText("主导航")).not.toBeInTheDocument();
 expect(screen.getByLabelText("current path")).toHaveTextContent("/login?returnTo=%2Fplatform%2Fquery-policies");
});
