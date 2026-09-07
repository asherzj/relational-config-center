import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";
import { ToastProvider } from "../../components/ui/Toast";
import { TestRouter } from "../../test/TestRouter";
import { testIdentity } from "../../test/account-session";

const id = "00000000-0000-4000-8000-000000000002";
const target = { id, username: "editor.user", display_name: "编辑者", enabled: true, roles: ["VIEWER"], version: "1" };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
afterEach(() => vi.unstubAllGlobals());
it("管理员通过界面组合分配角色，并保留并发版本和请求标识", async () => {
 const writes: RequestInit[] = [];
 vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
  const path=String(input);
  if (path.startsWith("/api/v1/auth/")) return json({ ...testIdentity, account: { ...testIdentity.account, roles:["ADMIN"] } });
  if (init?.method === "PUT") { writes.push(init); return json({...target,roles:["EDITOR","APPROVER"],version:"2"}); }
  if (path.includes("/history")) return json({events:[],next_cursor:""});
  if (path.startsWith("/api/v1/account-roles")) return json({accounts:[target],next_cursor:""});
  return json({error:{code:"test_unexpected_request",message:path}},404);
 }));
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={["/platform/account-roles"]}><AppRoutes /></TestRouter></ToastProvider></QueryClientProvider>);
 const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"管理 editor.user 的角色"}));
 await user.click(screen.getByRole("checkbox",{name:/查看者 VIEWER/}));
 await user.click(screen.getByRole("checkbox",{name:/编辑者 EDITOR/}));
 await user.click(screen.getByRole("checkbox",{name:/审批人 APPROVER/}));
 await user.click(screen.getByRole("button",{name:"保存角色"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0].body))).toEqual({roles:["EDITOR","APPROVER"],expected_version:"1"});
 expect(new Headers(writes[0].headers).get("Idempotency-Key")).toMatch(/^[a-f0-9-]{36}$/);
 expect(await screen.findByText("角色已保存，后续请求立即生效。")).toBeVisible();
});
it("查看者浏览规则时新建按钮不可用，也不能从地址直接打开编辑表单",async()=>{
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input);
  if(path.startsWith("/api/v1/auth/"))return json({...testIdentity,account:{...testIdentity.account,roles:["VIEWER"]}});
  if(path.endsWith("query-policy-types"))return json({types:[{code:"page_query"}]});
  if(path.endsWith("query-policies"))return json({policies:[]});
  return json({error:{code:"policy_not_found",message:"missing"}},404);
 }));
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
 render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={["/platform/query-policies/new"]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
 expect(await screen.findByRole("button",{name:"新建草稿"})).toBeDisabled();
 expect(screen.queryByRole("button",{name:"创建草稿"})).not.toBeInTheDocument();
 expect(screen.queryByRole("link",{name:"账号角色"})).not.toBeInTheDocument();
});

it.each(["query","mutation"])("%s 规则编辑中撤权保留输入并禁止保存",async(kind)=>{
 let roles=["ADMIN"];
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input);
  if(path.startsWith("/api/v1/auth/"))return json({...testIdentity,account:{...testIdentity.account,roles}});
  if(path.endsWith("query-policy-types"))return json({types:[{code:"page_query"}]});
  if(path.endsWith("mutation-policy-types"))return json({types:[{code:"single_table_mutation",operations:["ADD","MODIFY","DELETE"]}]});
  return json({policies:[]});
 }));
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
 render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={[`/platform/${kind}-policies/new`]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
 const user=userEvent.setup();
 await user.type(await screen.findByLabelText("显示名称"),"已填写但未提交");
 roles=["VIEWER"];act(()=>window.dispatchEvent(new Event("rcc:account-roles-changed")));
 await waitFor(()=>expect(screen.getByLabelText("显示名称")).toHaveValue("已填写但未提交"));
 await waitFor(()=>expect(screen.getByRole("button",{name:"创建草稿"})).toBeDisabled());
 expect(screen.getByLabelText("显示名称")).toBeDisabled();
});

it("角色保存收到503时使用原标识和内容重试",async()=>{
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
  const path=String(input);
  if(path.startsWith("/api/v1/auth/"))return json({...testIdentity,account:{...testIdentity.account,roles:["ADMIN"]}});
  if(init?.method==="PUT"){writes.push(init);return writes.length===1?json({error:{code:"auth_unavailable",message:"unavailable",request_id:"role-save-503"}},503):json({...target,roles:["VIEWER","EDITOR"],version:"2"});}
  if(path.includes("/history"))return json({events:[],next_cursor:""});
  return json({accounts:[target],next_cursor:""});
 }));
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={["/platform/account-roles"]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
 const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"管理 editor.user 的角色"}));
 await user.click(screen.getByRole("checkbox",{name:/编辑者 EDITOR/}));
 await user.click(screen.getByRole("button",{name:"保存角色"}));
 await user.click(await screen.findByRole("button",{name:"使用原请求重试"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(writes[1].body).toBe(writes[0].body);
 expect(new Headers(writes[1].headers).get("Idempotency-Key")).toBe(new Headers(writes[0].headers).get("Idempotency-Key"));
 expect(await screen.findByText("角色已保存，后续请求立即生效。")).toBeVisible();
});

it("角色编辑中撤权保留选择并禁止提交", async () => {
 let roles=["ADMIN"];
 const writes: RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
  const path=String(input);
  if(path.startsWith("/api/v1/auth/"))return json({...testIdentity,account:{...testIdentity.account,roles}});
  if(init?.method==="PUT")writes.push(init);
  if(path.includes("/history"))return json({events:[],next_cursor:""});
  return json({accounts:[target],next_cursor:""});
 }));
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={["/platform/account-roles"]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
 const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"管理 editor.user 的角色"}));
 await user.click(screen.getByRole("checkbox",{name:/编辑者 EDITOR/}));
 roles=["VIEWER"];act(()=>window.dispatchEvent(new Event("rcc:account-roles-changed")));
 await waitFor(()=>expect(screen.getByRole("button",{name:"保存角色"})).toBeDisabled());
 expect(screen.getByRole("checkbox",{name:/编辑者 EDITOR/})).toBeChecked();
 expect(screen.getByRole("checkbox",{name:/编辑者 EDITOR/})).toBeDisabled();
 await user.click(screen.getAllByRole("button",{name:"关闭"}).at(-1)!);
 expect(await screen.findByRole("alertdialog",{name:"放弃未保存的修改？"})).toBeVisible();
 expect(writes).toHaveLength(0);
});

