import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useLocation } from "react-router-dom";
import { TestRouter } from "../../test/TestRouter";
import { afterEach, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";

// Session/draft recovery fixtures represent an explicitly authorized catalog administrator.
const identity = { account: { id: "ab09850e-ef9a-4317-a000-d67465416b5b", username: "alice", display_name: "小爱", email: "alice@example.com", email_verified: false, status: "enabled", roles:["ADMIN"] }, csrf_token: "session-csrf", expires_at: "2099-09-07T08:00:00Z", idle_expires_at: "2099-09-07T00:30:00Z" };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
const failure = (code: string, status: number) => json({ error: { code, message: "safe failure", request_id: "request-1" } }, status);
function Location() { const location = useLocation(); return <output aria-label="current path">{location.pathname}{location.search}{location.hash}</output>; }
function renderWorkspace(path: string) {
 const client = new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 return render(<QueryClientProvider client={client}><TestRouter initialEntries={[path]}><ToastProvider><AppRoutes /><Location /></ToastProvider></TestRouter></QueryClientProvider>);
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
  if(path.endsWith("/activity"))return signedIn?json(identity):failure("session_invalid",401);
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

it("handles a business session 401 by hiding the workspace behind reauthentication", async () => {
 let revoked=false;
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  if(String(input).endsWith("/session"))return revoked?failure("session_invalid",401):json(identity);
  if(String(input).endsWith("-types"))return json({types:[]});
  revoked=true;
  return failure("session_invalid",401);
 }));
  renderWorkspace("/platform/query-policies");
  expect(await screen.findByRole("heading",{name:"登录本地账号"})).toBeVisible();
  expect(screen.getByLabelText("主导航")).not.toBeVisible();
  expect(screen.getByLabelText("current path")).toHaveTextContent("/platform/query-policies");
});

it("cancels pending dirty navigation on expiry and restores the guarded draft after same-account reauthentication", async () => {
 const draftPolicy = {
  code:"editable_query_v1",name:"原名称",description:"",type_code:"page_query",default_order_field:"id",
  default_order_direction:"DESC",default_page_size:20,max_page_size:200,status:"DRAFT",
  creator:identity.account.id,modifier:identity.account.id,gmt_created:"2026-09-07T00:00:00Z",gmt_modified:"2026-09-07T00:00:00Z",
 };
 let expired=false;
 let detailReads=0;
 let writes=0;
 localStorage.setItem("rcc:last-activity-report",String(Date.now()));
 vi.stubGlobal("navigator",Object.assign(Object.create(navigator),{locks:{request:(_name:string,callback:()=>Promise<unknown>)=>callback()}}));
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
  const path=String(input);
  if(path.endsWith("/session"))return expired?failure("session_invalid",401):json(identity);
  if(path.endsWith("/activity"))return expired?failure("session_invalid",401):json(identity);
  if(path.endsWith("/csrf"))return json({csrf_token:"preauth-csrf"});
  if(path.endsWith("/login")){expired=false;return json(identity);}
  if(expired)return failure("session_invalid",401);
  if(path.endsWith("/query-policy-types"))return json({types:[{code:"page_query"}]});
  if(path.endsWith("/query-policies/editable_query_v1")){
   if(init?.method){writes++;return json(draftPolicy);}
   detailReads++;return json({...draftPolicy,gmt_modified:`2026-09-07T00:00:0${detailReads}Z`});
  }
  if(path.endsWith("/query-policies"))return json({policies:[draftPolicy]});
  throw new Error(`unexpected ${path}`);
 }));
 renderWorkspace("/platform/query-policies/editable_query_v1?mode=edit");
 const user=userEvent.setup();
 const name=await screen.findByLabelText("显示名称");
 await user.clear(name);
 await user.type(name,"会话中断编辑意图");
 await user.click(screen.getByRole("link",{name:"变更规则定义"}));
 expect(screen.getByRole("alertdialog",{name:"放弃未保存的修改？"})).toBeVisible();
 expired=true;
 act(()=>window.dispatchEvent(new CustomEvent("rcc:business-session-invalid",{detail:{code:"session_invalid"}})));
 expect(await screen.findByRole("heading",{name:"登录本地账号"})).toBeVisible();
 expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
 const nativeLeave = new Event("beforeunload", { cancelable: true });
 window.dispatchEvent(nativeLeave);
 expect(nativeLeave.defaultPrevented).toBe(true);
 expect(screen.getByLabelText("主导航")).not.toBeVisible();
 await user.type(screen.getByLabelText("用户名"),"alice");
 await user.tab();
 expect(screen.getByLabelText("密码")).toHaveFocus();
 await user.type(screen.getByLabelText("密码"),"correct horse battery staple");
 await user.click(screen.getByRole("button",{name:"登录"}));
 await waitFor(()=>expect(screen.queryByRole("heading",{name:"登录本地账号"})).not.toBeInTheDocument());
 expect(screen.getByLabelText("显示名称")).toHaveValue("会话中断编辑意图");
 expect(detailReads).toBeGreaterThanOrEqual(2);
 expect(writes).toBe(0);
 expect(screen.getByLabelText("current path")).toHaveTextContent("/platform/query-policies/editable_query_v1?mode=edit");
 await user.click(screen.getByRole("button",{name:"关闭抽屉"}));
 expect(screen.getByRole("alertdialog",{name:"放弃未保存的修改？"})).toBeVisible();
});

