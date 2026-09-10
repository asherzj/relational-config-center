import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {act,fireEvent,render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach,expect,it,vi} from "vitest";
import {AppRoutes} from "../../app";
import {ToastProvider} from "../../components/ui/Toast";
import {TestRouter} from "../../test/TestRouter";
import {testAdminIdentity,withAdminSession} from "../../test/account-session";
import {defaultFieldPolicies} from "../../test/field-policy-fixture";

const id="12345678123456781234567812345678";
const rollbackID="87654321876543218765432187654321";
const repreparedID="abcdefabcdefabcdefabcdefabcdefab";
const order={id,title:"更新渠道展示名称",table_name:"items",applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-07T08:00:00Z",updated_at:"2026-09-07T08:00:00Z",history:[{action:"CREATE",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-07T08:00:00Z",reason:""}],allowed_actions:["edit","cancel"],items:[{operation:"MODIFY",id:"1",expected_record_version:"0",before:{id:"1",label:"original"},content:{label:"proposal"},fields:[{name:"id",type:"uint64",nullable:false,editable:false,before_state:"value",before:"1",proposed_state:"omitted",proposed:null},{name:"label",type:"string",nullable:true,editable:true,before_state:"value",before:"original",proposed_state:"value",proposed:"proposal"}]}]};
const json=(value:unknown,status=200)=>new Response(JSON.stringify(value, (key,item)=>key==="orders"?item.map((order:{items:unknown[]})=>({...order,item_count:order.items.length,operation_counts:{MODIFY:order.items.length}})):item),{status,headers:{"Content-Type":"application/json"}});
let fieldPolicyResponse=()=>json(defaultFieldPolicies("items",[{name:"id",type:"uint64",nullable:false},{name:"label",type:"string",nullable:true}]));
function mount(path="/configuration/release-orders"){
 const request=globalThis.fetch;
 vi.stubGlobal("fetch",(input:RequestInfo|URL,init?:RequestInit)=>String(input).includes("/table-field-policies/")?Promise.resolve(fieldPolicyResponse()):request(input,init));
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 return {...render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={[path]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>),client};
}
afterEach(()=>{vi.unstubAllGlobals();sessionStorage.clear();fieldPolicyResponse=()=>json(defaultFieldPolicies("items",[{name:"id",type:"uint64",nullable:false},{name:"label",type:"string",nullable:true}]))});
it("原申请人或管理员核对最新配置后原子化重新准备已批准普通单",async()=>{
 const approved={...order,applicant_id:"original-applicant",state:"APPROVED",version:"3",allowed_actions:["execute","reprepare"],frozen_digest:"a".repeat(64)};
 const freshItems=approved.items.map(item=>({...item,expected_record_version:"2",before:{id:"1",label:"latest database value"},fields:item.fields.map(field=>field.name==="label"?{...field,before:"latest database value"}:field)}));
 const draft={...order,id:repreparedID,applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",allowed_actions:["edit","submit","cancel"],copied_from_id:id,items:freshItems,history:[{action:"REPREPARE",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-09T01:00:00Z",reason:"",related_order_id:id}]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/preview"))return json({table_name:"items",items:freshItems});
  if(path.endsWith("/reprepare")){writes.push(init!);return json(draft,201)}
  if(path.endsWith("/people"))return json({people:{}});
  if(path===`/api/v1/release-orders/${repreparedID}`)return json(draft);
  return json(approved);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"重新准备"}));
 expect(screen.getByRole("button",{name:"继续重新准备"})).toBeDisabled();
 expect(screen.getByText(/旧单会取消并释放目标/)).toBeVisible();
 await user.click(screen.getByRole("button",{name:"读取最新配置"}));
 expect(await screen.findByText("latest database value")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"继续重新准备"}));
 const confirmation=await screen.findByRole("alertdialog",{name:"取消旧单并创建新草稿？"});
 const cancel=screen.getByRole("button",{name:"取消"});
 const confirm=screen.getByRole("button",{name:"取消旧单并创建新草稿"});
 expect(cancel).toHaveFocus();expect(confirm).toHaveAttribute("data-variant","destructive");
 await user.click(cancel);expect(confirmation).not.toBeInTheDocument();expect(writes).toHaveLength(0);
 await user.click(screen.getByRole("button",{name:"继续重新准备"}));
 await user.click(screen.getByRole("button",{name:"取消旧单并创建新草稿"}));
 expect(await screen.findByText("重新准备自",{exact:false})).toBeVisible();
 expect(screen.getByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"3",confirmed:true,items:[{operation:"MODIFY",id:"1",expected_record_version:"2",content:{label:"proposal"}}]});
 expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBeTruthy();
});