it("并发冲突后的读取失败只重试读取，保留选择并使用新版本再次保存", async () => {
 const writes: RequestInit[]=[];
 let latestReads=0;
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
  const path=String(input);
  if(path.startsWith("/api/v1/auth/"))return json({...testIdentity,account:{...testIdentity.account,roles:["ADMIN"]}});
  if(init?.method==="PUT"){
   writes.push(init);
   return writes.length===1
    ? json({error:{code:"account_roles_conflict",message:"conflict",request_id:"role-conflict"}},409)
    : json({...target,roles:["VIEWER","EDITOR"],version:"3"});
  }
  if(path.includes("/history"))return json({events:[],next_cursor:""});
  if(path.includes(id)){
   latestReads++;
   if(latestReads===1)throw new TypeError("connection lost while reading");
   return json({accounts:[{...target,roles:["APPROVER"],version:"2"}],next_cursor:""});
  }
  return json({accounts:[target],next_cursor:""});
 }));
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={["/platform/account-roles"]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
 const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"管理 editor.user 的角色"}));
 await user.click(screen.getByRole("checkbox",{name:/编辑者 EDITOR/}));
 await user.click(screen.getByRole("button",{name:"保存角色"}));
 await user.click(await screen.findByRole("button",{name:"查看最新角色"}));
 await waitFor(()=>expect(screen.getByRole("button",{name:"查看最新角色"})).toBeEnabled());
 expect(screen.queryByRole("button",{name:"使用原请求重试"})).not.toBeInTheDocument();
 expect(screen.getByRole("button",{name:"保存角色"})).toBeDisabled();
 expect(screen.getByRole("checkbox",{name:/编辑者 EDITOR/})).toBeChecked();
 expect(writes).toHaveLength(1);
 await user.click(screen.getByRole("button",{name:"查看最新角色"}));
 await waitFor(()=>expect(screen.getByRole("button",{name:"保存角色"})).toBeEnabled());
 expect(screen.getByText("服务器当前角色：审批人（版本 2）")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"保存角色"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(JSON.parse(String(writes[1].body))).toEqual({roles:["VIEWER","EDITOR"],expected_version:"2"});
 expect(new Headers(writes[1].headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0].headers).get("Idempotency-Key"));
 expect(latestReads).toBe(2);
});

it.each([401,403])("不确定保存重试收到 %s 后仍保留原请求", async (status) => {
 const writes: RequestInit[]=[];
 let authenticated=true;
 vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
  const path=String(input);
  if(path.startsWith("/api/v1/auth/")){
   if(path.endsWith("/login"))authenticated=true;
   if(path.endsWith("/session")&&!authenticated)return json({error:{code:"session_invalid",message:"expired",request_id:"expired-session"}},401);
   return json({...testIdentity,account:{...testIdentity.account,roles:["ADMIN"]}});
  }
  if(init?.method==="PUT"){
   writes.push(init);
   if(writes.length===1)return json({error:{code:"auth_unavailable",message:"unavailable",request_id:"first-503"}},503);
   if(writes.length===2){if(status===401)authenticated=false;return json({error:{code:status===401?"session_invalid":"permission_denied",message:"rejected",request_id:"retry-auth"}},status);}
   return json({...target,roles:["VIEWER","EDITOR"],version:"2"});
  }
  if(path.includes("/history"))return json({events:[],next_cursor:""});
  return json({accounts:[target],next_cursor:""});
 }));
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={["/platform/account-roles"]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
 const user=userEvent.setup();
 await user.click(await screen.findByRole("button",{name:"管理 editor.user 的角色"}));
 await user.click(screen.getByRole("checkbox",{name:/编辑者 EDITOR/}));
 await user.click(screen.getByRole("button",{name:"保存角色"}));
 await user.click(await screen.findByRole("button",{name:"使用原请求重试"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 if(status===401){
  await user.type(await screen.findByLabelText("用户名"),"test.user");
  await user.type(screen.getByLabelText("密码"),"test password long enough");
  await user.click(screen.getByRole("button",{name:"登录"}));
  await waitFor(()=>expect(screen.queryByRole("heading",{name:"登录本地账号"})).not.toBeInTheDocument());
 }
 await user.click(await screen.findByRole("button",{name:"使用原请求重试"}));
 await waitFor(()=>expect(writes).toHaveLength(3));
 for(const write of writes.slice(1)){
  expect(write.body).toBe(writes[0].body);
  expect(new Headers(write.headers).get("Idempotency-Key")).toBe(new Headers(writes[0].headers).get("Idempotency-Key"));
 }
 expect(await screen.findByText("角色已保存，后续请求立即生效。")).toBeVisible();
});
