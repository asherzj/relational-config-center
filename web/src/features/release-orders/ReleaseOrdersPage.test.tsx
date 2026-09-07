import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach,expect,it,vi} from "vitest";
import {AppRoutes} from "../../app";
import {ToastProvider} from "../../components/ui/Toast";
import {TestRouter} from "../../test/TestRouter";
import {testAdminIdentity,withAdminSession} from "../../test/account-session";

const id="12345678123456781234567812345678";
const order={id,table_name:"items",applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-07T08:00:00Z",updated_at:"2026-09-07T08:00:00Z",history:[{action:"CREATE",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-07T08:00:00Z",reason:""}],allowed_actions:["edit","cancel"],items:[{operation:"MODIFY",id:"1",expected_record_version:"0",before:{id:"1",label:"original"},content:{label:"proposal"},fields:[{name:"id",type:"uint64",nullable:false,editable:false,before_state:"value",before:"1",proposed_state:"omitted",proposed:null},{name:"label",type:"string",nullable:true,editable:true,before_state:"value",before:"original",proposed_state:"value",proposed:"proposal"}]}]};
const json=(value:unknown,status=200)=>new Response(JSON.stringify(value),{status,headers:{"Content-Type":"application/json"}});
function mount(path="/configuration/release-orders"){
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 return render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={[path]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
}
afterEach(()=>{vi.unstubAllGlobals();sessionStorage.clear()});
it("从列表打开持久草稿，并取消后保留历史",async()=>{
 let current=structuredClone(order);const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/cancel")){writes.push(init!);current={...current,state:"CANCELLED",version:"2",allowed_actions:[]};return json(current)}
  if(path===`/api/v1/release-orders/${id}`)return json(current);
  if(path.startsWith("/api/v1/release-orders"))return json({orders:[current],next_cursor:""});
  return json({policies:[]});
 })));
 const user=userEvent.setup();mount();
 await user.click(await screen.findByRole("link",{name:id}));
 expect(await screen.findByText("proposal")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"取消草稿"}));
 await user.type(screen.getByLabelText("取消原因"),"调整计划");
 await user.click(screen.getByRole("button",{name:"确认取消草稿"}));
 expect(await screen.findByRole("heading",{name:"items · 已取消"})).toBeVisible();
 expect(screen.queryByRole("button",{name:"编辑草稿"})).not.toBeInTheDocument();
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0].body))).toEqual({expected_version:"1",reason:"调整计划"});
 expect(new Headers(writes[0].headers).get("Idempotency-Key")).toBeTruthy();
});