it("clears an in-memory rule draft when focus reveals a different Cookie account", async () => {
 const bob={...identity,account:{...identity.account,id:"9e5e2b50-6aaa-4eaa-83fb-f56d84240214",username:"bob",display_name:"小博",email:"bob@example.com"},csrf_token:"bob-csrf"};
 let current=identity;
 localStorage.setItem("rcc:last-activity-report",String(Date.now()));
 vi.stubGlobal("navigator",Object.assign(Object.create(navigator),{locks:{request:(_name:string,callback:()=>Promise<unknown>)=>callback()}}));
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input);
  if(path.endsWith("/activity"))return json(current);
  if(path.endsWith("/session"))return json(current);
  if(path.endsWith("/query-policy-types"))return json({types:[{code:"page_query"}]});
  if(path.endsWith("/query-policies"))return json({policies:[]});
  throw new Error(`unexpected ${path}`);
 }));
 renderWorkspace("/platform/query-policies/new");
 const user=userEvent.setup();
 const name=await screen.findByLabelText("显示名称");
 await user.type(name,"不得跨账号显示");
 current=bob;
 act(()=>document.dispatchEvent(new Event("visibilitychange")));
 expect(await screen.findByText("小博")).toBeVisible();
 expect(screen.getByLabelText("显示名称")).toHaveValue("");
});

it("clears an in-memory rule draft when another tab ends the session", async () => {
 let signedIn=true;
 vi.stubGlobal("navigator",Object.assign(Object.create(navigator),{locks:{request:(_name:string,callback:()=>Promise<unknown>)=>callback()}}));
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input);
  if(path.endsWith("/session"))return signedIn?json(identity):failure("session_invalid",401);
  if(path.endsWith("/activity"))return signedIn?json(identity):failure("session_invalid",401);
  if(path.endsWith("/csrf"))return json({csrf_token:"preauth-csrf"});
  if(path.endsWith("/login")){signedIn=true;return json(identity);}
  if(path.endsWith("/query-policy-types"))return json({types:[{code:"page_query"}]});
  if(path.endsWith("/query-policies"))return json({policies:[]});
  throw new Error(`unexpected ${path}`);
 }));
 renderWorkspace("/platform/query-policies/new");
 const user=userEvent.setup();
 await user.type(await screen.findByLabelText("显示名称"),"跨标签退出后必须销毁");
 await user.click(screen.getByRole("link",{name:"变更规则定义"}));
 expect(screen.getByRole("alertdialog",{name:"放弃未保存的修改？"})).toBeVisible();
 signedIn=false;
 act(()=>window.dispatchEvent(new StorageEvent("storage",{key:"rcc:session-event",newValue:JSON.stringify({type:"ended",at:Date.now()})})));
 expect(await screen.findByRole("heading",{name:"登录本地账号"})).toBeVisible();
 expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
 await user.type(screen.getByLabelText("用户名"),"alice");
 await user.type(screen.getByLabelText("密码"),"correct horse battery staple");
 await user.click(screen.getByRole("button",{name:"登录"}));
 await waitFor(()=>expect(screen.getByLabelText("current path")).toHaveTextContent("/platform/query-policies/new"));
 expect(await screen.findByLabelText("显示名称")).toHaveValue("");
});