it("重新准备响应丢失后跨刷新保留原正文与幂等键并恢复同一草稿",async()=>{
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["reprepare"],frozen_digest:"a".repeat(64)};
 const draft={...order,id:repreparedID,state:"DRAFT",version:"1",allowed_actions:["edit","submit","cancel"],copied_from_id:id,history:[{action:"REPREPARE",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-09T01:00:00Z",reason:"",related_order_id:id}]};
 const writes:RequestInit[]=[];let attempts=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/preview"))return json({table_name:"items",items:order.items});
  if(path.endsWith("/reprepare")){writes.push(init!);attempts++;if(attempts===1)throw new TypeError("lost response");return json(draft,201)}
  if(path.endsWith("/people"))return json({people:{}});
  if(path===`/api/v1/release-orders/${repreparedID}`)return json(draft);
  return json(approved);
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"重新准备"}));
 await user.click(screen.getByRole("button",{name:"读取最新配置"}));
 await user.click(await screen.findByRole("button",{name:"继续重新准备"}));
 await user.click(await screen.findByRole("button",{name:"取消旧单并创建新草稿"}));
 await screen.findByRole("button",{name:"使用原请求重试"});page.unmount();page=mount();
 await user.click(await screen.findByText("查看原申请内容"));
 expect(await screen.findByText(`重新准备原单 ${id}`)).toBeVisible();
 await user.click(screen.getByRole("button",{name:"恢复原发布请求"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect(writes).toHaveLength(2);expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 expect(sessionStorage.length).toBe(0);
});
it("详情同时展示标题、相关人员当前姓名、首字头像和永久 ID",async()=>{
 const reviewerID="11111111-2222-4333-8444-555555555555";
 const current={...order,history:[...order.history,{action:"APPROVE",actor_id:reviewerID,version:"2",at:"2026-09-08T08:00:00Z",reason:"已核对"}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{[order.applicant_id]:"申请人阿青",[reviewerID]:"审批人小白"}}):json(current))));
 mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect((await screen.findAllByText("申请人阿青")).length).toBeGreaterThan(0);
 expect(screen.getAllByText("审批人小白")[0]).toBeVisible();
 expect(screen.getAllByText("申").length).toBeGreaterThan(0);
 expect(screen.getAllByText("审")[0]).toBeVisible();
 expect(screen.getAllByText(order.applicant_id).length).toBeGreaterThan(0);
 expect(screen.getAllByText(reviewerID)[0]).toBeVisible();
});
it("人员姓名重读失败时清除缓存姓名、显示错误标识并允许独立重试",async()=>{
 let peopleReads=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>{
  if(!String(input).endsWith("/people"))return json(order);
  peopleReads++;
  if(peopleReads===2)return json({error:{code:"release_unavailable",message:"down",request_id:"people-req-2"}},500);
  return json({people:{[order.applicant_id]:peopleReads===1?"缓存申请人":"重试后的申请人"}});
 })));
 const user=userEvent.setup();const {client}=mount(`/configuration/release-orders/${id}`);
 expect((await screen.findAllByText("缓存申请人")).length).toBeGreaterThan(0);
 await act(async()=>{await client.refetchQueries({queryKey:["release-order-people",id]})});
 expect(await screen.findByText("人员姓名读取失败，当前仅显示永久账号 ID。")).toBeVisible();
 expect(screen.getByText("错误代码：release_unavailable")).toBeVisible();
 expect(screen.getByText("请求编号：people-req-2")).toBeVisible();
 expect(screen.queryAllByText("缓存申请人")).toHaveLength(0);
 expect(screen.getAllByText(order.applicant_id).length).toBeGreaterThan(0);
 await user.click(screen.getByRole("button",{name:"重新读取人员姓名"}));
 expect((await screen.findAllByText("重试后的申请人")).length).toBeGreaterThan(0);
 expect(screen.queryByText("人员姓名读取失败，当前仅显示永久账号 ID。")).not.toBeInTheDocument();
});
it("发布者明确确认完结，取消无写入且不要求意见",async()=>{
 let current={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete"]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/complete")){writes.push(init!);current={...current,state:"COMPLETED",version:"5",allowed_actions:["rollback"]}}
  return json(current);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"完结发布单"}));
 expect(screen.getByText(/释放全部目标记录的占用/)).toBeVisible();
 expect(screen.getByText(/关闭快速回滚/)).toBeVisible();
 expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
 await user.click(screen.getAllByRole("button",{name:/^关闭$/}).at(-1)!);expect(writes).toHaveLength(0);
 await user.click(screen.getByRole("button",{name:"完结发布单"}));
 await user.click(screen.getByRole("button",{name:"确认完结"}));
 expect(await screen.findByText("items · 已完结")).toBeVisible();
 expect(writes).toHaveLength(1);expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"4"});
 expect(await screen.findByRole("button",{name:"申请回滚"})).toBeVisible();
});

it("编辑者从已发布详情说明原因并创建关联回滚草稿",async()=>{
 const original={...order,state:"COMPLETED",version:"5",allowed_actions:["rollback"],rollback_pending:false};
 const reverse={...order,id:rollbackID,title:"回滚：更新渠道展示名称",state:"DRAFT",version:"1",allowed_actions:["submit","cancel"],rollback_of_id:id,rollback_pending:false,history:[{action:"ROLLBACK",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-08T08:00:00Z",reason:"恢复误发布配置",related_order_id:id}]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/rollback")){writes.push(init!);return json(reverse,201)}
  if(String(input)===`/api/v1/release-orders/${rollbackID}`)return json(reverse);
  return json(original);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"申请回滚"}));
 expect(screen.getByRole("button",{name:"创建回滚草稿"})).toBeDisabled();
 await user.type(screen.getByLabelText("回滚原因"),"恢复误发布配置");
 await user.click(screen.getByRole("button",{name:"创建回滚草稿"}));
 expect(await screen.findByRole("heading",{name:"回滚：更新渠道展示名称"})).toBeVisible();
 expect(screen.getAllByRole("link",{name:id})).toHaveLength(2);
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"5",reason:"恢复误发布配置"});
 expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBeTruthy();
});

it("回滚原因按 UTF-8 的 2000 bytes 上限在发送前校验",async()=>{
 const original={...order,state:"COMPLETED",version:"5",allowed_actions:["rollback"],rollback_pending:false};let writes=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(_input,init)=>{if(init?.method==="POST")writes++;return json(original)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"申请回滚"}));
 const reason=screen.getByLabelText("回滚原因");
 fireEvent.change(reason,{target:{value:"回".repeat(667)}});
 expect(screen.getByText("2001 / 2000 bytes")).toBeVisible();expect(screen.getByRole("button",{name:"创建回滚草稿"})).toBeDisabled();expect(writes).toBe(0);
 fireEvent.change(reason,{target:{value:"回".repeat(666)}});
 expect(screen.getByText("1998 / 2000 bytes")).toBeVisible();expect(screen.getByRole("button",{name:"创建回滚草稿"})).toBeEnabled();
});