it("编辑冲突保留输入，读取最新后必须明确重建再保存",async()=>{
 let current=structuredClone(order);const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(init?.method==="PUT"){
   writes.push(init);
   if(writes.length===1){current={...current,version:"2"};return json({error:{code:"release_version_conflict",message:"changed",request_id:"conflict"}},409)}
   current={...current,version:"3"};return json(current);
  }
  if(String(input)===`/api/v1/release-orders/${id}`)return json(current);
  return json({orders:[current],next_cursor:""});
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.clear(screen.getByLabelText("label 申请值"));await user.type(screen.getByLabelText("label 申请值"),"my retained proposal");
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 expect(await screen.findByText(/发布单已被其他窗口修改/)).toBeVisible();
 expect(screen.getByLabelText("label 申请值")).toHaveValue("my retained proposal");
 expect(screen.getByRole("button",{name:"保存草稿修改"})).toBeDisabled();
 await user.click(screen.getByRole("button",{name:"查看最新发布单"}));
 expect(await screen.findByRole("button",{name:"基于最新发布单重建"})).toBeEnabled();
 expect(writes).toHaveLength(1);
 await user.click(screen.getByRole("button",{name:"基于最新发布单重建"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(JSON.parse(String(writes[1].body))).toMatchObject({expected_version:"2",items:[{content:{label:"my retained proposal"}}]});
});

it("刷新后恢复未知请求，随后403也不丢弃原标识或跨账号重放",async()=>{
 let account={...testAdminIdentity.account};let attempts=0;const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account});
  if(init?.method==="PUT"){
   writes.push(init);attempts++;
   if(attempts===1)throw new TypeError("response lost");
   if(attempts===2)return json({error:{code:"permission_denied",message:"role revoked",request_id:"revoked"}},403);
   return json({...order,version:"2"});
  }
  if(String(input)===`/api/v1/release-orders/${id}`)return json(order);
  return json({orders:[order],next_cursor:""});
 }));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 expect(await screen.findByRole("button",{name:"使用原请求重试"})).toBeVisible();
 page.unmount();account={...account,id:"00000000-0000-4000-8000-000000000099"};page=mount();
 await screen.findByRole("heading",{name:"发布单"});expect(screen.queryByRole("button",{name:"恢复原发布请求"})).not.toBeInTheDocument();expect(writes).toHaveLength(1);
 page.unmount();account={...testAdminIdentity.account};page=mount();
 await user.click(await screen.findByRole("button",{name:"恢复原发布请求"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 await waitFor(()=>expect(screen.getByRole("button",{name:"恢复原发布请求"})).toBeEnabled());
 await user.click(screen.getByRole("button",{name:"恢复原发布请求"}));
 await waitFor(()=>expect(writes).toHaveLength(3));
 for(const next of writes.slice(1)){expect(next.body).toBe(writes[0].body);expect(new Headers(next.headers).get("Idempotency-Key")).toBe(new Headers(writes[0].headers).get("Idempotency-Key"))}
 expect(await screen.findByRole("heading",{name:"items · 草稿"})).toBeVisible();
});

it("已知 ADD 冲突后查看缺行墓碑并明确重建，保留当前申请值",async()=>{
 const add={...order,items:[{...order.items[0]!,operation:"ADD",before:null,content:{id:"1",label:"proposal"},fields:order.items[0]!.fields.map(field=>({...field,editable:true,before_state:"absent",before:null}))}]};
 const writes:RequestInit[]=[];let previews=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/preview")){previews++;return json({table_name:"items",items:[{...add.items[0]!,expected_record_version:"2"}]})}
  if(init?.method==="PUT"){writes.push(init);return writes.length===1?json({error:{code:"record_version_conflict",message:"stale record",request_id:"record-conflict"}},409):json({...add,version:"2"})}
  return json(add);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await user.click(await screen.findByRole("button",{name:"查看最新配置"}));
 await screen.findByText(/记录基线 2/);
 await user.selectOptions(screen.getByLabelText("id 提交方式"),"value");
 await user.clear(screen.getByLabelText("id 申请值"));await user.type(screen.getByLabelText("id 申请值"),"1");
 expect(screen.queryByRole("button",{name:"基于最新配置重建"})).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"查看最新配置"}));await screen.findByText(/记录基线 2/);

 expect(screen.getByRole("button",{name:"保存草稿修改"})).toBeDisabled();
 await user.click(screen.getByRole("button",{name:"基于最新配置重建"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(2));expect(previews).toBe(2);
 expect(JSON.parse(String(writes[1].body))).toMatchObject({expected_version:"1",items:[{operation:"ADD",expected_record_version:"2",content:{id:"1",label:"proposal"}}]});
});