it("keeps same-account memory hidden through an authentication service failure", async () => {
 let unavailable=false;
 localStorage.setItem("rcc:last-activity-report",String(Date.now()));
 vi.stubGlobal("navigator",Object.assign(Object.create(navigator),{locks:{request:(_name:string,callback:()=>Promise<unknown>)=>callback()}}));
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input);
  if(path.endsWith("/activity"))return json(identity);
  if(path.endsWith("/session"))return unavailable?failure("auth_unavailable",503):json(identity);
  if(path.endsWith("/query-policy-types"))return json({types:[{code:"page_query"}]});
  if(path.endsWith("/query-policies"))return json({policies:[]});
  throw new Error(`unexpected ${path}`);
 }));
 renderWorkspace("/platform/query-policies/new");
 const user=userEvent.setup();
 const name=await screen.findByLabelText("显示名称");
 fireEvent.change(name,{target:{value:"服务恢复后仍在"}});
 unavailable=true;
 act(()=>document.dispatchEvent(new Event("visibilitychange")));
 expect(await screen.findByRole("alert")).toHaveTextContent("账号服务暂时不可用");
 expect(screen.getByLabelText("主导航")).not.toBeVisible();
 unavailable=false;
 await user.click(screen.getByRole("button",{name:"重新检查登录状态"}));
 await waitFor(()=>expect(screen.queryByText("账号服务暂时不可用")).not.toBeInTheDocument());
 expect(screen.getByLabelText("显示名称")).toHaveValue("服务恢复后仍在");
});

it("destroys recoverable drafts when the server identifies a disabled account", async () => {
 let disabled=false;
 localStorage.setItem("rcc:last-activity-report",String(Date.now()));
 vi.stubGlobal("navigator",Object.assign(Object.create(navigator),{locks:{request:(_name:string,callback:()=>Promise<unknown>)=>callback()}}));
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input);
  if(path.endsWith("/activity"))return disabled?failure("account_disabled",401):json(identity);
  if(path.endsWith("/session"))return disabled?failure("account_disabled",401):json(identity);
  if(path.endsWith("/query-policy-types"))return json({types:[{code:"page_query"}]});
  if(path.endsWith("/query-policies"))return json({policies:[]});
  throw new Error(`unexpected ${path}`);
 }));
 renderWorkspace("/platform/query-policies/new");
 const user=userEvent.setup();
 await user.type(await screen.findByLabelText("显示名称"),"停用后必须销毁");
  disabled=true;
 localStorage.removeItem("rcc:last-activity-report");
 fireEvent.pointerDown(document.body);
 expect(await screen.findByRole("heading",{name:"登录本地账号"})).toBeVisible();
 expect(screen.getByLabelText("current path")).toHaveTextContent("/login");
 expect(screen.queryByDisplayValue("停用后必须销毁")).not.toBeInTheDocument();
});

it("destroys an interrupted draft when the embedded login inspection discovers a disabled account", async () => {
 let sessionReads=0;
 vi.stubGlobal("navigator",Object.assign(Object.create(navigator),{locks:{request:(_name:string,callback:()=>Promise<unknown>)=>callback()}}));
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input);
  if(path.endsWith("/session"))return sessionReads++===0?json(identity):failure("account_disabled",401);
  if(path.endsWith("/query-policy-types"))return json({types:[{code:"page_query"}]});
  if(path.endsWith("/query-policies"))return json({policies:[]});
  throw new Error(`unexpected ${path}`);
 }));
 renderWorkspace("/platform/query-policies/new");
 const user=userEvent.setup();
 await user.type(await screen.findByLabelText("显示名称"),"停用检查必须销毁");
 act(()=>window.dispatchEvent(new CustomEvent("rcc:business-session-invalid",{detail:{code:"session_invalid"}})));
 await waitFor(()=>expect(screen.getByLabelText("current path")).toHaveTextContent("/login"));
 expect(screen.getByRole("heading",{name:"登录本地账号"})).toBeVisible();
 expect(screen.queryByDisplayValue("停用检查必须销毁")).not.toBeInTheDocument();
});