it("回滚申请丢响应后跨刷新按账号恢复原理由和幂等键",async()=>{
 let attempts=0;const writes:RequestInit[]=[];
 const original={...order,state:"COMPLETED",version:"5",allowed_actions:["rollback"],rollback_pending:false};
 const reverse={...order,id:rollbackID,state:"DRAFT",version:"1",allowed_actions:["submit","cancel"],rollback_of_id:id,rollback_pending:false};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/rollback")){writes.push(init!);attempts++;if(attempts===1)throw new TypeError("lost response");return json(reverse,201)}
  if(String(input)===`/api/v1/release-orders/${rollbackID}`)return json(reverse);
  if(String(input)===`/api/v1/release-orders/${id}`)return json(original);
  return json({orders:[],next_cursor:""});
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"申请回滚"}));
 await user.type(screen.getByLabelText("回滚原因"),"保留这个回滚理由");
 await user.click(screen.getByRole("button",{name:"创建回滚草稿"}));
 await screen.findByRole("button",{name:"使用原请求重试"});page.unmount();page=mount();
 await user.click(await screen.findByText("查看原申请内容"));
 expect(await screen.findByText("回滚原因：保留这个回滚理由")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"恢复原发布请求"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect(writes).toHaveLength(2);expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it("回滚申请明确冲突后先读取当前原单，再用原理由和新键重建",async()=>{
 let current={...order,state:"COMPLETED",version:"5",allowed_actions:["rollback"],rollback_pending:false};const writes:RequestInit[]=[];let previews=0;
 const reverse={...order,id:rollbackID,state:"DRAFT",version:"1",allowed_actions:["submit","cancel"],rollback_of_id:id,rollback_pending:false};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/preview")){previews++;return json({table_name:"items",items:order.items})}
  if(path.endsWith("/rollback")){writes.push(init!);if(writes.length===1){current={...current,version:"5"};return json({error:{code:"rollback_conflict",message:"another rollback ended",request_id:"rollback-conflict"}},409)}return json(reverse,201)}
  if(path===`/api/v1/release-orders/${id}`)return json(current);
  if(path===`/api/v1/release-orders/${rollbackID}`)return json(reverse);
  return json({orders:[],next_cursor:""});
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"申请回滚"}));await user.type(screen.getByLabelText("回滚原因"),"仍要恢复原发布");await user.click(screen.getByRole("button",{name:"创建回滚草稿"}));
 expect(await screen.findByText(/已有进行中的回滚申请/)).toBeVisible();page.unmount();page=mount();
 await user.click(await screen.findByText("查看原申请内容"));expect(await screen.findByText("回滚原因：仍要恢复原发布")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"查看最新状态与配置"}));
 await user.click(await screen.findByRole("button",{name:"确认按最新状态申请回滚"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();expect(previews).toBe(0);expect(writes).toHaveLength(2);
 expect(JSON.parse(String(writes[1]!.body))).toEqual({expected_version:"5",reason:"仍要恢复原发布"});
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it("反向草稿保持只读关联，提交冲突恢复不调用普通预览更新基线",async()=>{
 let current={...order,id:rollbackID,state:"DRAFT",version:"1",allowed_actions:["edit","copy","submit","cancel"],rollback_of_id:id,rollback_pending:false,frozen_digest:"a".repeat(64),history:[{action:"ROLLBACK",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-08T08:00:00Z",reason:"恢复原值",related_order_id:id}]};
 const writes:RequestInit[]=[];let previews=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/preview")){previews++;return json({table_name:"items",items:current.items})}
  if(path.endsWith("/submit")){writes.push(init!);if(writes.length===1)throw new TypeError("lost");if(writes.length===2)return json({error:{code:"record_version_conflict",message:"newer row",request_id:"stale"}},409);current={...current,state:"PENDING_APPROVAL",version:"2",allowed_actions:[]};return json(current)}
  if(path===`/api/v1/release-orders/${rollbackID}`)return json(current);
  return json({orders:[],next_cursor:""});
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${rollbackID}`);
 await screen.findByText(/明细来自原发布的实际结果，不可编辑或复制/);
 expect(screen.getByText(/回滚意图已冻结/)).toBeVisible();expect(screen.queryByRole("button",{name:"编辑草稿"})).not.toBeInTheDocument();expect(screen.queryByRole("button",{name:"复制新草稿"})).not.toBeInTheDocument();expect(screen.queryByRole("link",{name:"添加明细"})).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"提交审批"}));await user.click(screen.getByRole("button",{name:"确认提交审批"}));await user.click(await screen.findByRole("button",{name:"使用原请求重试"}));
 expect(await screen.findByText(/记录已被其他操作修改/)).toBeVisible();page.unmount();page=mount();
 await user.click(await screen.findByRole("button",{name:"查看最新状态与配置"}));
 await user.click(await screen.findByRole("button",{name:"确认按最新状态提交审批"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();expect(previews).toBe(0);expect(writes).toHaveLength(3);
 expect(writes[1]!.body).toBe(writes[0]!.body);expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 expect(new Headers(writes[2]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it("原执行旧键重放返回已发布快照后仍重新读取当前已回滚详情",async()=>{
 const published={...order,state:"SUCCEEDED",version:"4",allowed_actions:[]};
 const current={...published,state:"ROLLED_BACK",version:"6",rollback_order_id:rollbackID,rollback_pending:false,history:[...published.history,{action:"ROLLBACK_EXECUTE",actor_id:testAdminIdentity.account.id,version:"6",at:"2026-09-08T09:00:00Z",reason:"",related_order_id:rollbackID}]};
 sessionStorage.setItem(`rcc:release-requests:${testAdminIdentity.account.id}`,JSON.stringify([{scope:`execute:${id}`,path:`/api/v1/release-orders/${id}/execute`,method:"POST",body:'{"expected_version":"3"}',key:"original-execute-key",label:`执行发布 ${id}`}]))
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(String(input).endsWith("/execute")){writes.push(init!);return json(published)}if(String(input)===`/api/v1/release-orders/${id}`)return json(current);return json({orders:[],next_cursor:""})})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"恢复原发布请求"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();expect(screen.getByText("最新回滚发布单",{exact:false})).toBeVisible();
 expect(writes).toHaveLength(1);expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBe("original-execute-key");expect(sessionStorage.length).toBe(0);
});
it("从列表的查看详情入口打开持久草稿，并取消后保留历史",async()=>{
 let current=structuredClone(order);const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/cancel")){writes.push(init!);current={...current,state:"CANCELLED",version:"2",allowed_actions:[]};return json(current)}
  if(path===`/api/v1/release-orders/${id}`)return json(current);
  if(path.startsWith("/api/v1/release-orders"))return json({orders:[current],next_cursor:""});
  return json({policies:[]});
 })));
 const user=userEvent.setup();mount();
 await user.click(await screen.findByRole("link",{name:"查看详情：更新渠道展示名称"}));
 expect(await screen.findByText("proposal")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"取消草稿"}));
 await user.type(screen.getByLabelText("取消原因"),"调整计划");
 await user.click(screen.getByRole("button",{name:"确认取消草稿"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
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

it("发布单标题按 Unicode 字符计数并允许 100 个 emoji",async()=>{
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(init?.method==="PUT"){writes.push(init);return json({...order,title:JSON.parse(String(init.body)).title,version:"2"})}
  if(String(input)===`/api/v1/release-orders/${id}/people`)return json({people:{}});
  return json(order);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 const title=screen.getByLabelText("发布单标题");
 expect(title).toBeRequired();
 fireEvent.change(title,{target:{value:"   "}});
 expect(title).toHaveAttribute("aria-invalid","true");
 expect(title).toHaveAccessibleDescription(/发布单标题必填/);
 expect(screen.getByText("发布单标题必填。")).toBeVisible();
 fireEvent.change(title,{target:{value:"😀".repeat(100)}});
 expect(screen.getByText("100 / 100 字符")).toBeVisible();
 expect(title).toHaveAttribute("aria-invalid","false");
 expect(screen.getByRole("button",{name:"保存草稿修改"})).toBeEnabled();
 fireEvent.change(title,{target:{value:"😀".repeat(101)}});
 expect(screen.getByText("101 / 100 字符")).toBeVisible();
 expect(title).toHaveAccessibleDescription(/不能超过 100 个字符/);
 expect(screen.getByRole("button",{name:"保存草稿修改"})).toBeDisabled();
 fireEvent.change(title,{target:{value:"😀".repeat(100)}});
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body)).title).toBe("😀".repeat(100));
});

it("编辑发布草稿时保留原始 CRLF 和 CR，明确转换后才允许修改",async()=>{
 const raw="line one\r\nline two\rline three";
 const draft={...order,items:[{...order.items[0]!,content:{label:raw},fields:order.items[0]!.fields.map(field=>field.name==="label"?{...field,proposed:raw}:field)}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async()=>json(draft))));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 const input=screen.getByLabelText("label 申请值");
 expect(input).toHaveAttribute("readonly");
 expect(input).toHaveValue("line one\nline two\nline three");
 await user.click(screen.getByRole("button",{name:"label 申请值：转换为 LF 再编辑"}));
 expect(input).not.toHaveAttribute("readonly");
 expect(input).toHaveValue("line one\nline two\nline three");
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
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
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
 await waitFor(()=>expect(within(screen.getByRole("region",{name:"基本信息"})).getByText("发布单版本").parentElement).toHaveTextContent("发布单版本3"));
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
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
});

it("仅有审批角色的人填写意见批准冻结单据",async()=>{
 const applicantID="other-applicant",reviewerID=testAdminIdentity.account.id;let peopleReads=0;
 let current={...order,applicant_id:applicantID,state:"PENDING_APPROVAL",version:"2",allowed_actions:["approve","reject"],frozen_digest:"a".repeat(64),history:[{action:"CREATE",actor_id:applicantID,version:"1",at:"2026-09-07T08:00:00Z",reason:""},{action:"SUBMIT",actor_id:applicantID,version:"2",at:"2026-09-07T09:00:00Z",reason:""}]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:["APPROVER"]}});
  if(String(input).endsWith("/people")){peopleReads++;return json({people:peopleReads===1?{[applicantID]:"当前申请人"}:{[applicantID]:"当前申请人",[reviewerID]:"首次审批人"}})}
  if(String(input).endsWith("/approve")){writes.push(init!);current={...current,state:"APPROVED",version:"3",allowed_actions:[],history:[...current.history,{action:"APPROVE",actor_id:reviewerID,version:"3",at:"2026-09-07T10:00:00Z",reason:"已核对变更范围"}]};return json(current)}
  if(String(input)===`/api/v1/release-orders/${id}`)return json(current);
  return json({orders:[current],next_cursor:""});
 }));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 expect((await screen.findAllByText("当前申请人")).length).toBeGreaterThan(0);expect(screen.queryByText("首次审批人")).not.toBeInTheDocument();
 await user.click(await screen.findByRole("button",{name:"批准发布单"}));
 expect(screen.getByRole("button",{name:"确认批准"})).toBeDisabled();
 await user.type(screen.getByLabelText("审批意见"),"已核对变更范围");
 await user.click(screen.getByRole("button",{name:"确认批准"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect((await screen.findAllByText("首次审批人"))[0]).toBeVisible();expect(peopleReads).toBe(2);
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
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();expect(writes).toHaveLength(2);
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
  if(path.endsWith("/people"))return json({people:{[identity.account.id]:"发布人小程"}});
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
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect((await screen.findAllByText("发布人小程")).length).toBeGreaterThan(0);expect(screen.getAllByText(identity.account.id).length).toBeGreaterThan(0);
 expect(await screen.findByText("items · 已发布待完结")).toBeVisible();
 expect(screen.getByText("值：actual database value")).toBeVisible();expect(screen.getByText("SQL NULL")).toBeVisible();expect(screen.getByText("JSON：null")).toBeVisible();
 expect(screen.getByText("刷新通知：notice · 分发尚未接入")).toBeVisible();expect(sessionStorage.length).toBe(0);
});

it("编辑任意明细并移除另一项，保存仍提交整张草稿",async()=>{
 const second={...order.items[0]!,id:"2",content:{label:"second"},fields:order.items[0]!.fields.map(field=>field.name==="label"?{...field,proposed:"second"}:field)};
 const current={...order,items:[order.items[0]!,second]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(init?.method==="PUT"){writes.push(init);return json({...current,version:"2"})}return json(current)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.selectOptions(screen.getByLabelText("编辑明细"),"1");
 await user.clear(screen.getByLabelText("label 申请值"));await user.type(screen.getByLabelText("label 申请值"),"second edited");
 await user.selectOptions(screen.getByLabelText("编辑明细"),"0");
 await user.click(screen.getByRole("button",{name:"移除此明细"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(1));expect(JSON.parse(String(writes[0]!.body))).toMatchObject({expected_version:"1",items:[{id:"2",content:{label:"second edited"}}]});
 expect(JSON.parse(String(writes[0]!.body)).items).toHaveLength(1);
});

it("千项预览可定位最后一项，审批仍包含整单",async()=>{
 const large={...order,state:"PENDING_APPROVAL",applicant_id:"another-account",allowed_actions:["approve"],items:Array.from({length:1000},(_,index)=>({...order.items[0]!,id:String(index+1),content:{label:`item-${index+1}`},fields:order.items[0]!.fields.map(field=>field.name==="label"?{...field,proposed:`item-${index+1}`}:field)}))};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(String(input).endsWith("/approve")){writes.push(init);return json({...large,state:"APPROVED",version:"2"})}return json(large)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 const jump=await screen.findByLabelText("定位明细");await user.clear(jump);await user.type(jump,"1000");
 expect(await screen.findByText("item-1000")).toBeVisible();expect(screen.queryByText("item-1")).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"批准发布单"}));
 expect(await screen.findByText(/全部 1,000 项将一起/)).toBeVisible();
 await user.type(screen.getByLabelText("审批意见"),"核对整单");await user.click(screen.getByRole("button",{name:"确认批准"}));
 await waitFor(()=>expect(writes).toHaveLength(1));expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"1",reason:"核对整单"});
});

it("大单与待恢复请求超出浏览器保存容量时发送前拒绝并保留原请求",async()=>{
 const key=`rcc:release-requests:${testAdminIdentity.account.id}`;
 const previous=JSON.stringify([{scope:"create",path:"/api/v1/release-orders",method:"POST",body:JSON.stringify({title:"原申请标题",table_name:"items",items:[{operation:"ADD",content:{label:"original intent"}}]}),key:"original-key",label:"已有原请求"}]);
 sessionStorage.setItem(key,previous);
 const large={...order,items:Array.from({length:1000},(_,index)=>({...order.items[0]!,id:String(index+1)}))};let writes=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(init?.method==="PUT")writes++;return json(large)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 const set=vi.spyOn(Storage.prototype,"setItem").mockImplementation(()=>{throw new DOMException("quota","QuotaExceededError")});
 try{await user.click(screen.getByRole("button",{name:"保存草稿修改"}));expect(await screen.findByText(/浏览器无法保存完整请求/)).toBeVisible();expect(writes).toBe(0);expect(sessionStorage.getItem(key)).toBe(previous)}finally{set.mockRestore()}
});

it("千项编辑器只列当前20项并能直接编辑第1000项",async()=>{
 const large={...order,items:Array.from({length:1000},(_,index)=>({...order.items[0]!,id:String(index+1),content:{label:`item-${index+1}`}}))};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(_input,init)=>{if(init?.method==="PUT")writes.push(init);return json(large)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 expect(screen.getByLabelText("编辑明细").querySelectorAll("option")).toHaveLength(20);
 await user.type(screen.getByLabelText("定位编辑明细"),"1000");
 expect(screen.getByLabelText("编辑明细")).toHaveValue("999");
 expect(screen.getByLabelText("label 申请值")).toHaveValue("item-1000");
 await user.clear(screen.getByLabelText("label 申请值"));await user.type(screen.getByLabelText("label 申请值"),"last edited");
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 const items=JSON.parse(String(writes[0]!.body)).items;expect(items).toHaveLength(1000);expect(items[999].content.label).toBe("last edited");expect(items[0].content.label).toBe("item-1");
});

it("完结丢响应后保留原请求并阻止另一个终止动作直到恢复",async()=>{
 let current={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete"]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/complete")){
   writes.push(init!);current={...current,state:"COMPLETED",version:"5",allowed_actions:["rollback"]};
   if(writes.length===1)throw new TypeError("response lost");
  }
  return json(current);
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"完结发布单"}));
 const confirm=screen.getByRole("button",{name:"确认完结"});fireEvent.click(confirm);fireEvent.click(confirm);
 await screen.findByRole("button",{name:"使用原请求重试"});expect(writes).toHaveLength(1);
 page.unmount();page=mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("button",{name:"申请回滚"})).toBeDisabled();
 await user.click(screen.getByRole("button",{name:"恢复原发布请求"}));
 await waitFor(()=>expect(screen.getByRole("button",{name:"申请回滚"})).toBeEnabled());
 expect(writes).toHaveLength(2);
 expect(writes[0]!.body).toBe(writes[1]!.body);
 expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[1]!.headers).get("Idempotency-Key"));
});

it("快速回滚先读取整单恢复预览，取消无写入且原因必填",async()=>{
 const published={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete","quick-rollback"]};
 const restoredItems=order.items.map(item=>({...item,before:{id:"1",label:"proposal"},content:{label:"original"},fields:item.fields.map(field=>field.name==="label"?{...field,before:"proposal",proposed:"original"}:field)}));
 let resolvePreview!:(response:Response)=>void;const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/quick-rollback/preview"))return new Promise<Response>(resolve=>{resolvePreview=resolve});
  if(String(input).endsWith("/quick-rollback"))writes.push(init!);
  return String(input).endsWith("/people")?json({people:{}}):json(published);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"快速回滚"}));
 expect(screen.getByRole("button",{name:"确认整单快速回滚"})).toBeDisabled();
 expect(screen.getByText("正在读取整单恢复预览…")).toBeVisible();
 resolvePreview(json({order_id:id,expected_version:"4",table_name:"items",preview_digest:"a".repeat(64),items:restoredItems}));
 expect(await screen.findByText("本次恢复涉及全部 1 项，无需再次审批。成功后原单和回滚结果均结束，释放目标记录的占用。")).toBeVisible();
 expect(screen.getByRole("columnheader",{name:"当前值"})).toBeVisible();
 expect(screen.getByRole("columnheader",{name:"恢复值"})).toBeVisible();
 expect(screen.getByLabelText("快速回滚原因")).toBeRequired();
 expect(screen.getByRole("button",{name:"确认整单快速回滚"})).toBeDisabled();
 await user.click(screen.getByRole("button",{name:"取消快速回滚"}));
 expect(writes).toHaveLength(0);expect(screen.queryByLabelText("快速回滚原因")).not.toBeInTheDocument();
 expect(screen.getByText("items · 已发布待完结")).toBeVisible();
});

it("快速回滚未知结果保留原摘要原因与标识，锁住完结并跨刷新恢复",async()=>{
 const identity={...testAdminIdentity,account:{...testAdminIdentity.account,roles:["PUBLISHER"]}};
 let current={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete","quick-rollback"]};
 const digest="a".repeat(64),writes:RequestInit[]=[];let previewReads=0;let rejectFirst!:(error:Error)=>void;
 const reverse={...order,id:rollbackID,title:"回滚：更新渠道展示名称",state:"COMPLETED",version:"1",allowed_actions:[],rollback_of_id:id,history:[{action:"QUICK_ROLLBACK",actor_id:identity.account.id,at:order.updated_at,version:"1",reason:"恢复实际配置",related_order_id:id}]};
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  const path=String(input);if(path.startsWith("/api/v1/auth/"))return json(identity);
  if(path.endsWith("/people"))return json({people:{}});
  if(path.endsWith("/quick-rollback/preview")){previewReads++;return json({order_id:id,expected_version:"4",table_name:"items",preview_digest:digest,items:order.items})}
  if(path.endsWith("/quick-rollback")){writes.push(init!);current={...current,state:"ROLLED_BACK",version:"5",allowed_actions:[]};if(writes.length===1)return new Promise<Response>((_,reject)=>{rejectFirst=reject});return json(reverse)}
  if(path.endsWith(rollbackID))return json(reverse);return json(current);
 }));
 const user=userEvent.setup();const first=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"快速回滚"}));
 await user.type(screen.getByLabelText("快速回滚原因"),"恢复实际配置");
 await user.dblClick(screen.getByRole("button",{name:"确认整单快速回滚"}));
 await waitFor(()=>expect(writes).toHaveLength(1));rejectFirst(new TypeError("lost committed response"));
 expect(await screen.findByRole("button",{name:"使用原请求重试"})).toBeVisible();
 expect(screen.getByLabelText("快速回滚原因")).toBeDisabled();
 expect(screen.getByRole("button",{name:"完结发布单"})).toBeDisabled();
 expect(screen.getByRole("button",{name:"快速回滚"})).toBeDisabled();
 expect(previewReads).toBe(1);
 first.unmount();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"恢复原发布请求"}));
 expect(await screen.findByRole("heading",{name:"回滚：更新渠道展示名称"})).toBeVisible();
 expect(writes).toHaveLength(2);expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"4",preview_digest:digest,reason:"恢复实际配置"});
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 expect(previewReads).toBe(1);expect(sessionStorage.length).toBe(0);
});

it("快速回滚明确拒绝后保留原因，只有主动重读并审阅新预览才重建",async()=>{
 const published={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete","quick-rollback"]};
 const reverse={...order,id:rollbackID,title:"回滚：更新渠道展示名称",state:"COMPLETED",version:"1",allowed_actions:[],rollback_of_id:id};
 const writes:RequestInit[]=[];let previewReads=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/people"))return json({people:{}});
  if(String(input).endsWith("/quick-rollback/preview")){previewReads++;return json({order_id:id,expected_version:"4",table_name:"items",preview_digest:"a".repeat(64),items:order.items})}
  if(String(input).endsWith("/quick-rollback")){writes.push(init!);return writes.length===1?json({error:{code:"release_frozen_changed",message:"rules changed",request_id:"changed-rules"}},409):json(reverse)}
  return json(String(input).endsWith(rollbackID)?reverse:published);
 })));
 const user=userEvent.setup();const first=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"快速回滚"}));await user.type(screen.getByLabelText("快速回滚原因"),"需要保留的恢复原因");await user.click(screen.getByRole("button",{name:"确认整单快速回滚"}));
 expect(await screen.findByText("服务器已明确拒绝原请求。原申请保留，请查看最新状态与配置后决定是否重建。")).toBeVisible();
 expect(previewReads).toBe(1);expect(screen.getByLabelText("快速回滚原因")).toHaveValue("需要保留的恢复原因");
 first.unmount();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByText("查看原申请内容"));expect(screen.getByText("快速回滚原因：需要保留的恢复原因")).toBeVisible();
 expect(previewReads).toBe(1);
 await user.click(screen.getByRole("button",{name:"查看最新状态与配置"}));
 expect(await screen.findByText("重新审阅整单恢复预览，确认后将使用新的请求标识执行：")).toBeVisible();
 expect(previewReads).toBe(2);expect(writes).toHaveLength(1);
 await user.click(screen.getByRole("button",{name:"确认按最新状态快速回滚"}));
 expect(await screen.findByRole("heading",{name:"回滚：更新渠道展示名称"})).toBeVisible();
 expect(writes).toHaveLength(2);expect(JSON.parse(String(writes[1]!.body)).reason).toBe("需要保留的恢复原因");
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it("快速回滚预览失败时保留原因，主动重读成功后仍拒绝超长原因",async()=>{
 const published={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete","quick-rollback"]};
 let previewReads=0,writes=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input)=>{
  if(String(input).endsWith("/people"))return json({people:{}});
  if(String(input).endsWith("/quick-rollback/preview")){previewReads++;return previewReads===1?json({error:{code:"release_unavailable",message:"preview unavailable",request_id:"preview-read"}},503):json({order_id:id,expected_version:"4",table_name:"items",preview_digest:"b".repeat(64),items:order.items})}
  if(String(input).endsWith("/quick-rollback"))writes++;
  return json(published);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"快速回滚"}));
 const reason=screen.getByLabelText("快速回滚原因");await user.type(reason,"保留恢复原因");
 expect(screen.getByRole("button",{name:"确认整单快速回滚"})).toBeDisabled();
 await user.click(screen.getByRole("button",{name:"重试"}));
 expect(await screen.findByRole("columnheader",{name:"恢复值"})).toBeVisible();
 expect(reason).toHaveValue("保留恢复原因");expect(previewReads).toBe(2);expect(writes).toBe(0);
 fireEvent.change(reason,{target:{value:"中".repeat(667)}});
 expect(screen.getByText("快速回滚原因不能超过 2000 字节。")).toBeVisible();
 expect(reason).toHaveAttribute("aria-invalid","true");expect(screen.getByRole("button",{name:"确认整单快速回滚"})).toBeDisabled();
 fireEvent.change(reason,{target:{value:"正常原因"}});
 expect(screen.getByRole("button",{name:"确认整单快速回滚"})).toBeEnabled();expect(writes).toBe(0);
});

it.each(["VIEWER","EDITOR","APPROVER"])("当前 %s 即使详情旧权限含快速回滚也不提供入口",async role=>{
 const published={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete","quick-rollback"]};let previewReads=0;
 vi.stubGlobal("fetch",vi.fn(async input=>{
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:[role]}});
  if(String(input).endsWith("/people"))return json({people:{}});
  if(String(input).endsWith("/quick-rollback/preview"))previewReads++;
  return json(published);
 }));
 mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("heading",{name:order.title})).toBeVisible();
 expect(screen.queryByRole("button",{name:"快速回滚"})).not.toBeInTheDocument();expect(previewReads).toBe(0);
});

it("重新准备发送前响应未知时，关闭和刷新仍阻止同单取消与执行并恢复原正文和键",async()=>{
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["cancel","execute","reprepare"],frozen_digest:"a".repeat(64)};
 const draft={...order,id:repreparedID,copied_from_id:id};const writes:RequestInit[]=[];let attempts=0;
 const identity={...testAdminIdentity,account:{...testAdminIdentity.account,roles:["EDITOR","PUBLISHER"]}};
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.includes("/auth/"))return json(identity);
  if(path.endsWith("/preview"))return json({table_name:"items",items:order.items});
  if(path.endsWith("/reprepare")){writes.push(init!);if(++attempts===1)throw new TypeError("disconnected before dispatch");return json(draft,201)}
  if(path.endsWith("/people"))return json({people:{}});
  if(path===`/api/v1/release-orders/${repreparedID}`)return json(draft);
  if(init?.method==="POST")throw new Error(`unexpected conflicting write ${path}`);
  return json(approved);
 }));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"重新准备"}));await user.click(screen.getByRole("button",{name:"读取最新配置"}));
 await user.click(await screen.findByRole("button",{name:"继续重新准备"}));await user.click(screen.getByRole("button",{name:"取消旧单并创建新草稿"}));
 await screen.findByRole("button",{name:"使用原请求重试"});
 await user.click(screen.getByRole("button",{name:"取消"}));
 await user.click(screen.getAllByRole("button",{name:"关闭"}).at(-1)!);
 const discard=await screen.findByRole("button",{name:"放弃修改并离开"});await user.click(discard);
 expect(screen.getByRole("button",{name:"取消发布单"})).toBeDisabled();expect(screen.getByRole("button",{name:"执行发布"})).toBeDisabled();
 page.unmount();page=mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("button",{name:"执行发布"})).toBeDisabled();expect(screen.getByRole("button",{name:"取消发布单"})).toBeDisabled();
 await user.click(screen.getByRole("button",{name:"恢复原发布请求"}));await screen.findByText(/复制自/);
 expect(writes).toHaveLength(2);expect(writes[1]!.body).toBe(writes[0]!.body);expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});
it("已回滚详情默认实际原发布结果并可读取可信恢复结果或申请差异",async()=>{
 const row=(value:string)=>({format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"generated",type:"varchar(40)",encoding:"text",value}],checksum:"b".repeat(64)});
 const publication=(value:string)=>({table_version:"8",publisher_id:order.applicant_id,executed_at:order.updated_at,notification:{id:"notice",table_version:"8",status:"NOT_CONNECTED"},commands:[{order_id:id,sequence:"9",table_name:"items",table_version:"8",operation:"MODIFY",id:"1",record_version:"3",before:row("before"),final:row(value)}]});
 const original={...order,state:"ROLLED_BACK",rollback_order_id:rollbackID,publication:publication("actual original generated"),allowed_actions:[]};
 const reverse={...order,id:rollbackID,state:"COMPLETED",rollback_of_id:id,publication:publication("actual restored generated"),allowed_actions:[]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json(String(input).endsWith(rollbackID)?reverse:original))));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByText("值：actual original generated")).toBeVisible();expect(screen.queryByText("proposal")).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"恢复结果"}));expect(await screen.findByText("值：actual restored generated")).toBeVisible();expect(screen.queryByText("值：actual original generated")).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"申请差异"}));expect(await screen.findByText("proposal")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"原发布结果"}));expect(await screen.findByText("值：actual original generated")).toBeVisible();
});

it("字段名称和选项标签重读后实时更新，但申请 before 与持久化 final 原值不变",async()=>{
 const row=(value:string)=>({format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"channel",type:"varchar(40)",encoding:"text",value}],checksum:"b".repeat(64)});
 const current={...order,state:"SUCCEEDED",allowed_actions:[],items:[{...order.items[0]!,fields:[{name:"channel",type:"string",nullable:false,editable:true,before_state:"value",before:"old",proposed_state:"value",proposed:"new"}]}],publication:{table_version:"8",publisher_id:order.applicant_id,executed_at:order.updated_at,notification:{id:"notice",table_version:"8",status:"NOT_CONNECTED"},commands:[{order_id:id,sequence:"9",table_name:"items",table_version:"8",operation:"MODIFY",id:"1",record_version:"3",before:row("old"),final:row("new")}]}};
 let currentName="渠道";let oldLabel="旧渠道";let newLabel="新渠道";let metadataReads=0;
 fieldPolicyResponse=()=>{
  metadataReads++;
  const configuration=defaultFieldPolicies("items",[{name:"channel",type:"string",nullable:false}]);
  configuration.fields[0]!.state="active";
  configuration.fields[0]!.effective={...configuration.fields[0]!.effective,display_name:currentName,is_visible:false,ui_type:"select",ui_options:{options:[{label:oldLabel,value:"old"},{label:newLabel,value:"new"}]},enabled:true};
  return json(configuration);
 };
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json(current))));
 const user=userEvent.setup();const {client}=mount(`/configuration/release-orders/${id}`);

 expect(await screen.findByText("新渠道")).toBeVisible();
 expect(screen.getByText("旧渠道")).toBeVisible();
 expect(screen.getByText("渠道")).toBeVisible();
 expect(screen.getByLabelText("真实值：old")).toBeVisible();
 expect(screen.getByLabelText("真实值：new")).toBeVisible();
 currentName="通知渠道";oldLabel="原渠道";newLabel="目标渠道";
 await act(async()=>{await client.refetchQueries({queryKey:["current-field-display"]})});
 expect(await screen.findByText("目标渠道")).toBeVisible();
 expect(screen.getByText("原渠道")).toBeVisible();
 expect(screen.getByText("通知渠道")).toBeVisible();
 expect(screen.queryByText("新渠道")).not.toBeInTheDocument();
 expect(screen.getByLabelText("真实值：new")).toBeVisible();
 expect(screen.getByRole("region",{name:/变更顺序 9 实际结果/})).toHaveClass("release-diff-scroll");
 await user.click(screen.getByRole("button",{name:"申请差异"}));
 expect(await screen.findByText("原渠道")).toBeVisible();
 expect(screen.getByText("目标渠道")).toBeVisible();
 expect(screen.getByLabelText("真实值：old")).toBeVisible();
 expect(screen.getByLabelText("真实值：new")).toBeVisible();
 expect(metadataReads).toBe(2);
});

it("字段显示读取失败时保留发布单原始内容，并可独立重试当前标签",async()=>{
 let reads=0;
 fieldPolicyResponse=()=>{
  reads++;
  if(reads===1)return json({error:{code:"field_policy_unavailable",message:"down",request_id:"field-display-1"}},503);
  const configuration=defaultFieldPolicies("items",[{name:"id",type:"uint64",nullable:false},{name:"label",type:"string",nullable:true}]);
  configuration.fields[1]!.state="active";
  configuration.fields[1]!.effective={...configuration.fields[1]!.effective,display_name:"展示名称",ui_type:"select",ui_options:{options:[{label:"建议值",value:"proposal"}]},enabled:true};
  return json(configuration);
 };
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json(order))));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);

 expect(await screen.findByText("proposal")).toBeVisible();
 const alert=await screen.findByRole("alert");
 expect(alert).toHaveTextContent("字段配置暂时不可用");
 expect(alert).toHaveTextContent("field-display-1");
 await user.click(within(alert).getByRole("button",{name:"重试"}));
 expect(await screen.findByText("建议值")).toBeVisible();
 expect(screen.getByText("展示名称")).toBeVisible();
 expect(screen.getByLabelText("真实值：proposal")).toBeVisible();
 expect(reads).toBe(2);
});

it("同一表的不同发布单在每次查看详情时重读当前显示规则",async()=>{
 const secondID="22345678123456781234567812345678";
 const first={...order,title:"第一张发布单"};
 const second={...order,id:secondID,title:"第二张发布单"};
 let currentName="字段一";let reads=0;
 fieldPolicyResponse=()=>{
  reads++;
  const configuration=defaultFieldPolicies("items",[{name:"id",type:"uint64",nullable:false},{name:"label",type:"string",nullable:true}]);
  configuration.fields[1]!.state="active";
  configuration.fields[1]!.effective={...configuration.fields[1]!.effective,display_name:currentName,enabled:true};
  return json(configuration);
 };
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>{
  const path=String(input);
  if(path.endsWith("/people"))return json({people:{}});
  if(path===`/api/v1/release-orders/${id}`)return json(first);
  if(path===`/api/v1/release-orders/${secondID}`)return json(second);
  return json({orders:[first,second],next_cursor:""});
 })));
 const user=userEvent.setup();mount();
 await user.click(await screen.findByRole("link",{name:"第一张发布单"}));
 expect(await screen.findByText("字段一")).toBeVisible();
 currentName="字段二";
 await user.click(screen.getByRole("link",{name:"返回发布单列表"}));
 await user.click(await screen.findByRole("link",{name:"第二张发布单"}));
 expect(await screen.findByText("字段二")).toBeVisible();
 expect(reads).toBe(2);
});

it("详情四阶段与最近五条历史可展开，并复制真实单号",async()=>{
 const events=Array.from({length:8},(_,index)=>({action:index===7?"REJECT":"EDIT",actor_id:order.applicant_id,version:String(index+1),at:order.created_at,reason:`审阅记录 ${index+1}`}));
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json({...order,state:"REJECTED",allowed_actions:["copy"],history:events}))));
 const user=userEvent.setup();const copy=vi.spyOn(navigator.clipboard,"writeText").mockResolvedValue();mount(`/configuration/release-orders/${id}`);
 const progress=await screen.findByRole("list",{name:"发布阶段"});expect(within(progress).getAllByRole("listitem")).toHaveLength(4);
 expect(within(progress).getByText("已拒绝")).toBeVisible();expect(within(progress).getByText("未发布")).toBeVisible();expect(within(progress).queryByText("已完结")).not.toBeInTheDocument();
 const history=screen.getByRole("region",{name:"操作历史"});expect(within(history).getAllByRole("listitem")).toHaveLength(5);expect(within(history).queryByText("审阅记录 3")).not.toBeInTheDocument();expect(within(history).getAllByRole("listitem")[0]).toHaveTextContent("审阅记录 8");
 await user.click(within(history).getByRole("button",{name:"查看全部 8 条记录"}));expect(within(history).getAllByRole("listitem")).toHaveLength(8);
 await user.click(screen.getByRole("button",{name:"复制发布单号"}));expect(copy).toHaveBeenCalledWith(id);expect(await screen.findByText("已复制发布单号")).toBeVisible();
});

it.each([
 ["DRAFT","准备中","待发布后完结"],["PENDING_APPROVAL","待审批","待发布后完结"],["APPROVED","待执行","待发布后完结"],["SUCCEEDED","数据库已发布","待人工完结"],["COMPLETED","数据库已发布","已完结"],["REJECTED","未发布","已终止"],["CANCELLED","未发布","已终止"],["ROLLED_BACK","数据库已发布","已回滚"],
])("%s 详情使用真实阶段",async(state,progress,ending)=>{
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json({...order,state,allowed_actions:[]}))));
 mount(`/configuration/release-orders/${id}`);const stages=within(await screen.findByRole("list",{name:"发布阶段"}));
 expect(stages.getByText(progress)).toBeVisible();expect(stages.getAllByRole("listitem")[3]).toHaveTextContent(ending);
 if(["REJECTED","CANCELLED","ROLLED_BACK"].includes(state))expect(stages.getAllByRole("listitem")[3]).not.toHaveClass("is-complete");
});
it("快速回滚结果的审批阶段明确无需新审批",async()=>{
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json({...order,state:"COMPLETED",rollback_of_id:rollbackID,allowed_actions:[],history:[{...order.history[0],action:"QUICK_ROLLBACK"}]}))));
 mount(`/configuration/release-orders/${id}`);const stages=within(await screen.findByRole("list",{name:"发布阶段"}));expect(stages.getByText("无需再次审批")).toBeVisible();expect(stages.getAllByRole("listitem")[1]).not.toHaveClass("is-complete");
});
it("步骤人员时间来自提交而非创建，完结显示真实完结操作者",async()=>{
 const events=[{action:"CREATE",actor_id:"creator",at:"2026-09-01T01:00:00Z"},{action:"SUBMIT",actor_id:"submitter",at:"2026-09-02T02:00:00Z"},{action:"APPROVE",actor_id:"approver",at:"2026-09-03T03:00:00Z"},{action:"EXECUTE",actor_id:"publisher",at:"2026-09-04T04:00:00Z"},{action:"COMPLETE",actor_id:"closer",at:"2026-09-05T05:00:00Z"}].map((event,index)=>({...event,version:String(index+1),reason:""}));
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{submitter:"提交人",approver:"审批人",publisher:"发布人",closer:"完结人"}}):json({...order,state:"COMPLETED",history:events,allowed_actions:[]}))));
 mount(`/configuration/release-orders/${id}`);const stages=within(await screen.findByRole("list",{name:"发布阶段"}));
 const steps=stages.getAllByRole("listitem");expect(await within(steps[0]!).findByText("提交人")).toBeVisible();expect(steps[0]!.querySelector("time")).toHaveAttribute("datetime",events[1]!.at);expect(within(steps[0]!).queryByText("creator")).not.toBeInTheDocument();
 expect(within(steps[3]!).getByText("完结人")).toBeVisible();expect(steps[3]!.querySelector("time")).toHaveAttribute("datetime",events[4]!.at);
 expect(within(screen.getByRole("region",{name:"操作历史"})).getByText("完结了发布单")).toBeVisible();
});

it("重新准备未知后明确拒绝，必须读取最新版本和差异才可用新键重建",async()=>{
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["cancel","execute","reprepare"]};let current=approved;
 const writes:RequestInit[]=[];let previews=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);if(path.endsWith("/people"))return json({people:{}});
  if(path.endsWith("/preview")){previews++;return json({table_name:"items",items:order.items.map(item=>({...item,expected_record_version:previews===1?"0":"2",fields:item.fields.map(field=>field.name==="label"?{...field,before:previews===1?"original":"fresh reviewed value"}:field)}))})}
  if(path.endsWith("/reprepare")){writes.push(init!);if(writes.length===1)throw new TypeError("unknown");if(writes.length===2){current={...approved,version:"4"};return json({error:{code:"release_version_conflict",message:"updated",request_id:"changed"}},409)}return json({...order,id:repreparedID,copied_from_id:id})}
  return json(current);
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"重新准备"}));await user.click(screen.getByRole("button",{name:"读取最新配置"}));await user.click(screen.getByRole("button",{name:"继续重新准备"}));await user.click(screen.getByRole("button",{name:"取消旧单并创建新草稿"}));await screen.findByRole("button",{name:"使用原请求重试"});
 page.unmount();page=mount(`/configuration/release-orders/${id}`);await user.click(await screen.findByRole("button",{name:"恢复原发布请求"}));await screen.findByRole("button",{name:"查看最新状态与配置"});
 expect(screen.queryByRole("button",{name:"确认按最新状态重新准备"})).not.toBeInTheDocument();expect(previews).toBe(1);
 await user.click(screen.getByRole("button",{name:"查看最新状态与配置"}));expect(await screen.findByText("fresh reviewed value")).toBeVisible();await user.click(screen.getByRole("button",{name:"确认按最新状态重新准备"}));
 await waitFor(()=>expect(writes).toHaveLength(3));expect(writes[0]!.body).toBe(writes[1]!.body);expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[1]!.headers).get("Idempotency-Key"));expect(new Headers(writes[2]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));expect(JSON.parse(String(writes[2]!.body))).toMatchObject({expected_version:"4",items:[{expected_record_version:"2",content:{label:"proposal"}}]});
});
it.each([false,true])("重新准备会话中断隐藏浮层，重新登录后按账号隔离原请求（切换账号=%s）",async(otherAccount)=>{
 let signedIn=true;let currentIdentity={...testAdminIdentity,account:{...testAdminIdentity.account,roles:["EDITOR","PUBLISHER"]}};
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["cancel","execute","reprepare"]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/auth/csrf"))return json({csrf_token:"preauth"});
  if(path.endsWith("/auth/login")){signedIn=true;if(otherAccount)currentIdentity={...currentIdentity,account:{...currentIdentity.account,id:"00000000-0000-4000-8000-000000000099"}};return json(currentIdentity)}
  if(path.includes("/auth/"))return signedIn?json(currentIdentity):json({error:{code:"session_invalid",message:"expired",request_id:"expired"}},401);
  if(path.endsWith("/people"))return json({people:{}});
  if(path.endsWith("/preview"))return json({table_name:"items",items:order.items});
  if(path.endsWith("/reprepare")){writes.push(init!);if(writes.length===1){signedIn=false;return json({error:{code:"session_invalid",message:"expired",request_id:"expired"}},401)}return json({...order,id:repreparedID,copied_from_id:id})}
  return json(approved);
 }));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"重新准备"}));await user.click(screen.getByRole("button",{name:"读取最新配置"}));await user.click(screen.getByRole("button",{name:"继续重新准备"}));await user.click(screen.getByRole("button",{name:"取消旧单并创建新草稿"}));
 expect(await screen.findByRole("dialog",{name:"登录会话已中断"})).toBeVisible();expect(screen.getByRole("alertdialog",{hidden:true})).not.toBeVisible();
 await user.type(await screen.findByLabelText("用户名"),"test.user");await user.type(screen.getByLabelText("密码"),"correct horse battery staple");await user.click(screen.getByRole("button",{name:"登录"}));
 await waitFor(()=>expect(screen.queryByRole("dialog",{name:"登录会话已中断"})).not.toBeInTheDocument());
 if(otherAccount){expect(screen.queryByRole("region",{name:"待处理发布请求"})).not.toBeInTheDocument();expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();expect(writes).toHaveLength(1)}
 else {await user.click(await screen.findByRole("button",{name:"使用原请求重试"}));await waitFor(()=>expect(writes).toHaveLength(2));expect(writes[1]!.body).toBe(writes[0]!.body);expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"))}
});
