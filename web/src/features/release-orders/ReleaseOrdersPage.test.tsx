import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {fireEvent,render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach,expect,it,vi} from "vitest";
import {AppRoutes} from "../../app";
import {ToastProvider} from "../../components/ui/Toast";
import {TestRouter} from "../../test/TestRouter";
import {testAdminIdentity,withAdminSession} from "../../test/account-session";

const id="12345678123456781234567812345678";
const rollbackID="87654321876543218765432187654321";
const order={id,title:"更新渠道展示名称",table_name:"items",applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-07T08:00:00Z",updated_at:"2026-09-07T08:00:00Z",history:[{action:"CREATE",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-07T08:00:00Z",reason:""}],allowed_actions:["edit","cancel"],items:[{operation:"MODIFY",id:"1",expected_record_version:"0",before:{id:"1",label:"original"},content:{label:"proposal"},fields:[{name:"id",type:"uint64",nullable:false,editable:false,before_state:"value",before:"1",proposed_state:"omitted",proposed:null},{name:"label",type:"string",nullable:true,editable:true,before_state:"value",before:"original",proposed_state:"value",proposed:"proposal"}]}]};
const json=(value:unknown,status=200)=>new Response(JSON.stringify(value, (key,item)=>key==="orders"?item.map((order:{items:unknown[]})=>({...order,item_count:order.items.length,operation_counts:{MODIFY:order.items.length}})):item),{status,headers:{"Content-Type":"application/json"}});
function mount(path="/configuration/release-orders"){
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 return render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={[path]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>);
}
afterEach(()=>{vi.unstubAllGlobals();sessionStorage.clear()});
it("详情同时展示标题、相关人员当前姓名、首字头像和永久 ID",async()=>{
 const reviewerID="11111111-2222-4333-8444-555555555555";
 const current={...order,history:[...order.history,{action:"APPROVE",actor_id:reviewerID,version:"2",at:"2026-09-08T08:00:00Z",reason:"已核对"}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{[order.applicant_id]:"申请人阿青",[reviewerID]:"审批人小白"}}):json(current))));
 mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect((await screen.findAllByText("申请人阿青")).length).toBeGreaterThan(0);
 expect(screen.getByText("审批人小白")).toBeVisible();
 expect(screen.getAllByText("申").length).toBeGreaterThan(0);
 expect(screen.getByText("审")).toBeVisible();
 expect(screen.getAllByText(order.applicant_id).length).toBeGreaterThan(0);
 expect(screen.getByText(reviewerID)).toBeVisible();
});
it("人员姓名重读失败时清除缓存姓名、显示错误标识并允许独立重试",async()=>{
 let peopleReads=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>{
  if(!String(input).endsWith("/people"))return json(order);
  peopleReads++;
  if(peopleReads===2)return json({error:{code:"release_unavailable",message:"down",request_id:"people-req-2"}},500);
  return json({people:{[order.applicant_id]:peopleReads===1?"缓存申请人":"重试后的申请人"}});
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 expect((await screen.findAllByText("缓存申请人")).length).toBeGreaterThan(0);
 await user.click(screen.getByRole("button",{name:"重新读取发布单"}));
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
 await user.click(await screen.findByRole("link",{name:"更新渠道展示名称"}));
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
 expect(await screen.findByText("首次审批人")).toBeVisible();expect(peopleReads).toBe(2);
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