it("未知请求重试得到明确版本冲突后，可核对并用新请求重建",async()=>{
 let current=structuredClone(order);const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(init?.method==="PUT"){
   writes.push(init);
   if(writes.length===1)throw new TypeError("response lost before commit");
   if(writes.length===2){current={...current,version:"2"};return json({error:{code:"release_version_conflict",message:"changed",request_id:"conflict"}},409)}
   return json({...current,version:"3"});
  }
  if(String(input)===`/api/v1/release-orders/${id}`)return json(current);
  return json({orders:[current],next_cursor:""});
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.clear(screen.getByLabelText("label 申请值"));await user.type(screen.getByLabelText("label 申请值"),"retained after unknown");
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await user.click(await screen.findByRole("button",{name:"使用原请求重试"}));
 expect(await screen.findByText(/发布单已被其他窗口修改/)).toBeVisible();
 expect(screen.getByLabelText("label 申请值")).toBeEnabled();
 expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 await user.click(screen.getByRole("button",{name:"查看最新发布单"}));
 await user.click(await screen.findByRole("button",{name:"基于最新发布单重建"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(3));
 expect(JSON.parse(String(writes[2]!.body))).toMatchObject({expected_version:"2",items:[{content:{label:"retained after unknown"}}]});
 expect(new Headers(writes[2]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it("未知请求明确被取消状态拒绝后，保留输入并只读查看终态",async()=>{
 let current=structuredClone(order);let attempts=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(_input,init)=>{
  if(init?.method==="PUT"){
   attempts++;
   if(attempts===1)throw new TypeError("response lost before commit");
   current={...current,state:"CANCELLED",version:"2",allowed_actions:[]};
   return json({error:{code:"release_state_invalid",message:"cancelled",request_id:"state"}},422);
  }
  return json(current);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await user.click(await screen.findByRole("button",{name:"使用原请求重试"}));
 await user.click(await screen.findByRole("button",{name:"查看最新发布单"}));
 expect(await screen.findByText(/状态：CANCELLED/)).toBeVisible();
 expect(screen.getByRole("button",{name:"基于最新发布单重建"})).toBeDisabled();
 expect(screen.getByLabelText("label 申请值")).toHaveValue("proposal");
 expect(sessionStorage.getItem(`rcc:release-requests:${testAdminIdentity.account.id}`)).toContain("release_state_invalid");
 expect(attempts).toBe(2);
});

it("刷新后原请求被明确拒绝仍保留申请，核对后才能确认重建",async()=>{
 let current=structuredClone(order);const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/preview"))return json({table_name:"items",items:current.items});
  if(init?.method==="PUT"){
   writes.push(init);
   if(writes.length===1)throw new TypeError("response lost before commit");
   if(writes.length===2){current={...current,version:"2"};return json({error:{code:"release_version_conflict",message:"changed",request_id:"conflict"}},409)}
   return json({...current,version:"3",items:[{...current.items[0]!,content:{label:"unique recovered intent"}}]});
  }
  if(String(input)===`/api/v1/release-orders/${id}`)return json(current);
  return json({orders:[current],next_cursor:""});
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.clear(screen.getByLabelText("label 申请值"));await user.type(screen.getByLabelText("label 申请值"),"unique recovered intent");
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await screen.findByRole("button",{name:"使用原请求重试"});page.unmount();page=mount();
 await user.click(await screen.findByRole("button",{name:"恢复原发布请求"}));
 await user.click(await screen.findByText("查看原申请内容"));
 expect(await screen.findByText("值：unique recovered intent")).toBeVisible();
 expect(sessionStorage.getItem(`rcc:release-requests:${testAdminIdentity.account.id}`)).toContain("unique recovered intent");
 page.unmount();page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(sessionStorage.getItem(`rcc:release-requests:${testAdminIdentity.account.id}`)).toContain("unique recovered intent");
 page.unmount();mount();
 await user.click(await screen.findByRole("button",{name:"查看最新状态与配置"}));
 const rebuild=await screen.findByRole("button",{name:"确认重建并保存草稿"});expect(rebuild).toBeEnabled();expect(writes).toHaveLength(2);
 await user.click(rebuild);await waitFor(()=>expect(writes).toHaveLength(3));
 expect(JSON.parse(String(writes[2]!.body))).toMatchObject({expected_version:"2",items:[{content:{label:"unique recovered intent"}}]});
 expect(new Headers(writes[2]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it("原业务结果重放成功后重新读取已挂载详情的当前版本",async()=>{
 let attempts=0;const latest={...order,version:"3",items:[{...order.items[0]!,content:{label:"latest committed"},fields:order.items[0]!.fields.map(field=>field.name==="label"?{...field,proposed:"latest committed"}:field)}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(init?.method==="PUT"){attempts++;if(attempts===1)throw new TypeError("lost");return json({...order,version:"2"})}
  if(String(input)===`/api/v1/release-orders/${id}`)return json(attempts?latest:order);
  return json({orders:[],next_cursor:""});
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await user.click(await screen.findByRole("button",{name:"使用原请求重试"}));
 expect(await screen.findByText("发布单版本：3")).toBeVisible();
 expect(await screen.findByText("latest committed")).toBeVisible();
});

it.each([[403,1],[404,1],[503,2]])("发布单GET %s遵循有限重试规则",async(status,expected)=>{
 let reads=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async()=>{reads++;return json({error:{code:status===503?"release_unavailable":"release_not_found",message:"read rejected",request_id:"read"}},status)})));
 const client=new QueryClient({defaultOptions:{queries:{retryDelay:0},mutations:{retry:false}}});
 render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={[`/configuration/release-orders/${id}`]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
 await screen.findByRole("alert");expect(reads).toBe(expected);
});

it("取消请求刷新恢复冲突后，明确显示真实取消动作并保留原因",async()=>{
 let current=structuredClone(order);const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/cancel")){
   writes.push(init!);
   if(writes.length===1)throw new TypeError("lost");
   if(writes.length===2){current={...current,version:"2"};return json({error:{code:"release_version_conflict",message:"changed",request_id:"conflict"}},409)}
   current={...current,state:"CANCELLED",version:"3",allowed_actions:[]};return json(current);
  }
  if(String(input)===`/api/v1/release-orders/${id}`)return json(current);
  return json({orders:[current],next_cursor:""});
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"取消草稿"}));
 await user.type(screen.getByLabelText("取消原因"),"retained cancellation");
 await user.click(screen.getByRole("button",{name:"确认取消草稿"}));
 await screen.findByRole("button",{name:"使用原请求重试"});page.unmount();page=mount();
 await user.click(await screen.findByRole("button",{name:"恢复原发布请求"}));
 await user.click(await screen.findByRole("button",{name:"查看最新状态与配置"}));
 const cancel=await screen.findByRole("button",{name:"确认按最新状态取消草稿"});
 expect(screen.queryByRole("button",{name:"确认重建并保存草稿"})).not.toBeInTheDocument();
 await user.click(cancel);await waitFor(()=>expect(writes).toHaveLength(3));
 expect(JSON.parse(String(writes[2]!.body))).toEqual({expected_version:"2",reason:"retained cancellation"});
 expect(await screen.findByRole("heading",{name:"items · 已取消"})).toBeVisible();
});

it("仅有审批角色的人填写意见批准冻结单据",async()=>{
 let current={...order,applicant_id:"other-applicant",state:"PENDING_APPROVAL",version:"2",allowed_actions:["approve","reject"],frozen_digest:"a".repeat(64)};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:["APPROVER"]}});
  if(String(input).endsWith("/approve")){writes.push(init!);current={...current,state:"APPROVED",version:"3",allowed_actions:[]};return json(current)}
  if(String(input)===`/api/v1/release-orders/${id}`)return json(current);
  return json({orders:[current],next_cursor:""});
 }));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"批准发布单"}));
 expect(screen.getByRole("button",{name:"确认批准"})).toBeDisabled();
 await user.type(screen.getByLabelText("审批意见"),"已核对变更范围");
 await user.click(screen.getByRole("button",{name:"确认批准"}));
 expect(await screen.findByRole("heading",{name:"items · 已批准"})).toBeVisible();
 expect(writes).toHaveLength(1);expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"2",reason:"已核对变更范围"});
});

it("审批状态冲突跨刷新保留原意见，查看最新后才显式重建",async()=>{
 let current={...order,applicant_id:"other-applicant",state:"PENDING_APPROVAL",version:"2",allowed_actions:["approve","reject"]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:["APPROVER"]}});
  if(String(input).endsWith("/approve")){writes.push(init!);if(writes.length===1){current={...current,version:"3"};return json({error:{code:"release_version_conflict",message:"changed",request_id:"cas"}},409)}current={...current,state:"APPROVED",version:"4",allowed_actions:[]};return json(current)}
  if(String(input)===`/api/v1/release-orders/${id}`)return json(current);return json({orders:[current],next_cursor:""});
 }));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"批准发布单"}));await user.type(screen.getByLabelText("审批意见"),"保留这条意见");await user.click(screen.getByRole("button",{name:"确认批准"}));
 await waitFor(()=>expect(writes).toHaveLength(1));page.unmount();page=mount();
 await user.click(await screen.findByText("查看原申请内容"));expect(await screen.findByText("审批意见：保留这条意见")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"查看最新状态与配置"}));await user.click(await screen.findByRole("button",{name:"确认按最新状态批准发布单"}));
 expect(await screen.findByRole("heading",{name:"items · 已批准"})).toBeVisible();expect(writes).toHaveLength(2);
 expect(JSON.parse(String(writes[1]!.body))).toEqual({expected_version:"3",reason:"保留这条意见"});expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it("仅 PUBLISHER 执行原审批，丢响应后跨刷新使用原键确认并显示最终值",async()=>{
 const identity={...testAdminIdentity,account:{...testAdminIdentity.account,roles:["PUBLISHER"]}};
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["execute"]};
 const final={...approved,state:"SUCCEEDED",version:"4",allowed_actions:[],publication:{table_version:"7",publisher_id:identity.account.id,executed_at:"2026-09-08T01:00:00Z",notification:{id:"notice",table_version:"7",status:"NOT_CONNECTED"},commands:[{order_id:id,sequence:"9",table_name:"items",table_version:"7",operation:"MODIFY",id:"1",record_version:"2",before:{format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"label",type:"varchar(40)",encoding:"text",value:"original"}],checksum:"b".repeat(64)},final:{format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"label",type:"varchar(40)",encoding:"text",value:"actual database value"},{name:"empty",type:"text",encoding:"text",value:""},{name:"nil",type:"json",encoding:"sql_null",value:null},{name:"json",type:"json",encoding:"json",value:"null"}],checksum:"c".repeat(64)}}]}};
 let current:typeof approved|typeof final=approved;
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/auth/session")||path.endsWith("/auth/activity"))return json(identity);
  if(path.endsWith("/execute")){writes.push(init!);current=final;if(writes.length===1)throw new TypeError("lost response");return json(final)}
  if(path===`/api/v1/release-orders/${id}`)return json(current);
  return json({orders:[current],next_cursor:""});
 }));
 const user=userEvent.setup();const first=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"执行发布"}));
 await user.click(screen.getByRole("button",{name:"确认发布到数据库"}));
 expect(await screen.findByText(/结果待确认。原请求与意见已保留/)).toBeVisible();
 first.unmount();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"恢复原发布请求"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(writes[0]!.body).toBe('{"expected_version":"3"}');expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 expect(await screen.findByRole("heading",{name:"items · 已发布"})).toBeVisible();
 expect(screen.getByText("值：actual database value")).toBeVisible();expect(screen.getByText("SQL NULL")).toBeVisible();expect(screen.getByText("JSON：null")).toBeVisible();
 expect(screen.getByText("刷新通知：notice · 分发尚未接入")).toBeVisible();expect(sessionStorage.length).toBe(0);
});
