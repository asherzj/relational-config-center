import {releaseFixture,withReleaseReadRoutes,withExecution} from "../../test/release-fixture";
import {IDBObjectStore} from "fake-indexeddb";
import {rememberReleaseRequest,pendingReleaseRequests,hydrateReleaseRequests} from "./release-journal";
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
const order={id,title:"更新渠道展示名称",applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",created_at:"2026-09-07T08:00:00Z",updated_at:"2026-09-07T08:00:00Z",history:[{action:"CREATE",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-07T08:00:00Z",reason:""}],allowed_actions:["edit","cancel"],items:[{detail_id:"1".repeat(32),table_name:"items",operation:"MODIFY",id:"1",expected_record_version:"0",before:{id:"1",label:"original"},content:{label:"proposal"},fields:[{name:"id",type:"uint64",nullable:false,editable:false,before_state:"value",before:"1",proposed_state:"omitted",proposed:null},{name:"label",type:"string",nullable:true,editable:true,before_state:"value",before:"original",proposed_state:"value",proposed:"proposal"}]}]};
const json=(value:unknown,status=200)=>new Response(JSON.stringify(releaseFixture(value)),{status,headers:{"Content-Type":"application/json"}});
let fieldPolicyResponse: (tableName:string)=>Response=()=>json(defaultFieldPolicies("items",[{name:"id",type:"uint64",nullable:false},{name:"label",type:"string",nullable:true}]));
let mountedFetch:typeof fetch|undefined,sourceFetch:typeof fetch;
function mount(path="/configuration/release-orders"){
 const request=globalThis.fetch===mountedFetch?sourceFetch:globalThis.fetch;sourceFetch=request;
 mountedFetch=withReleaseReadRoutes((input,init)=>String(input).includes("/table-field-policies/")?Promise.resolve(fieldPolicyResponse(decodeURIComponent(String(input).split("/").at(-1)!))):request(input,init));
 vi.stubGlobal("fetch",mountedFetch);
 const client=new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
 return {...render(<QueryClientProvider client={client}><ToastProvider><TestRouter initialEntries={[path]}><AppRoutes/></TestRouter></ToastProvider></QueryClientProvider>),client};
}
async function originalButton(name:string){
 const button=await screen.findByRole("button",{name});await waitFor(()=>expect(button).toBeEnabled());return button;
}
async function cancelMenuItem(user:ReturnType<typeof userEvent.setup>,name="取消草稿"){
 await user.click(await originalButton("更多操作"));
 return screen.findByRole("menuitem",{name});
}
async function repeatOriginal(user:ReturnType<typeof userEvent.setup>,action:string,confirm:string){
 const trigger=action.startsWith("取消")?"更多操作":action;
 await waitFor(()=>expect(screen.queryByRole("button",{name:trigger})??screen.queryByRole("link",{name:`查看详情：${order.title}`})).toBeInTheDocument());
 if(!screen.queryByRole("button",{name:trigger}))await user.click(await screen.findByRole("link",{name:`查看详情：${order.title}`}));
 await user.click(action.startsWith("取消")?await cancelMenuItem(user,action):await originalButton(action));
 if(action==="重新准备")await user.click(await originalButton("继续重新准备"));
 await user.click(await originalButton(confirm));
}
afterEach(()=>{vi.unstubAllGlobals();sessionStorage.clear();fieldPolicyResponse=()=>json(defaultFieldPolicies("items",[{name:"id",type:"uint64",nullable:false},{name:"label",type:"string",nullable:true}]))});
it("多表详情按各自已保存实例展示节点和来源，重读不改用当前模板",async()=>{
 const first={instance_id:"flow-items-1",table_name:"items",release_type:"STANDARD",template_code:"finance_v1",template_name:"资金配置核对",template_version:"7",association_version:"3",instantiated_at:"2026-09-11T01:00:00Z",node_list:[{code:"review_funds",type:"APPROVAL",name:"资金负责人确认",required_role:"TABLE_APPROVER",state:"PENDING"},{code:"publish_funds",type:"PUBLICATION",name:"整单资金生效",required_role:"PUBLISHER",state:"PENDING"},{code:"finish_funds",type:"COMPLETION",name:"资金结果确认",required_role:"PUBLISHER",state:"PENDING"}]};
 const second={...first,instance_id:"flow-channels-1",table_name:"channels",template_code:"channels_v3",template_name:"渠道配置复核",template_version:"2",node_list:first.node_list.map((node,index)=>({...node,code:`channel-${index}`,name:["渠道负责人复核","整单渠道发布","渠道结果核实"][index]}))};
 const current={...order,items:[...order.items,{...order.items[0],detail_id:"2".repeat(32),table_name:"channels"}],table_flows:[first,second]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(init?.method&&init.method!=="GET")writes.push(init);return String(input).endsWith("/people")?json({people:{}}):json(current)})));
 const user=userEvent.setup(),{client}=mount(`/configuration/release-orders/${id}`);
 const flows=await screen.findByRole("region",{name:"逐表发布流程"});
 expect(within(flows).getByText("资金配置核对")).toBeVisible();expect(within(flows).getByText("渠道配置复核")).toBeVisible();
 expect(within(flows).getByRole("list",{name:"items 流程节点"})).toHaveTextContent("资金负责人确认");
 expect(within(flows).getByRole("list",{name:"channels 流程节点"})).toHaveTextContent("渠道负责人复核");
 await user.click(within(flows).getAllByText("查看实例来源与版本")[0]!);
 expect(within(flows).getByText("flow-items-1")).toBeVisible();expect(within(flows).getByText("finance_v1 · 版本 7")).toBeVisible();
 await act(async()=>{await client.refetchQueries({queryKey:["release-order",id]})});
 expect(within(flows).getByText("资金负责人确认")).toBeVisible();expect(writes).toHaveLength(0);
 expect(screen.getByRole("region",{name:"提交审批安排"})).toBeVisible();
});
it("缺失流程阻止提交，配置修复后无需改动内容也可明确保存补齐",async()=>{
 let current={...order,missing_flow_tables:["items"],table_flows:[] as unknown[],allowed_actions:["edit","cancel"]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(init?.method==="PUT"){writes.push(init);current={...current,version:"2",missing_flow_tables:[],allowed_actions:["edit","submit","cancel"],table_flows:[{instance_id:"repaired-items",table_name:"items",release_type:"STANDARD",template_code:"repaired_standard",template_name:"恢复的常规流程",template_version:"1",association_version:"2",instantiated_at:"2026-09-11T03:00:00Z",node_list:[{code:"review",type:"APPROVAL",name:"恢复后的表审批",required_role:"TABLE_APPROVER",state:"PENDING"},{code:"publish",type:"PUBLICATION",name:"恢复后的发布",required_role:"PUBLISHER",state:"PENDING"},{code:"finish",type:"COMPLETION",name:"恢复后的完结",required_role:"PUBLISHER",state:"PENDING"}]}]};return json(current)}
  return String(input).endsWith("/people")?json({people:{}}):json(current);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 const missing=await screen.findByRole("region",{name:"流程配置未完成"});
 expect(within(missing).getByText("items")).toBeVisible();expect(screen.queryByRole("button",{name:"提交审批"})).not.toBeInTheDocument();
 expect(within(screen.getByRole("region",{name:"逐表发布流程"})).queryByRole("list")).not.toBeInTheDocument();expect(writes).toHaveLength(0);
 await user.click(within(missing).getByRole("button",{name:"保存草稿以补齐流程"}));
 await user.click(await originalButton("保存草稿修改"));
 expect(await screen.findByText("恢复后的表审批")).toBeVisible();expect(screen.queryByRole("region",{name:"流程配置未完成"})).not.toBeInTheDocument();
 await waitFor(()=>expect(screen.getByRole("button",{name:"提交审批"})).toBeEnabled());
 expect(writes).toHaveLength(1);expect(JSON.parse(String(writes[0]!.body))).toEqual({title:order.title,expected_version:"1",changes:{upserts:[],delete_detail_ids:[]}});
 expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBeTruthy();
});
it("补齐流程保存已确认时，后续详情读取失败仍明确反馈保存成功",async()=>{
 const current={...order,missing_flow_tables:["items"],table_flows:[],allowed_actions:["edit","cancel"]};let saved=false;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(init?.method==="PUT"){saved=true;return json({...current,version:"2"})}
  if(String(input).endsWith("/people"))return json({people:{}});
  if(saved)return json({error:{code:"release_unavailable",message:"详情暂时不可读取",request_id:"flow-read-after-save"}},503);
  return json(current);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"保存草稿以补齐流程"}));
 await user.click(await originalButton("保存草稿修改"));
 expect(await screen.findByText("草稿已保存")).toBeVisible();
 expect(await screen.findByText(/flow-read-after-save/)).toBeVisible();
 expect(screen.queryByRole("dialog",{name:"编辑多表草稿"})).not.toBeInTheDocument();
});
it("草稿用唯一业务标题与紧凑信息，添加变更位于审阅工具栏且保留当前草稿路由",async()=>{
 const draft={...order,title:"channel_configs 配置变更",copied_from_id:rollbackID,allowed_actions:["edit","submit","cancel"]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{[order.applicant_id]:"申请人阿青"}}):json(draft))));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("heading",{level:1,name:draft.title})).toBeVisible();expect(screen.getAllByRole("heading",{level:1})).toHaveLength(1);
 const header=within(screen.getByLabelText("发布单页头"));
 expect(header.getByLabelText("发布单状态")).toHaveTextContent("草稿");expect(header.getByText("变更数量").parentElement).toHaveTextContent("1 项");
 expect(screen.getAllByRole("link",{name:"返回发布单列表"})).toHaveLength(1);expect(screen.queryByRole("navigation",{name:"发布单位置"})).not.toBeInTheDocument();
 expect(header.getByRole("button",{name:"编辑草稿"})).toHaveAttribute("data-variant","outline");expect(header.getByRole("button",{name:"提交审批"})).toHaveAttribute("data-variant","default");
 const info=screen.getByLabelText("基本信息");expect(info).not.toHaveAttribute("open");expect(within(info).getByText(id)).not.toBeVisible();
 await user.click(within(info).getByText("基本信息"));expect(within(info).getByText(id)).toBeVisible();expect(within(info).getByRole("button",{name:"复制发布单号"}).parentElement).toContainElement(within(info).getByText(id));expect(within(info).getByRole("link",{name:rollbackID})).toHaveAttribute("href",`/configuration/release-orders/${rollbackID}`);
 const review=within(screen.getByRole("region",{name:"发布单审阅"}));expect(review.getByRole("heading",{name:"变更内容"})).toBeVisible();
 expect(review.getByRole("link",{name:"添加变更"})).toHaveAttribute("href",`/configuration/managed-data?table_name=items&draft=${id}`);expect(review.getByRole("link",{name:"添加变更"})).toHaveAttribute("data-variant","outline");
 expect(review.queryByRole("group",{name:"审阅内容切换"})).not.toBeInTheDocument();expect(review.queryByRole("button",{name:/申请差异|申请内容/})).not.toBeInTheDocument();
 expect(screen.queryByRole("button",{name:"取消草稿"})).not.toBeInTheDocument();const cancel=await cancelMenuItem(user);expect(cancel).toHaveAttribute("data-variant","destructive");
 await user.click(cancel);expect(await screen.findByRole("button",{name:"确认取消草稿"})).toHaveAttribute("data-variant","destructive");expect(screen.getByRole("dialog",{name:"取消草稿"})).toBeVisible();
 await user.click(within(screen.getByRole("dialog",{name:"取消草稿"})).getAllByRole("button",{name:"关闭"}).at(-1)!);
 await waitFor(()=>expect(header.getByRole("button",{name:"更多操作"})).toHaveFocus());
 await user.keyboard("{Enter}");
 await waitFor(()=>expect(screen.getByRole("menuitem",{name:"取消草稿"})).toHaveFocus());
 await user.keyboard("{Escape}");
 await waitFor(()=>expect(header.getByRole("button",{name:"更多操作"})).toHaveFocus());
});

it("VIEWER 具备服务器表审批资格时页头保持批准与拒绝权限，隐藏草稿和更多操作入口",async()=>{
 const current={...order,applicant_id:"another-person",state:"PENDING_APPROVAL",allowed_actions:["edit","submit","cancel","approve","reject"],approvals:[{table_name:"items",roles:[{id:"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",name:"负责审批人员"}],state:"PENDING"}],approval_context:{revision:"header-reviewer",tables:[{table_name:"items",mode:"ROLE",reason:"当前账号是该表角色成员",can_approve:true}],approvable_tables:["items"]}};
 const identity={...testAdminIdentity,account:{...testAdminIdentity.account,roles:["VIEWER"]}};
 vi.stubGlobal("fetch",vi.fn(async input=>String(input).startsWith("/api/v1/auth/")?json(identity):String(input).endsWith("/people")?json({people:{}}):json(current)));
 mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("button",{name:"批准发布单"})).toHaveAttribute("data-variant","default");expect(screen.getByRole("button",{name:"拒绝发布单"})).toHaveAttribute("data-variant","outline");
 expect(screen.queryByRole("button",{name:/编辑草稿|提交审批|更多操作/})).not.toBeInTheDocument();expect(screen.queryByRole("link",{name:"添加变更"})).not.toBeInTheDocument();
});

it("原申请人或管理员核对最新配置后原子化重新准备已批准普通单",async()=>{
 const approved={...order,applicant_id:"original-applicant",state:"APPROVED",version:"3",allowed_actions:["execute","reprepare"],frozen_digest:"a".repeat(64)};
 const freshItems=approved.items.map(item=>({...item,expected_record_version:"2",before:{id:"1",label:"latest database value"},fields:item.fields.map(field=>field.name==="label"?{...field,before:"latest database value"}:field)}));
 const draft={...order,id:repreparedID,applicant_id:testAdminIdentity.account.id,state:"DRAFT",version:"1",allowed_actions:["edit","submit","cancel"],copied_from_id:id,items:freshItems,history:[{action:"REPREPARE",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-09T01:00:00Z",reason:"",related_order_id:id}]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/preview"))return json({items:freshItems});
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
 await screen.findByText("重新准备自",{exact:false});await user.click(screen.getByText("基本信息",{selector:"summary"}));expect(screen.getByText("重新准备自",{exact:false})).toBeVisible();
 expect(screen.getByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"3",confirmed:true,items:[{detail_id:"1".repeat(32),table_name:"items",operation:"MODIFY",id:"1",expected_record_version:"2",content:{label:"proposal"}}]});
 expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBeTruthy();
});

it("重新准备响应丢失后跨刷新保留原正文与幂等键并恢复同一草稿",async()=>{
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["reprepare"],frozen_digest:"a".repeat(64)};
 const draft={...order,id:repreparedID,state:"DRAFT",version:"1",allowed_actions:["edit","submit","cancel"],copied_from_id:id,history:[{action:"REPREPARE",actor_id:testAdminIdentity.account.id,version:"1",at:"2026-09-09T01:00:00Z",reason:"",related_order_id:id}]};
 const writes:RequestInit[]=[];let attempts=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/preview"))return json({items:order.items});
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
 await originalButton("取消旧单并创建新草稿");page.unmount();page=mount(`/configuration/release-orders/${id}`);
 await user.click(await originalButton("重新准备"));
 await user.click(await screen.findByText("查看原申请内容"));
 expect(await screen.findByText(`重新准备原单 ${id}`)).toBeVisible();
 await user.click(await originalButton("继续重新准备"));await user.click(await originalButton("取消旧单并创建新草稿"));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 await waitFor(()=>expect(writes).toHaveLength(2));expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 await waitFor(()=>expect(pendingReleaseRequests(testAdminIdentity.account.id)).toHaveLength(0));
});
it("详情展示当前姓名和首字头像，按需展开完整永久账号 ID",async()=>{
 const reviewerID="11111111-2222-4333-8444-555555555555";
 const current={...order,history:[...order.history,{action:"APPROVE",actor_id:reviewerID,version:"2",at:"2026-09-08T08:00:00Z",reason:"已核对"}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{[order.applicant_id]:"申请人阿青",[reviewerID]:"审批人小白"}}):json(current))));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect((await screen.findAllByText("申请人阿青")).length).toBeGreaterThan(0);
 expect(screen.getAllByText("审批人小白").some(element=>element.closest("details")===null)).toBe(true);
 expect(screen.getAllByText("申").length).toBeGreaterThan(0);
 expect(screen.getAllByText("审").length).toBeGreaterThan(0);
 expect(screen.queryByText(order.applicant_id)).not.toBeInTheDocument();
 expect(screen.queryByText(reviewerID)).not.toBeInTheDocument();
 await user.click(screen.getAllByRole("button",{name:"查看申请人阿青的账号信息"})[0]!);
 expect(screen.getByText(order.applicant_id)).toBeVisible();
 await user.click(within(screen.getByRole("region",{name:"操作历史"})).getByRole("button",{name:"查看审批人小白的账号信息"}));
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
 const user=userEvent.setup();const {client}=mount(`/configuration/release-orders/${id}`);
 expect((await screen.findAllByText("缓存申请人")).length).toBeGreaterThan(0);
 await act(async()=>{await client.refetchQueries({queryKey:["release-order-people",id]})});
 expect(await screen.findByText("人员姓名读取失败，当前仅显示永久账号 ID。")).toBeVisible();
 expect(screen.getByText("错误代码：release_unavailable")).toBeVisible();
 expect(screen.getByText("请求编号：people-req-2")).toBeVisible();
 expect(screen.queryAllByText("缓存申请人")).toHaveLength(0);
 await user.click(screen.getAllByRole("button",{name:`查看账号 ${order.applicant_id.slice(0,8)}的账号信息`})[0]!);
 expect(screen.getByText(order.applicant_id)).toBeVisible();
 await user.click(screen.getByRole("button",{name:"重新读取人员姓名"}));
 expect((await screen.findAllByText("重试后的申请人")).length).toBeGreaterThan(0);
 expect(screen.queryByText("人员姓名读取失败，当前仅显示永久账号 ID。")).not.toBeInTheDocument();
});
it("发布者明确确认完结，取消无写入且不要求意见",async()=>{
 let current={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete"]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/complete")){writes.push(init!);current={...current,state:"COMPLETED",version:"5",allowed_actions:[]}}
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
 expect(await screen.findByText("已完结",{selector:'[aria-label="发布单状态"]'})).toBeVisible();
 expect(writes).toHaveLength(1);expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"4"});
 expect(screen.queryByRole("button",{name:"申请回滚"})).not.toBeInTheDocument();
});

it("已完结原单没有回滚入口",async()=>{
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json({...order,state:"COMPLETED",allowed_actions:[]}))));
 mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByText("已完结",{selector:'[aria-label="发布单状态"]'})).toBeVisible();
 expect(screen.queryByRole("button",{name:"快速回滚"})).not.toBeInTheDocument();
 expect(screen.queryByRole("button",{name:"申请回滚"})).not.toBeInTheDocument();
});

it("回滚执行人在原单补填和修正可选原因并看到每次真实留痕",async()=>{
 const rollbackExecutionID=`${id}:ROLLBACK`,executorID=testAdminIdentity.account.id;
 let current={...order,state:"ROLLED_BACK" as const,version:"5",updated_at:"2026-09-10T01:00:00Z",allowed_actions:["edit-rollback-reason"],executions:[{id:`${id}:PUBLICATION`,kind:"PUBLICATION" as const,actor_id:"forward-publisher",executed_at:"2026-09-10T00:30:00Z",table_versions:{items:"1"},item_count:1,outcome:"SUCCEEDED" as const},{id:rollbackExecutionID,kind:"ROLLBACK" as const,actor_id:executorID,executed_at:"2026-09-10T01:00:00Z",table_versions:{items:"2"},item_count:1,outcome:"SUCCEEDED" as const}],history:[...order.history,{action:"QUICK_ROLLBACK",actor_id:executorID,at:"2026-09-10T01:00:00Z",version:"5",reason:"",execution_id:rollbackExecutionID}]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/people"))return json({people:{[executorID]:"真实回滚人"}});
  if(path.endsWith("/rollback-reason")){
   writes.push(init!);const reason=JSON.parse(String(init!.body)).reason;
   current={...current,history:[...current.history,{action:"ROLLBACK_REASON",actor_id:executorID,at:"2026-09-10T02:00:00Z",version:"5",reason,execution_id:rollbackExecutionID}]};
   return json(current);
  }
  return json(current);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 const reasonPanel=await screen.findByRole("region",{name:"回滚原因"});
 expect(within(reasonPanel).getByText("尚未填写回滚原因。",{exact:true})).toBeVisible();
 await user.click(within(reasonPanel).getByRole("button",{name:"补填回滚原因"}));
 const dialog=screen.getByRole("dialog",{name:"补填回滚原因 · 更新渠道展示名称"});
 const input=within(dialog).getByLabelText("回滚原因（选填）");
 expect(input).not.toBeRequired();expect(input).toHaveValue("");
 expect(within(dialog).getByRole("button",{name:"取消"})).toHaveFocus();
 fireEvent.change(input,{target:{value:"界".repeat(667)}});expect(within(dialog).getByRole("alert")).toHaveTextContent("2000 字节");expect(within(dialog).getByRole("button",{name:"保存回滚原因"})).toBeDisabled();
 fireEvent.change(input,{target:{value:""}});
 await user.type(input,"数据库约束冲突");
 await user.click(within(dialog).getByRole("button",{name:"保存回滚原因"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body))).toEqual({reason:"数据库约束冲突"});
 expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBeTruthy();
 expect(await within(reasonPanel).findByText("数据库约束冲突",{exact:true})).toBeVisible();
 expect(screen.getByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect(screen.getByText("已回滚",{selector:'[aria-label="发布单状态"]'})).toBeVisible();
 expect(screen.getAllByText("版本 5",{exact:false}).length).toBeGreaterThan(0);

 await user.click(within(reasonPanel).getByRole("button",{name:"修改回滚原因"}));
 const correction=screen.getByRole("dialog",{name:"修改回滚原因 · 更新渠道展示名称"});
 expect(within(correction).getByLabelText("回滚原因（选填）")).toHaveValue("数据库约束冲突");
 await user.clear(within(correction).getByLabelText("回滚原因（选填）"));
 expect(within(correction).getByRole("button",{name:"保存回滚原因"})).toBeEnabled();
 await user.type(within(correction).getByLabelText("回滚原因（选填）"),"确认是外键约束冲突");
 await user.click(within(correction).getByRole("button",{name:"保存回滚原因"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(await within(reasonPanel).findByText("确认是外键约束冲突",{exact:true})).toBeVisible();
 const history=screen.getByRole("region",{name:"操作历史"});
 expect(within(history).getAllByText("修改了回滚原因",{exact:true})).toHaveLength(2);
 expect(within(history).getByText("数据库约束冲突",{exact:true})).toBeVisible();
 expect(within(history).getByText("确认是外键约束冲突",{exact:true})).toBeVisible();
});

it("回滚原因写入报错保留输入且只在再次点击时复用原正文和标识",async()=>{
 const rollbackExecutionID=`${id}:ROLLBACK`,writes:RequestInit[]=[];let attempts=0;
 const rolled={...order,state:"ROLLED_BACK" as const,version:"5",allowed_actions:["edit-rollback-reason"],executions:[{id:`${id}:PUBLICATION`,kind:"PUBLICATION" as const,actor_id:"forward",executed_at:order.updated_at,table_versions:{items:"1"},item_count:1,outcome:"SUCCEEDED" as const},{id:rollbackExecutionID,kind:"ROLLBACK" as const,actor_id:testAdminIdentity.account.id,executed_at:order.updated_at,table_versions:{items:"2"},item_count:1,outcome:"SUCCEEDED" as const}],history:[...order.history,{action:"QUICK_ROLLBACK",actor_id:testAdminIdentity.account.id,at:order.updated_at,version:"5",reason:"",execution_id:rollbackExecutionID}]};
 let current=rolled;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/people"))return json({people:{}});
  if(String(input).endsWith("/rollback-reason")){writes.push(init!);attempts++;if(attempts===1)throw new TypeError("lost response");current={...rolled,history:[...rolled.history,{action:"ROLLBACK_REASON",actor_id:testAdminIdentity.account.id,at:"2026-09-10T02:00:00Z",version:"5",reason:"保留这段输入",execution_id:rollbackExecutionID}]};return json(current)}
  return json(current);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"补填回滚原因"}));
 await user.type(screen.getByLabelText("回滚原因（选填）"),"保留这段输入");
 await user.click(screen.getByRole("button",{name:"保存回滚原因"}));
 expect(await screen.findByText("Admin 连接或响应传输中断。",{exact:true})).toBeVisible();
 expect(writes).toHaveLength(1);expect(screen.getByLabelText("回滚原因（选填）")).toHaveValue("保留这段输入");
 await new Promise(resolve=>setTimeout(resolve,20));expect(writes).toHaveLength(1);
 await user.click(screen.getByRole("button",{name:"保存回滚原因"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 expect(await within(screen.getByRole("region",{name:"回滚原因"})).findByText("保留这段输入",{exact:true})).toBeVisible();
});

it("回滚原因编辑时晚到的另一窗口原请求先恢复且不丢本窗口输入",async()=>{
 const rollbackExecutionID=`${id}:ROLLBACK`,writes:RequestInit[]=[];
 const rolled={...order,state:"ROLLED_BACK" as const,version:"5",allowed_actions:["edit-rollback-reason"],executions:[{id:`${id}:PUBLICATION`,kind:"PUBLICATION" as const,actor_id:"forward",executed_at:order.updated_at,table_versions:{items:"1"},item_count:1,outcome:"SUCCEEDED" as const},{id:rollbackExecutionID,kind:"ROLLBACK" as const,actor_id:testAdminIdentity.account.id,executed_at:order.updated_at,table_versions:{items:"2"},item_count:1,outcome:"SUCCEEDED" as const}],history:[...order.history,{action:"QUICK_ROLLBACK",actor_id:testAdminIdentity.account.id,at:order.updated_at,version:"5",reason:"",execution_id:rollbackExecutionID}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/people"))return json({people:{}});
  if(String(input).endsWith("/rollback-reason")){writes.push(init!);const reason=JSON.parse(String(init!.body)).reason;return json({...rolled,history:[...rolled.history,{action:"ROLLBACK_REASON",actor_id:testAdminIdentity.account.id,at:"2026-09-10T02:00:00Z",version:"5",reason,execution_id:rollbackExecutionID}]})}
  return json(rolled);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"补填回滚原因"}));
 const input=screen.getByLabelText("回滚原因（选填）");await user.type(input,"本窗口未保存内容");
 const otherBody=JSON.stringify({reason:"另一窗口原请求"});
 await act(async()=>{await rememberReleaseRequest(testAdminIdentity.account.id,{scope:`edit-rollback-reason:${id}`,method:"POST",path:`/api/v1/release-orders/${id}/rollback-reason`,body:otherBody,key:"other-reason-key",label:"另一窗口回滚原因"})});
 expect(input).toBeDisabled();expect(input).toHaveValue("本窗口未保存内容");
 await user.click(screen.getByRole("button",{name:"保存回滚原因"}));
 await waitFor(()=>expect(writes).toHaveLength(1));expect(writes[0]!.body).toBe(otherBody);
 await waitFor(()=>expect(input).toBeEnabled());expect(input).toHaveValue("本窗口未保存内容");
 expect(screen.getByRole("dialog",{name:"补填回滚原因 · 更新渠道展示名称"})).toBeVisible();
 await user.click(screen.getByRole("button",{name:"保存回滚原因"}));
 await waitFor(()=>expect(writes).toHaveLength(2));expect(JSON.parse(String(writes[1]!.body))).toEqual({reason:"本窗口未保存内容"});
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).not.toBe("other-reason-key");
});

it("回滚原因窗口打开后服务端撤销动作仍保留输入并禁止提交原请求",async()=>{
 const rollbackExecutionID=`${id}:ROLLBACK`,writes:RequestInit[]=[];
 let rolled={...order,state:"ROLLED_BACK" as const,version:"5",allowed_actions:["edit-rollback-reason"],executions:[{id:`${id}:PUBLICATION`,kind:"PUBLICATION" as const,actor_id:"forward",executed_at:order.updated_at,table_versions:{items:"1"},item_count:1,outcome:"SUCCEEDED" as const},{id:rollbackExecutionID,kind:"ROLLBACK" as const,actor_id:testAdminIdentity.account.id,executed_at:order.updated_at,table_versions:{items:"2"},item_count:1,outcome:"SUCCEEDED" as const}],history:[...order.history,{action:"QUICK_ROLLBACK",actor_id:testAdminIdentity.account.id,at:order.updated_at,version:"5",reason:"",execution_id:rollbackExecutionID}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(String(input).endsWith("/people"))return json({people:{}});if(String(input).endsWith("/rollback-reason")){writes.push(init!);return json(rolled)}return json(rolled)})));
 const user=userEvent.setup();const {client}=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"补填回滚原因"}));
 const input=screen.getByLabelText("回滚原因（选填）");await user.type(input,"撤权后保留");
 await act(async()=>{rolled={...rolled,allowed_actions:[]};await client.refetchQueries({queryKey:["release-order",id]})});
 expect(input).toHaveValue("撤权后保留");await waitFor(()=>expect(screen.getByRole("button",{name:"保存回滚原因"})).toBeDisabled());
 await act(async()=>{await rememberReleaseRequest(testAdminIdentity.account.id,{scope:`edit-rollback-reason:${id}`,method:"POST",path:`/api/v1/release-orders/${id}/rollback-reason`,body:JSON.stringify({reason:"已存原请求"}),key:"revoked-reason-key",label:"已撤权原请求"})});
 expect(screen.getByRole("button",{name:"保存回滚原因"})).toBeDisabled();expect(writes).toHaveLength(0);
});

it("原执行旧键重放返回已发布快照后仍重新读取当前已回滚详情",async()=>{
 const published={...order,state:"SUCCEEDED",version:"4",allowed_actions:[]};
 const current={...published,state:"ROLLED_BACK",version:"6",history:[...published.history,{action:"QUICK_ROLLBACK",actor_id:testAdminIdentity.account.id,version:"6",at:"2026-09-08T09:00:00Z",reason:"",execution_id:`${id}:ROLLBACK`}]};
 await rememberReleaseRequest(testAdminIdentity.account.id,{scope:`execute:${id}`,path:`/api/v1/release-orders/${id}/execute`,method:"POST",body:'{"expected_version":"3"}',key:"original-execute-key",label:`执行发布 ${id}`})
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(String(input).endsWith("/execute")){writes.push(init!);return json(published)}if(String(input)===`/api/v1/release-orders/${id}`)return json(current);return json({orders:[],next_cursor:""})})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await repeatOriginal(user,"执行发布","确认发布到数据库");
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();expect(within(screen.getByRole("list",{name:"发布阶段"})).getByText("已回滚")).toBeVisible();
 await waitFor(()=>expect(writes).toHaveLength(1));expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBe("original-execute-key");await waitFor(()=>expect(pendingReleaseRequests(testAdminIdentity.account.id)).toHaveLength(0));
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
 await user.click(await cancelMenuItem(user));
 await user.type(screen.getByLabelText("取消原因"),"调整计划");
 await user.click(screen.getByRole("button",{name:"确认取消草稿"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 await waitFor(()=>expect(screen.queryByRole("button",{name:"编辑草稿"})).not.toBeInTheDocument());
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
 expect(JSON.parse(String(writes[1].body))).toMatchObject({expected_version:"2",changes:{upserts:[{content:{label:"my retained proposal"}}]}});
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
 expect(await originalButton("保存草稿修改")).toBeVisible();
 page.unmount();account={...account,id:"00000000-0000-4000-8000-000000000099"};page=mount();
 await screen.findByRole("heading",{name:"发布单"});expect(screen.queryByRole("button",{name:"恢复原发布请求"})).not.toBeInTheDocument();expect(writes).toHaveLength(1);
 page.unmount();account={...testAdminIdentity.account};page=mount();
 await repeatOriginal(user,"编辑草稿","保存草稿修改");
 await waitFor(()=>expect(writes).toHaveLength(2));
 await user.click(await originalButton("保存草稿修改"));
 await waitFor(()=>expect(writes).toHaveLength(3));
 for(const next of writes.slice(1)){expect(next.body).toBe(writes[0].body);expect(new Headers(next.headers).get("Idempotency-Key")).toBe(new Headers(writes[0].headers).get("Idempotency-Key"))}
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
});

it("已知 ADD 冲突后查看缺行墓碑并明确重建，保留当前申请值",async()=>{
 const add={...order,items:[{...order.items[0]!,operation:"ADD",before:null,content:{id:"1",label:"proposal"},fields:order.items[0]!.fields.map(field=>({...field,editable:true,before_state:"absent",before:null}))}]};
 const writes:RequestInit[]=[];let previews=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/preview")){previews++;return json({items:[{...add.items[0]!,expected_record_version:"2"}]})}
  if(init?.method==="PUT"){writes.push(init);return writes.length===1?json({error:{code:"record_version_conflict",message:"stale record",request_id:"record-conflict"}},409):json({...add,version:"2"})}
  return json(add);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(screen.getByRole("button",{name:"查看最新配置"})).toBeEnabled());
 await user.click(screen.getByRole("button",{name:"查看最新配置"}));
 await screen.findByText(/记录基线 2/);
 await user.selectOptions(screen.getByLabelText("id 提交方式"),"value");
 await user.clear(screen.getByLabelText("id 申请值"));await user.type(screen.getByLabelText("id 申请值"),"1");
 expect(screen.queryByRole("button",{name:"基于最新配置重建"})).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"查看最新配置"}));await screen.findByText(/记录基线 2/);

 expect(screen.getByRole("button",{name:"保存草稿修改"})).toBeDisabled();
 await user.click(screen.getByRole("button",{name:"基于最新配置重建"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(2));expect(previews).toBe(2);
 expect(JSON.parse(String(writes[1].body))).toMatchObject({expected_version:"1",changes:{upserts:[{operation:"ADD",expected_record_version:"2",content:{id:"1",label:"proposal"}}]}});
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
 await user.click(await originalButton("保存草稿修改"));
 expect(await screen.findByText(/发布单已被其他窗口修改/)).toBeVisible();
 expect(screen.getByLabelText("label 申请值")).toBeEnabled();
 expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 await user.click(screen.getByRole("button",{name:"查看最新发布单"}));
 await user.click(await screen.findByRole("button",{name:"基于最新发布单重建"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(3));
 expect(JSON.parse(String(writes[2]!.body))).toMatchObject({expected_version:"2",changes:{upserts:[{content:{label:"retained after unknown"}}]}});
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
 await user.click(await originalButton("保存草稿修改"));
 await user.click(await screen.findByRole("button",{name:"查看最新发布单"}));
 expect(await screen.findByText(/状态：CANCELLED/)).toBeVisible();
 expect(screen.getByRole("button",{name:"基于最新发布单重建"})).toBeDisabled();
 expect(screen.getByLabelText("label 申请值")).toHaveValue("proposal");
 expect(JSON.stringify(pendingReleaseRequests(testAdminIdentity.account.id))).toContain("release_state_invalid");
 expect(attempts).toBe(2);
});

it("刷新后原请求被明确拒绝仍保留申请，核对后才能确认重建",async()=>{
 let current=structuredClone(order);const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/preview"))return json({items:current.items});
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
 await originalButton("保存草稿修改");page.unmount();page=mount();
 await repeatOriginal(user,"编辑草稿","保存草稿修改");
 await user.click(await screen.findByText("查看原申请内容"));
 expect((await screen.findAllByText("值：unique recovered intent")).some(element=>element.closest("details")?.open)).toBe(true);
 expect(JSON.stringify(pendingReleaseRequests(testAdminIdentity.account.id))).toContain("unique recovered intent");
 page.unmount();page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 expect(screen.getByRole("button",{name:"保存草稿修改"})).toBeEnabled();
 expect(writes).toHaveLength(2);
 expect(JSON.stringify(pendingReleaseRequests(testAdminIdentity.account.id))).toContain("unique recovered intent");
 page.unmount();mount();
 await user.click(await screen.findByRole("button",{name:"查看最新状态与配置"}));
 const rebuild=await screen.findByRole("button",{name:"确认重建并保存草稿"});expect(rebuild).toBeEnabled();expect(writes).toHaveLength(2);
 await user.click(rebuild);await waitFor(()=>expect(writes).toHaveLength(3));
 expect(JSON.parse(String(writes[2]!.body))).toMatchObject({expected_version:"2",changes:{upserts:[{content:{label:"unique recovered intent"}}]}});
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
 await user.click(await originalButton("保存草稿修改"));
 await waitFor(()=>expect(within(screen.getByLabelText("基本信息")).getByText("发布单版本").parentElement).toHaveTextContent("发布单版本3"));
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
 await user.click(await cancelMenuItem(user));
 await user.type(screen.getByLabelText("取消原因"),"retained cancellation");
 await user.click(screen.getByRole("button",{name:"确认取消草稿"}));
 await originalButton("确认取消草稿");page.unmount();page=mount();
 await repeatOriginal(user,"取消草稿","确认取消草稿");
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
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:["VIEWER"]}});
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
 const info=screen.getByLabelText("基本信息");await user.click(within(info).getByText("基本信息"));
 expect(within(info).getByRole("button",{name:"查看首次审批人的账号信息"})).toBeVisible();expect(peopleReads).toBe(2);
 expect(writes).toHaveLength(1);expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"2",reason:"已核对变更范围",confirmed_tables:["items"],expected_approval_revision:"fixture-approval-1"});
});

it("审批状态冲突跨刷新保留原意见，查看最新后才显式重建",async()=>{
 let current={...order,applicant_id:"other-applicant",state:"PENDING_APPROVAL",version:"2",allowed_actions:["approve","reject"]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:["VIEWER"]}});
  if(String(input).endsWith("/approve")){writes.push(init!);if(writes.length===1){current={...current,version:"3"};return json({error:{code:"release_version_conflict",message:"changed",request_id:"cas"}},409)}current={...current,state:"APPROVED",version:"4",allowed_actions:[]};return json(current)}
  if(String(input)===`/api/v1/release-orders/${id}`)return json(current);return json({orders:[current],next_cursor:""});
 }));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"批准发布单"}));await user.type(screen.getByLabelText("审批意见"),"保留这条意见");await user.click(screen.getByRole("button",{name:"确认批准"}));
 await waitFor(()=>expect(writes).toHaveLength(1));page.unmount();page=mount();
 await user.click(await screen.findByText("查看原申请内容"));expect(await screen.findByText("审批意见：保留这条意见")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"查看最新状态与配置"}));await user.click(await screen.findByRole("button",{name:"确认按最新状态批准发布单"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();expect(writes).toHaveLength(2);
 expect(JSON.parse(String(writes[1]!.body))).toEqual({expected_version:"3",reason:"保留这条意见",confirmed_tables:["items"],expected_approval_revision:"fixture-approval-1"});expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it("仅 PUBLISHER 执行原审批，丢响应后跨刷新使用原键确认并显示最终值",async()=>{
 const identity={...testAdminIdentity,account:{...testAdminIdentity.account,roles:["PUBLISHER"]}};
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["execute"]};
 const final=withExecution({...approved,state:"SUCCEEDED",version:"4",allowed_actions:[]},"PUBLICATION",[{order_id:id,sequence:"9",table_name:"items",table_version:"7",operation:"MODIFY",id:"1",record_version:"2",before:{format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"label",type:"varchar(40)",encoding:"text",value:"original"}],checksum:"b".repeat(64)},final:{format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"label",type:"varchar(40)",encoding:"text",value:"actual database value"},{name:"empty",type:"text",encoding:"text",value:""},{name:"nil",type:"json",encoding:"sql_null",value:null},{name:"json",type:"json",encoding:"json",value:"null"}],checksum:"c".repeat(64)}}],identity.account.id,"2026-09-08T01:00:00Z");
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
 expect(await screen.findByText(/原请求与意见已保留/)).toBeVisible();
 first.unmount();mount(`/configuration/release-orders/${id}`);
 await repeatOriginal(user,"执行发布","确认发布到数据库");
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(writes[0]!.body).toBe('{"expected_version":"3"}');expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 expect((await screen.findAllByText("发布人小程")).length).toBeGreaterThan(0);await user.click(screen.getAllByRole("button",{name:"查看发布人小程的账号信息"})[0]!);expect(screen.getByText(identity.account.id)).toBeVisible();
 expect(await screen.findByText("已发布待完结",{selector:'[aria-label="发布单状态"]'})).toBeVisible();
 expect(screen.getByText("值：actual database value")).toBeVisible();expect(screen.getByText("SQL NULL")).toBeVisible();expect(screen.getByText("JSON：null")).toBeVisible();
 expect(screen.getByText(`items · 表发布版本 7 · 刷新通知 ${id}:PUBLICATION · 分发尚未接入`)).toBeVisible();await waitFor(()=>expect(pendingReleaseRequests(testAdminIdentity.account.id)).toHaveLength(0));
});

it("分页编辑及删除只提交本次明细变化和整单版本",async()=>{
 const second={...order.items[0]!,detail_id:"2".repeat(32),id:"2",content:{label:"second"},fields:order.items[0]!.fields.map(field=>field.name==="label"?{...field,proposed:"second"}:field)};
 const current={...order,items:[order.items[0]!,second]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(init?.method==="PUT"){writes.push(init);return json({...current,version:"2"})}return json(current)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.selectOptions(screen.getByLabelText("编辑明细"),"1");
 await user.clear(screen.getByLabelText("label 申请值"));await user.type(screen.getByLabelText("label 申请值"),"second edited");
 await user.selectOptions(screen.getByLabelText("编辑明细"),"0");
 await user.click(screen.getByRole("button",{name:"移除此明细"}));
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await waitFor(()=>expect(writes).toHaveLength(1));expect(JSON.parse(String(writes[0]!.body))).toMatchObject({expected_version:"1",changes:{upserts:[{detail_id:"2".repeat(32),id:"2",content:{label:"second edited"}}],delete_detail_ids:["1".repeat(32)]}});
 expect(JSON.parse(String(writes[0]!.body)).items).toBeUndefined();
});

it("千项预览可定位最后一项，审批仍覆盖本人整张表",async()=>{
 const large={...order,state:"PENDING_APPROVAL",applicant_id:"another-account",allowed_actions:["approve"],items:Array.from({length:1000},(_,index)=>({...order.items[0]!,detail_id:String(index+1).padStart(32,"0"),id:String(index+1),content:{label:`item-${index+1}`},fields:order.items[0]!.fields.map(field=>field.name==="label"?{...field,proposed:`item-${index+1}`}:field)}))};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(String(input).endsWith("/approve")){writes.push(init);return json({...large,state:"APPROVED",version:"2"})}return json(large)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 const jump=await screen.findByLabelText("定位明细");await user.clear(jump);await user.type(jump,"1000");
 expect(await screen.findByText("item-1000")).toBeVisible();expect(screen.queryByText("item-1")).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"批准发布单"}));
 expect(screen.getByRole("region",{name:"本次审批范围"})).toHaveTextContent("items");
 await user.type(screen.getByLabelText("审批意见"),"核对整单");await user.click(screen.getByRole("button",{name:"确认批准"}));
 await waitFor(()=>expect(writes).toHaveLength(1));expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"1",reason:"核对整单",confirmed_tables:["items"],expected_approval_revision:"fixture-approval-1"});
});

it("大单与待恢复请求超出浏览器保存容量时发送前拒绝并保留原请求",async()=>{
 const previous=[{scope:"create",path:"/api/v1/release-orders",method:"POST",body:JSON.stringify({title:"原申请标题",items:[{table_name:"items",operation:"ADD",content:{label:"original intent"}}]}),key:"original-key",label:"已有原请求"}] as const;
 await rememberReleaseRequest(testAdminIdentity.account.id,previous[0]);
 const large={...order,items:Array.from({length:1000},(_,index)=>({...order.items[0]!,detail_id:String(index+1).padStart(32,"0"),id:String(index+1)}))};let writes=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(init?.method==="PUT")writes++;return json(large)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 const set=vi.spyOn(IDBObjectStore.prototype,"put").mockImplementation(()=>{throw new DOMException("quota","QuotaExceededError")});
 try{await user.click(screen.getByRole("button",{name:"保存草稿修改"}));expect(await screen.findByText(/浏览器无法保存完整请求/)).toBeVisible();expect(writes).toBe(0);await hydrateReleaseRequests(testAdminIdentity.account.id);expect(pendingReleaseRequests(testAdminIdentity.account.id)).toEqual(previous)}finally{set.mockRestore()}
});

it("千项编辑器只列当前20项并能直接编辑第1000项",async()=>{
 const large={...order,items:Array.from({length:1000},(_,index)=>({...order.items[0]!,detail_id:String(index+1).padStart(32,"0"),id:String(index+1),content:{label:`item-${index+1}`}}))};const writes:RequestInit[]=[];
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
 const input=JSON.parse(String(writes[0]!.body));expect(input.items).toBeUndefined();expect(input.changes.upserts).toHaveLength(1);expect(input.changes.upserts[0].detail_id).toBe("1000".padStart(32,"0"));expect(input.changes.upserts[0].content.label).toBe("last edited");
});

it("完结丢响应后保留原请求并通过原完结操作安全重推",async()=>{
 let current={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete"]};const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/complete")){
   writes.push(init!);current={...current,state:"COMPLETED",version:"5",allowed_actions:[]};
   if(writes.length===1)throw new TypeError("response lost");
  }
  return json(current);
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"完结发布单"}));
 const confirm=screen.getByRole("button",{name:"确认完结"});fireEvent.click(confirm);fireEvent.click(confirm);
 await originalButton("确认完结");expect(writes).toHaveLength(1);
 page.unmount();page=mount(`/configuration/release-orders/${id}`);
 await screen.findByText("已完结",{selector:'[aria-label="发布单状态"]'});expect(screen.queryByRole("button",{name:"申请回滚"})).not.toBeInTheDocument();
 await repeatOriginal(user,"完结发布单","确认完结");
 await waitFor(()=>expect(pendingReleaseRequests(testAdminIdentity.account.id)).toHaveLength(0));
 expect(writes).toHaveLength(2);
 expect(writes[0]!.body).toBe(writes[1]!.body);
 expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[1]!.headers).get("Idempotency-Key"));
});

it("快速回滚先读取整单恢复预览，取消无写入且无需必填原因",async()=>{
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
 expect(await screen.findByText("本次恢复涉及全部 1 项，无需再次审批。成功后原单标记已回滚，释放目标记录的占用。")).toBeVisible();
 expect(screen.getByRole("columnheader",{name:"当前值"})).toBeVisible();
 expect(screen.getByRole("columnheader",{name:"恢复值"})).toBeVisible();
 expect(screen.getByLabelText("快速回滚原因（选填）")).not.toBeRequired();
 expect(screen.getByRole("button",{name:"确认整单快速回滚"})).toBeEnabled();
 await user.click(screen.getByRole("button",{name:"取消快速回滚"}));
 expect(writes).toHaveLength(0);expect(screen.queryByLabelText("快速回滚原因（选填）")).not.toBeInTheDocument();
 expect(screen.getByText("已发布待完结",{selector:'[aria-label="发布单状态"]'})).toBeVisible();
});

it("快速回滚未知结果保留原摘要原因与标识，正常读取当前终态并跨刷新重推",async()=>{
 const identity={...testAdminIdentity,account:{...testAdminIdentity.account,roles:["PUBLISHER"]}};
 let current={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete","quick-rollback"]};
 const digest="a".repeat(64),writes:RequestInit[]=[];let previewReads=0;let rejectFirst!:(error:Error)=>void;
 const reverse={...order,id,title:"更新渠道展示名称",state:"ROLLED_BACK",version:"5",allowed_actions:[],history:[{action:"QUICK_ROLLBACK",actor_id:identity.account.id,at:order.updated_at,version:"1",reason:"恢复实际配置",related_order_id:id}]};
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  const path=String(input);if(path.startsWith("/api/v1/auth/"))return json(identity);
  if(path.endsWith("/people"))return json({people:{}});
  if(path.endsWith("/quick-rollback/preview")){previewReads++;return json({order_id:id,expected_version:"4",table_name:"items",preview_digest:digest,items:order.items})}
  if(path.endsWith("/quick-rollback")){writes.push(init!);current={...current,state:"ROLLED_BACK",version:"5",allowed_actions:[]};if(writes.length===1)return new Promise<Response>((_,reject)=>{rejectFirst=reject});return json(reverse)}
  if(path.endsWith(rollbackID))return json(reverse);return json(current);
 }));
 const user=userEvent.setup();const first=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"快速回滚"}));
 await user.type(screen.getByLabelText("快速回滚原因（选填）"),"恢复实际配置");
 await user.dblClick(screen.getByRole("button",{name:"确认整单快速回滚"}));
 await waitFor(()=>expect(writes).toHaveLength(1));rejectFirst(new TypeError("lost committed response"));
 expect(await originalButton("确认整单快速回滚")).toBeVisible();
 expect(screen.getByLabelText("快速回滚原因（选填）")).toBeDisabled();
 expect(screen.queryByRole("button",{name:"完结发布单"})).not.toBeInTheDocument();
 expect(screen.getByRole("button",{name:"快速回滚"})).toBeEnabled();
 expect(previewReads).toBe(1);
 first.unmount();mount(`/configuration/release-orders/${id}`);
 await repeatOriginal(user,"快速回滚","确认整单快速回滚");
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 await waitFor(()=>expect(writes).toHaveLength(2));expect(writes[1]!.body).toBe(writes[0]!.body);
 expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"4",preview_digest:digest,reason:"恢复实际配置"});
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 expect(previewReads).toBe(1);await waitFor(()=>expect(pendingReleaseRequests(testAdminIdentity.account.id)).toHaveLength(0));
});

it("快速回滚明确拒绝后保留原因，只有主动重读并审阅新预览才重建",async()=>{
 const published={...order,state:"SUCCEEDED",version:"4",allowed_actions:["complete","quick-rollback"]};
 const reverse={...order,id,title:"更新渠道展示名称",state:"ROLLED_BACK",version:"5",allowed_actions:[]};
 const writes:RequestInit[]=[];let previewReads=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/people"))return json({people:{}});
  if(String(input).endsWith("/quick-rollback/preview")){previewReads++;return json({order_id:id,expected_version:"4",table_name:"items",preview_digest:"a".repeat(64),items:order.items})}
  if(String(input).endsWith("/quick-rollback")){writes.push(init!);return writes.length===1?json({error:{code:"release_frozen_changed",message:"rules changed",request_id:"changed-rules"}},409):json(reverse)}
  return json(String(input).endsWith(rollbackID)?reverse:published);
 })));
 const user=userEvent.setup();const first=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"快速回滚"}));await user.type(screen.getByLabelText("快速回滚原因（选填）"),"需要保留的恢复原因");await user.click(screen.getByRole("button",{name:"确认整单快速回滚"}));
 expect(await screen.findByText("服务器已明确拒绝原请求。原申请保留，请查看最新状态与配置后决定是否重建。")).toBeVisible();
 expect(previewReads).toBe(1);expect(screen.getByLabelText("快速回滚原因（选填）")).toHaveValue("需要保留的恢复原因");
 first.unmount();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByText("查看原申请内容"));expect(screen.getByText("快速回滚原因：需要保留的恢复原因")).toBeVisible();
 expect(previewReads).toBe(1);
 await user.click(screen.getByRole("button",{name:"查看最新状态与配置"}));
 expect(await screen.findByText("重新审阅整单恢复预览，确认后将使用新的请求标识执行：")).toBeVisible();
 expect(previewReads).toBe(2);expect(writes).toHaveLength(1);
 await user.click(screen.getByRole("button",{name:"确认按最新状态快速回滚"}));
 expect(await screen.findByRole("heading",{name:"更新渠道展示名称"})).toBeVisible();
 await waitFor(()=>expect(writes).toHaveLength(2));expect(JSON.parse(String(writes[1]!.body)).reason).toBe("需要保留的恢复原因");
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
 const reason=screen.getByLabelText("快速回滚原因（选填）");await user.type(reason,"保留恢复原因");
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

it.each(["VIEWER","EDITOR"])("当前 %s 即使详情旧权限含快速回滚也不提供入口",async role=>{
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

it("重新准备发送前响应未知时，关闭和刷新保留原正文和键且当前主单操作可用",async()=>{
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["cancel","execute","reprepare"],frozen_digest:"a".repeat(64)};
 const draft={...order,id:repreparedID,copied_from_id:id};const writes:RequestInit[]=[];let attempts=0;
 const identity={...testAdminIdentity,account:{...testAdminIdentity.account,roles:["EDITOR","PUBLISHER"]}};
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.includes("/auth/"))return json(identity);
  if(path.endsWith("/preview"))return json({items:order.items});
  if(path.endsWith("/reprepare")){writes.push(init!);if(++attempts===1)throw new TypeError("disconnected before dispatch");return json(draft,201)}
  if(path.endsWith("/people"))return json({people:{}});
  if(path===`/api/v1/release-orders/${repreparedID}`)return json(draft);
  if(init?.method==="POST")throw new Error(`unexpected conflicting write ${path}`);
  return json(approved);
 }));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"重新准备"}));await user.click(screen.getByRole("button",{name:"读取最新配置"}));
 await user.click(await screen.findByRole("button",{name:"继续重新准备"}));await user.click(screen.getByRole("button",{name:"取消旧单并创建新草稿"}));
 await originalButton("取消旧单并创建新草稿");
 await user.click(screen.getByRole("button",{name:"取消"}));
 await user.click(screen.getAllByRole("button",{name:"关闭"}).at(-1)!);
 const discard=await screen.findByRole("button",{name:"放弃修改并离开"});await user.click(discard);
 expect(await cancelMenuItem(user,"取消发布单")).not.toHaveAttribute("data-disabled");await user.keyboard("{Escape}");expect(screen.getByRole("button",{name:"执行发布"})).toBeEnabled();
 page.unmount();page=mount(`/configuration/release-orders/${id}`);
 expect(await originalButton("执行发布")).toBeEnabled();expect(await cancelMenuItem(user,"取消发布单")).not.toHaveAttribute("data-disabled");await user.keyboard("{Escape}");
 await repeatOriginal(user,"重新准备","取消旧单并创建新草稿");await screen.findByText(/复制自/);
 await waitFor(()=>expect(writes).toHaveLength(2));expect(writes[1]!.body).toBe(writes[0]!.body);expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});
it("已回滚详情默认实际原发布结果并可读取可信恢复结果或申请差异",async()=>{
 const row=(value:string)=>({format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"generated",type:"varchar(40)",encoding:"text",value}],checksum:"b".repeat(64)});
 const commands=(value:string)=>[{order_id:id,sequence:"9",table_name:"items",table_version:"8",operation:"MODIFY",id:"1",record_version:"3",before:row("before"),final:row(value)}];
 const original=withExecution(withExecution({...order,state:"ROLLED_BACK",allowed_actions:[]},"PUBLICATION",commands("actual original generated")),"ROLLBACK",commands("actual restored generated"));
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json(original))));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByText("值：actual original generated")).toBeVisible();expect(screen.queryByText("proposal")).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"恢复结果"}));expect(await screen.findByText("值：actual restored generated")).toBeVisible();expect(screen.queryByText("值：actual original generated")).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"申请内容"}));expect(await screen.findByText("proposal")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"实际发布结果"}));expect(await screen.findByText("值：actual original generated")).toBeVisible();
});

it("字段名称和选项标签重读后实时更新，但申请 before 与持久化 final 原值不变",async()=>{
 const row=(value:string)=>({format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"channel",type:"varchar(40)",encoding:"text",value}],checksum:"b".repeat(64)});
 const current=withExecution({...order,state:"SUCCEEDED",allowed_actions:[],items:[{...order.items[0]!,fields:[{name:"channel",type:"string",nullable:false,editable:true,before_state:"value",before:"old",proposed_state:"value",proposed:"new"}]}]},"PUBLICATION",[{order_id:id,sequence:"9",table_name:"items",table_version:"8",operation:"MODIFY",id:"1",record_version:"3",before:row("old"),final:row("new")}]);
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
 expect(screen.getByRole("region",{name:/明细 1 items 实际结果/})).toHaveClass("release-diff-scroll");
 await user.click(screen.getByRole("button",{name:"申请内容"}));
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

it("详情真实阶段与最近五条历史可展开，并复制真实单号",async()=>{
 const events=Array.from({length:8},(_,index)=>({action:index===7?"REJECT":"EDIT",actor_id:order.applicant_id,version:String(index+1),at:order.created_at,reason:`审阅记录 ${index+1}`}));
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json({...order,state:"REJECTED",allowed_actions:["copy"],history:events}))));
 const user=userEvent.setup();const copy=vi.spyOn(navigator.clipboard,"writeText").mockResolvedValue();mount(`/configuration/release-orders/${id}`);
 const progress=await screen.findByRole("region",{name:"发布阶段"});
 expect(within(progress).getByText("已拒绝，整单终止")).toBeVisible();expect(within(progress).queryByText("已完结")).not.toBeInTheDocument();
 const history=screen.getByRole("region",{name:"操作历史"});expect(within(history).getAllByRole("listitem")).toHaveLength(5);expect(within(history).queryByText("审阅记录 3")).not.toBeInTheDocument();expect(within(history).getAllByRole("listitem")[0]).toHaveTextContent("审阅记录 8");
 await user.click(within(history).getByRole("button",{name:"查看全部 8 条记录"}));expect(within(history).getAllByRole("listitem")).toHaveLength(8);
 await user.click(screen.getByText("基本信息",{selector:"summary"}));await user.click(screen.getByRole("button",{name:"复制发布单号"}));expect(copy).toHaveBeenCalledWith(id);expect(await screen.findByText("已复制发布单号")).toBeVisible();
});

it.each([
 ["DRAFT","准备中"],["PENDING_APPROVAL","待审批"],["APPROVED","待执行发布"],["SUCCEEDED","数据库已发布，待人工完结"],["COMPLETED","已完结"],["REJECTED","已拒绝，整单终止"],["CANCELLED","已取消"],
])("%s 详情使用已保存整单阶段",async(state,phase)=>{
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json({...order,state,allowed_actions:[]}))));
 mount(`/configuration/release-orders/${id}`);const stages=within(await screen.findByRole("region",{name:"发布阶段"}));
 expect(stages.getByRole("heading",{name:phase})).toBeVisible();
 expect(stages.queryByRole("list")).not.toBeInTheDocument();
});
it("原单快速回滚保留原批准阶段且不创建新的审批阶段",async()=>{
 const current={...order,state:"ROLLED_BACK",allowed_actions:[],history:[...order.history,{...order.history[0]!,action:"APPROVE",actor_id:"original-approver"},{...order.history[0]!,action:"QUICK_ROLLBACK",actor_id:"actual-rollback"}]};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json(current))));
 mount(`/configuration/release-orders/${id}`);const stages=within(await screen.findByRole("list",{name:"发布阶段"}));expect(stages.getByText("已批准")).toBeVisible();expect(stages.getAllByRole("listitem")[1]).toHaveClass("is-complete");expect(stages.getByText("已回滚")).toBeVisible();
});
it("准备人员时间来自提交，节点显示服务端实例中的真实操作者",async()=>{
 const events=[{action:"CREATE",actor_id:"creator",at:"2026-09-01T01:00:00Z"},{action:"SUBMIT",actor_id:"submitter",at:"2026-09-02T02:00:00Z"},{action:"COMPLETE",actor_id:"closer",at:"2026-09-05T05:00:00Z"}].map((event,index)=>({...event,version:String(index+1),reason:""}));
 const table_flows=[{instance_id:"completed-items",table_name:"items",release_type:"STANDARD",template_code:"saved_flow",template_name:"已保存的核对流程",template_version:"1",association_version:"1",instantiated_at:order.created_at,node_list:[{code:"review",type:"APPROVAL",name:"已保存审批",required_role:"TABLE_APPROVER",state:"COMPLETED",actor_id:"instance-reviewer",at:"2026-09-03T03:00:00Z"},{code:"publish",type:"PUBLICATION",name:"已保存发布",required_role:"PUBLISHER",state:"COMPLETED",actor_id:"instance-publisher",at:"2026-09-04T04:00:00Z"},{code:"finish",type:"COMPLETION",name:"已保存完结",required_role:"PUBLISHER",state:"COMPLETED",actor_id:"closer",at:"2026-09-05T05:00:00Z"}]}];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{submitter:"提交人","instance-reviewer":"实例审批人","instance-publisher":"实例发布人",closer:"完结人"}}):json({...order,state:"COMPLETED",table_flows,history:events,allowed_actions:[]}))));
 mount(`/configuration/release-orders/${id}`);const stage=await screen.findByRole("region",{name:"发布阶段"});
 expect(await within(stage).findByText("提交人")).toBeVisible();expect(stage.querySelector("time")).toHaveAttribute("datetime",events[1]!.at);expect(within(stage).queryByText("creator")).not.toBeInTheDocument();
 const nodes=screen.getByRole("list",{name:"items 流程节点"});
 expect(within(nodes).getByText("实例审批人")).toBeVisible();expect(within(nodes).getByText("实例发布人")).toBeVisible();
 const last=within(nodes).getAllByRole("listitem")[2]!;expect(within(last).getByText("完结人")).toBeVisible();expect(last.querySelector("time")).toHaveAttribute("datetime",events[2]!.at);
 expect(within(screen.getByRole("region",{name:"操作历史"})).getByText("完结了发布单")).toBeVisible();
});

it("重新准备未知后明确拒绝，必须读取最新版本和差异才可用新键重建",async()=>{
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["cancel","execute","reprepare"]};let current=approved;
 const writes:RequestInit[]=[];let previews=0;
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);if(path.endsWith("/people"))return json({people:{}});
  if(path.endsWith("/preview")){previews++;return json({items:order.items.map(item=>({...item,expected_record_version:previews===1?"0":"2",fields:item.fields.map(field=>field.name==="label"?{...field,before:previews===1?"original":"fresh reviewed value"}:field)}))})}
  if(path.endsWith("/reprepare")){writes.push(init!);if(writes.length===1)throw new TypeError("unknown");if(writes.length===2){current={...approved,version:"4"};return json({error:{code:"release_version_conflict",message:"updated",request_id:"changed"}},409)}return json({...order,id:repreparedID,copied_from_id:id})}
  return json(current);
 })));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"重新准备"}));await user.click(screen.getByRole("button",{name:"读取最新配置"}));await user.click(screen.getByRole("button",{name:"继续重新准备"}));await user.click(screen.getByRole("button",{name:"取消旧单并创建新草稿"}));await originalButton("取消旧单并创建新草稿");
 page.unmount();page=mount(`/configuration/release-orders/${id}`);await repeatOriginal(user,"重新准备","取消旧单并创建新草稿");await screen.findByRole("button",{name:"查看最新状态与配置"});
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
  if(path.endsWith("/preview"))return json({items:order.items});
  if(path.endsWith("/reprepare")){writes.push(init!);if(writes.length===1){signedIn=false;return json({error:{code:"session_invalid",message:"expired",request_id:"expired"}},401)}return json({...order,id:repreparedID,copied_from_id:id})}
  return json(approved);
 }));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"重新准备"}));await user.click(screen.getByRole("button",{name:"读取最新配置"}));await user.click(screen.getByRole("button",{name:"继续重新准备"}));await user.click(screen.getByRole("button",{name:"取消旧单并创建新草稿"}));
 expect(await screen.findByRole("dialog",{name:"登录会话已中断"})).toBeVisible();expect(screen.getByRole("alertdialog",{hidden:true})).not.toBeVisible();
 await user.type(await screen.findByLabelText("用户名"),"test.user");await user.type(screen.getByLabelText("密码"),"correct horse battery staple");await user.click(screen.getByRole("button",{name:"登录"}));
 await waitFor(()=>expect(screen.queryByRole("dialog",{name:"登录会话已中断"})).not.toBeInTheDocument());
 if(otherAccount){expect(screen.queryByRole("region",{name:"待处理发布请求"})).not.toBeInTheDocument();expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();expect(writes).toHaveLength(1)}
 else {await user.click(await originalButton("取消旧单并创建新草稿"));await waitFor(()=>expect(writes).toHaveLength(2));expect(writes[1]!.body).toBe(writes[0]!.body);expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"))}
});

it.each(["DRAFT","PENDING_APPROVAL"] as const)("冷账号在%s读取持久编辑请求后仍可原键重推，不倒退主单",async(state)=>{
 const account={...testAdminIdentity.account,id:state==="DRAFT"?"deadbeef-dead-4000-8000-000000000001":"deadbeef-dead-4000-8000-000000000002"};
 const body=JSON.stringify({title:order.title,expected_version:"1",items:[{...order.items[0],content:{label:"cold stored intent"}}]});
 // Seed the browser boundary directly: this account has never hydrated the
 // module mirror, unlike the default fixture account in test setup.
 await new Promise<void>((resolve,reject)=>{const opening=indexedDB.open("rcc-release-requests",1);opening.onsuccess=()=>{const db=opening.result,tx=db.transaction("accounts","readwrite");tx.objectStore("accounts").put([{scope:`edit:${id}`,method:"PUT",path:`/api/v1/release-orders/${id}`,body,key:"cold-original-key",label:"冷启动原申请"}],account.id);tx.oncomplete=()=>{db.close();resolve()};tx.onabort=()=>reject(tx.error)};opening.onerror=()=>reject(opening.error)});
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account});
  if(init?.method==="PUT"){writes.push(init);return json({...order,applicant_id:account.id,version:"2"})}
  return String(input).endsWith("/people")?json({people:{}}):json({...order,applicant_id:account.id,state,version:state==="DRAFT"?"1":"3",allowed_actions:state==="DRAFT"?["edit"]:["approve"]});
 }));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await originalButton("编辑草稿");expect(writes).toHaveLength(0);
 await repeatOriginal(user,"编辑草稿","保存草稿修改");
 await waitFor(()=>expect(writes).toHaveLength(1));expect(writes[0]!.body).toBe(body);expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBe("cold-original-key");
 await waitFor(()=>expect(pendingReleaseRequests(account.id)).toHaveLength(0));
 if(state==="PENDING_APPROVAL")expect(await screen.findByText("待审批",{selector:'[aria-label="发布单状态"]'})).toBeVisible();
});

it("另一窗口留下同范围原请求时锁定当前输入，恢复后保留本窗口未保存内容",async()=>{
 const writes:RequestInit[]=[];vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{if(init?.method==="PUT")writes.push(init);return String(input).endsWith("/people")?json({people:{}}):json(order)})));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));await user.clear(screen.getByLabelText("label 申请值"));await user.type(screen.getByLabelText("label 申请值"),"this window unsaved");
 const body=JSON.stringify({title:order.title,expected_version:"1",items:[{...order.items[0],content:{label:"other window request"}}]});
 await act(async()=>{await rememberReleaseRequest(testAdminIdentity.account.id,{scope:`edit:${id}`,method:"PUT",path:`/api/v1/release-orders/${id}`,body,key:"other-window-key",label:"另一窗口原请求"})});
 expect(screen.getByLabelText("label 申请值")).toBeDisabled();expect(screen.getByLabelText("label 申请值")).toHaveValue("this window unsaved");
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));await waitFor(()=>expect(writes).toHaveLength(1));
 await waitFor(()=>expect(screen.getByLabelText("label 申请值")).toBeEnabled());expect(screen.getByLabelText("label 申请值")).toHaveValue("this window unsaved");expect(writes[0]!.body).toBe(body);
});

it("多表审阅按所属表读取一次当前标签，申请与两次实际结果不串表",async()=>{
 const reads:string[]=[];
 fieldPolicyResponse=tableName=>{
  reads.push(tableName);
  const configuration=defaultFieldPolicies(tableName,[{name:"label",type:"string",nullable:true}]);
  configuration.fields[0]!.effective={...configuration.fields[0]!.effective,display_name:`${tableName}字段`,is_visible:false,ui_type:"select",ui_options:{options:[{label:`${tableName}旧值`,value:"original"},{label:`${tableName}新值`,value:"proposal"}]},enabled:true};
  return json(configuration);
 };
 const items=["items","other","items"].map((table_name,index)=>({...order.items[0]!,table_name,detail_id:String(index+1).repeat(32),id:String(index+1)}));
 const row=(value:string)=>({format:"rcc-admin-mysql-row-v1",schema_digest:"a".repeat(64),deleted:false,fields:[{name:"label",type:"varchar(40)",encoding:"text",value}],checksum:"b".repeat(64)});
 const commands=items.map((item,index)=>({order_id:id,sequence:String(index===2?2:1),table_name:item.table_name,table_version:"1",operation:"MODIFY",id:item.id,record_version:"1",before:row("original"),final:row("proposal")}));
 const current=withExecution(withExecution({...order,items,state:"ROLLED_BACK",allowed_actions:[]},"PUBLICATION",commands),"ROLLBACK",commands.map(command=>({...command,before:command.final,final:command.before})));
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json(current))));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByText("other新值")).toBeVisible();
 const actual=screen.getByRole("region",{name:/明细 2 other 实际结果/});
 expect(within(actual).getByText("other字段")).toBeVisible();expect(within(actual).queryByText("items字段")).not.toBeInTheDocument();
 expect(within(actual).getByLabelText("真实值：proposal")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"恢复结果"}));
 expect(within(screen.getByRole("region",{name:/明细 2 other 实际结果/})).getByText("other旧值")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"申请内容"}));
 const second=screen.getByRole("region",{name:"明细 2"});
 await user.click(within(second).getByText(/明细 2 · other/));
 expect(within(second).getByText("other字段")).toBeVisible();expect(within(second).getByText("other新值")).toBeVisible();
 expect(reads.sort()).toEqual(["items","other"]);
});

it("版本冲突的最新草稿首次加入另一表时读取其标签并保留本窗口输入",async()=>{
 let updated=false;const reads:string[]=[];
 const newest={...order,version:"2",items:[order.items[0]!,{...order.items[0]!,detail_id:"2".repeat(32),table_name:"other"}]};
 fieldPolicyResponse=tableName=>{
  reads.push(tableName);const configuration=defaultFieldPolicies(tableName,[{name:"label",type:"string",nullable:true}]);
  configuration.fields[0]!.effective={...configuration.fields[0]!.effective,display_name:`${tableName}最新字段`,ui_type:"select",ui_options:{options:[{label:`${tableName}当前申请`,value:"proposal"}]},enabled:true};return json(configuration);
 };
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(init?.method==="PUT"){updated=true;return json({error:{code:"release_version_conflict",message:"changed",request_id:"new-table"}},409)}
  if(String(input).endsWith("/people"))return json({people:{}});
  return json(updated?newest:order);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 await user.clear(screen.getByLabelText("label 申请值"));await user.type(screen.getByLabelText("label 申请值"),"本窗口输入");
 await user.click(screen.getByRole("button",{name:"保存草稿修改"}));
 await user.click(await screen.findByRole("button",{name:"查看最新发布单"}));
 const dialog=screen.getByRole("dialog",{name:"编辑多表草稿"});
 const second=await within(dialog).findByRole("region",{name:"明细 2"});
 await user.click(within(second).getByText(/明细 2 · other/));
 expect(await within(second).findByText("other最新字段")).toBeVisible();expect(within(second).getByText("other当前申请")).toBeVisible();
 expect(screen.getByLabelText("label 申请值")).toHaveValue("本窗口输入");expect(new Set(reads.filter(table=>table==="other"))).toEqual(new Set(["other"]));
});

it.each(["saved","unavailable"] as const)("发布异常只报错，原动作手动重复保持原身份与正文（历史%s）",async(history)=>{
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["execute","reprepare"]};let current=approved;const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/execute")){writes.push(init!);if(writes.length===1)return json({error:{code:"duplicate_key",message:"constraint failed",request_id:"failed-once",execution_outcome:"not_committed",failure_history:history}},409);current={...approved,state:"SUCCEEDED",version:"4",allowed_actions:["complete"]};return json(current)}
  if(String(input).endsWith("/people"))return json({people:{}});return json(current);
 })));
 const user=userEvent.setup();const page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"执行发布"}));await user.click(screen.getByRole("button",{name:"确认发布到数据库"}));
 expect(await screen.findByText(history==="saved"?"本次执行未提交，失败已记录在操作历史中。":"本次执行未提交，但未能确认失败历史已保存。")).toBeVisible();
 expect(screen.queryByRole("button",{name:/恢复原发布请求|使用原请求重试|确认执行结果/})).not.toBeInTheDocument();
 expect(screen.getByRole("button",{name:"确认发布到数据库"})).toBeEnabled();
 await new Promise(resolve=>setTimeout(resolve,50));expect(writes).toHaveLength(1);
 await user.click(screen.getByRole("button",{name:"确认发布到数据库"}));await screen.findByText("已发布待完结",{selector:'[aria-label="发布单状态"]'});
 expect(writes).toHaveLength(2);expect(writes[1]!.body).toBe(writes[0]!.body);expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));page.unmount();
});

it("HTTP成功后的IndexedDB清理失败仍读取真实主单且不会变成业务失败",async()=>{
 const approved={...order,state:"APPROVED",version:"3",allowed_actions:["execute"]};let current=approved;const writes:RequestInit[]=[];
 const remove=vi.spyOn(IDBObjectStore.prototype,"delete").mockImplementation(()=>{throw new DOMException("cleanup unavailable","UnknownError")});
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  if(String(input).endsWith("/execute")){writes.push(init!);current={...approved,state:"SUCCEEDED",version:"4",allowed_actions:["complete"]};return json(current)}
  if(String(input).endsWith("/people"))return json({people:{}});return json(current);
 })));
 try{
  const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);await user.click(await screen.findByRole("button",{name:"执行发布"}));await user.click(screen.getByRole("button",{name:"确认发布到数据库"}));
  await screen.findByText("已发布待完结",{selector:'[aria-label="发布单状态"]'});expect(screen.queryByText(/本次执行未提交/)).not.toBeInTheDocument();expect(writes).toHaveLength(1);
  expect(pendingReleaseRequests(testAdminIdentity.account.id)).toHaveLength(1);await waitFor(()=>expect(screen.getByRole("button",{name:"完结发布单"})).toBeEnabled());
  remove.mockRestore();await user.click(screen.getByRole("button",{name:"执行发布"}));await user.click(screen.getByRole("button",{name:"确认发布到数据库"}));
  await waitFor(()=>expect(writes).toHaveLength(2));expect(writes[1]!.body).toBe(writes[0]!.body);expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
 }finally{remove.mockRestore()}
});


it("冷账号日志尚未读出时新建原操作等待，读出后保持完整原请求",async()=>{
 const account={...testAdminIdentity.account,id:"deadbeef-dead-4000-8000-000000000086"};
 const body=JSON.stringify({title:"冷启动原草稿",items:[{...order.items[0],table_name:"items",id:"1",content:{label:"cold create intent"}},{...order.items[0],table_name:"other",id:"1",content:{label:"other cold intent"}}]});
 await new Promise<void>((resolve,reject)=>{const opening=indexedDB.open("rcc-release-requests",1);opening.onsuccess=()=>{const db=opening.result,tx=db.transaction("accounts","readwrite");tx.objectStore("accounts").put([{scope:"create",method:"POST",path:"/api/v1/release-orders",body,key:"cold-create-key",label:"冷启动原申请"}],account.id);tx.oncomplete=()=>{db.close();resolve()};tx.onabort=()=>reject(tx.error)};opening.onerror=()=>reject(opening.error)});
 const open=indexedDB.open.bind(indexedDB);let releaseRead:()=>void=()=>{};
 vi.spyOn(indexedDB,"open").mockImplementationOnce((...args)=>{
  const request=open(...args);
  request.addEventListener("success",event=>{event.stopImmediatePropagation();releaseRead=()=>request.onsuccess?.call(request,event)},{once:true});
  return request;
 });
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  const path=String(input);if(path.startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account});
  if(init?.method==="POST"){writes.push(init);return json({...order,title:"冷启动原草稿",applicant_id:account.id})}
  if(path.startsWith("/api/v1/release-orders?"))return json({orders:[],next_cursor:""});
  if(path.endsWith("/people"))return json({people:{}});
  if(path==="/api/v1/table-policies")return json([]);return json({...order,applicant_id:account.id});
 }));
 const user=userEvent.setup();mount("/configuration/release-orders");
 const create=await screen.findByRole("button",{name:"新建草稿"});expect(create).toBeDisabled();await user.click(create);expect(screen.queryByRole("dialog")).not.toBeInTheDocument();expect(writes).toHaveLength(0);
 await act(async()=>{releaseRead()});await waitFor(()=>expect(create).toBeEnabled());await user.click(create);
 await user.click(await screen.findByText("查看原申请内容"));expect(screen.getByText("明细 1 · items · MODIFY · 记录 1")).toBeVisible();expect(screen.getByText("明细 2 · other · MODIFY · 记录 1")).toBeVisible();
 await user.click(await screen.findByRole("button",{name:"确认并保存草稿"}));await waitFor(()=>expect(writes).toHaveLength(1));
 expect(writes[0]!.body).toBe(body);expect(new Headers(writes[0]!.headers).get("Idempotency-Key")).toBe("cold-create-key");
});

it("后台 header 换版本和撤销编辑动作不重挂载输入，也不替换已留存原请求",async()=>{
 let current={...order};
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async input=>String(input).endsWith("/people")?json({people:{}}):json(current))));
 const user=userEvent.setup();const {client}=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"编辑草稿"}));
 const input=await screen.findByLabelText("label 申请值");await user.clear(input);await user.type(input,"本窗口独立输入");
 const retained={scope:`edit:${id}`,path:`/api/v1/release-orders/${id}`,method:"PUT" as const,body:JSON.stringify({title:order.title,expected_version:"1",changes:{upserts:[{detail_id:order.items[0]!.detail_id,table_name:"items",operation:"MODIFY",id:"1",expected_record_version:"0",content:{label:"已发原输入"}}]}}),key:"immutable-editor-request",label:"保留原保存"};
 await act(async()=>{await rememberReleaseRequest(testAdminIdentity.account.id,retained)});
 await act(async()=>{current={...order,version:"2",allowed_actions:[],items:order.items.map(item=>({...item,content:{label:"他人新内容"}}))};await client.refetchQueries({queryKey:["release-order",id]})});
 expect(screen.getByLabelText("label 申请值")).toBe(input);expect(input).toHaveValue("本窗口独立输入");expect(input).toBeDisabled();
 expect(pendingReleaseRequests(testAdminIdentity.account.id)).toEqual([retained]);
 expect(await screen.findByText("当前身份或发布单状态不允许编辑，已输入内容保留。")).toBeVisible();
});

it("VIEWER 成员按服务端资格确认表范围，部分批准不能显示整单已批准", async () => {
 const roles=[{id:"role-products",name:"商品运营"}];
 const tables=["items","prices"];
 const approvals=tables.map(table_name=>({table_name,roles,state:"PENDING"}));
 const context={revision:"scope-1",tables:tables.map(table_name=>({table_name,mode:"ROLE",reason:"由商品运营成员审批",can_approve:table_name==="items"})),approvable_tables:["items"]};
 let current={...order,applicant_id:"someone-else",state:"PENDING_APPROVAL",version:"2",allowed_actions:["approve","reject"],approvals,approval_context:context,items:[...order.items,{...order.items[0]!,detail_id:"2".repeat(32),table_name:"prices"}]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  if(String(input).startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:["VIEWER"]}});
  if(String(input).endsWith("/people"))return json({people:{}});
  if(String(input).endsWith("/approve")){writes.push(init!);current={...current,version:"3",allowed_actions:[],approvals:[{...approvals[0]!,state:"APPROVED"},approvals[1]!],approval_context:{...context,revision:"scope-2",approvable_tables:[]},history:[...order.history,{action:"APPROVE",actor_id:testAdminIdentity.account.id,at:"2026-09-11T02:00:00Z",version:"3",reason:"只确认负责表"}]};return json(current)}
  return json(current);
 }));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"批准发布单"}));
 expect(screen.getByRole("region",{name:"本次审批范围"})).toHaveTextContent("items");
 expect(screen.getByRole("region",{name:"本次审批范围"})).not.toHaveTextContent("prices");
 await user.type(screen.getByLabelText("审批意见"),"只确认负责表");await user.click(screen.getByRole("button",{name:"确认批准"}));
 await waitFor(()=>expect(writes).toHaveLength(1));
 expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"2",reason:"只确认负责表",confirmed_tables:["items"],expected_approval_revision:"scope-1"});
 await waitFor(()=>expect(screen.getByRole("region",{name:"逐表审批进度"})).toHaveTextContent("已通过 1 / 2 表"));
 expect(within(screen.getByRole("region",{name:"发布阶段"})).getByRole("heading",{name:"待审批"})).toBeVisible();
});

it("审批资格改变而单据版本不变时保留原范围，明确审阅后才扩展范围", async () => {
 const tables=["items","prices"],roles=[{id:"role-team",name:"联合审批"}];
 const initialContext={revision:"members-before",tables:tables.map(table_name=>({table_name,mode:"ROLE",reason:"角色成员审批",can_approve:table_name==="items"})),approvable_tables:["items"]};
 let current={...order,applicant_id:"independent-applicant",state:"PENDING_APPROVAL",version:"2",allowed_actions:["approve","reject"],approvals:tables.map(table_name=>({table_name,roles,state:"PENDING"})),approval_context:initialContext,items:[...order.items,{...order.items[0]!,detail_id:"2".repeat(32),table_name:"prices"}]};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.startsWith("/api/v1/auth/"))return json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:["VIEWER"]}});
  if(path.endsWith("/people"))return json({people:{}});
  if(path.endsWith("/approve")){writes.push(init!);if(writes.length===1)return json({error:{code:"release_approval_conflict",message:"review changed scope",request_id:"scope-conflict"}},409);current={...current,state:"APPROVED",version:"3",allowed_actions:[]};return json(current)}
  if(path.includes("/release-orders?"))return json({orders:[current],next_cursor:""});
  return json(current);
 }));
 const user=userEvent.setup();let page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"批准发布单"}));await user.type(screen.getByLabelText("审批意见"),"原审批意见必须保留");
 current={...current,approval_context:{revision:"members-after",tables:tables.map(table_name=>({table_name,mode:"ROLE",reason:"新成员资格",can_approve:true})),approvable_tables:tables}};
 await act(async()=>{await page.client.invalidateQueries({queryKey:["release-order",id]})});
 expect(screen.getByRole("region",{name:"本次审批范围"})).not.toHaveTextContent("prices");
 await user.click(screen.getByRole("button",{name:"确认批准"}));
 await waitFor(()=>expect(pendingReleaseRequests(testAdminIdentity.account.id)[0]?.rejection).toBe("release_approval_conflict"));
 expect(JSON.parse(String(writes[0]!.body))).toEqual({expected_version:"2",reason:"原审批意见必须保留",confirmed_tables:["items"],expected_approval_revision:"members-before"});
 page.unmount();page=mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByText("查看原申请内容"));
 expect(await screen.findByText("审批意见：原审批意见必须保留")).toBeVisible();
 expect(screen.getByText("原确认表范围：items")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"查看最新状态与配置"}));
 expect(await screen.findByRole("region",{name:"本次审批范围"})).toHaveTextContent("prices");
 await user.click(await screen.findByRole("button",{name:"确认按最新状态批准发布单"}));
 await waitFor(()=>expect(writes).toHaveLength(2));
 expect(JSON.parse(String(writes[1]!.body))).toEqual({expected_version:"2",reason:"原审批意见必须保留",confirmed_tables:tables,expected_approval_revision:"members-after"});
 expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"));
});

it.each(["ADMIN","VIEWER"])("%s 没有服务端表资格时旧动作也不能提供新审批", async role=>{
 const pending={...order,applicant_id:"another-person",state:"PENDING_APPROVAL",allowed_actions:["approve","reject"],approvals:[{table_name:"items",roles:[],state:"PENDING"}],approval_context:{revision:"no-eligibility",tables:[{table_name:"items",mode:"ROLE",reason:"由其他角色成员审批",can_approve:false}],approvable_tables:[]}};
 vi.stubGlobal("fetch",vi.fn(async input=>String(input).startsWith("/api/v1/auth/")?json({...testAdminIdentity,account:{...testAdminIdentity.account,roles:[role]}}):String(input).endsWith("/people")?json({people:{}}):json(pending)));
 mount(`/configuration/release-orders/${id}`);
 expect(await screen.findByRole("region",{name:"逐表审批进度"})).toHaveTextContent("由其他角色成员审批");
 expect(screen.queryByRole("button",{name:"批准发布单"})).not.toBeInTheDocument();
 expect(screen.queryByRole("button",{name:"拒绝发布单"})).not.toBeInTheDocument();
});

it.each([false,true])("无人审批明确拒绝后解除原提交并在窗口显示最新问题表（曾未知=%s）",async(previousUnknown)=>{
 const available={revision:"available-before-submit",tables:[{table_name:"items",mode:"ROLE",reason:"独立成员可审批",can_approve:false}],approvable_tables:[]};
 let current={...order,allowed_actions:["edit","submit","cancel"],approvals:[{table_name:"items",roles:[{id:"review-role",name:"审批组"}],state:"PENDING"}],approval_context:available};
 const writes:RequestInit[]=[];
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input,init)=>{
  const path=String(input);
  if(path.endsWith("/people"))return json({people:{}});
  if(path.endsWith("/submit")){
   writes.push(init!);
   if(previousUnknown&&writes.length===1)throw new TypeError("response unavailable");
   current={...current,approval_context:{revision:"members-gone",tables:[{table_name:"items",mode:"UNAVAILABLE",reason:"缺少独立审批人，请补充启用的角色成员或非申请人 ADMIN",can_approve:false}],approvable_tables:[]}};
   return json({error:{code:"release_approver_unavailable",message:"independent approver required for [items]",request_id:"no-approver"}},422);
  }
  return path.includes("/release-orders?")?json({orders:[current],next_cursor:""}):json(current);
 })));
 const user=userEvent.setup();mount(`/configuration/release-orders/${id}`);
 await user.click(await screen.findByRole("button",{name:"提交审批"}));
 await user.click(screen.getByRole("button",{name:"确认提交审批"}));
 if(previousUnknown){
  expect(await screen.findByText("原请求与意见已保留；再次点击同一操作将提交原请求。")).toBeVisible();
  await user.click(await originalButton("确认提交审批"));
 }
 expect(await screen.findByText("有表缺少独立审批人，请查看各表审批安排并补充合格人员后重新提交。")).toBeVisible();
 await waitFor(()=>expect(pendingReleaseRequests(testAdminIdentity.account.id)).toHaveLength(0));
 const dialog=screen.getByRole("dialog",{name:"提交审批"});
 expect(within(dialog).getByRole("region",{name:"提交审批安排"})).toHaveTextContent("items");
 expect(within(dialog).getByText("暂无独立审批人",{exact:true})).toBeVisible();
 expect(within(dialog).queryByText(/原请求与意见已保留/)).not.toBeInTheDocument();
 if(previousUnknown){expect(writes[1]!.body).toBe(writes[0]!.body);expect(new Headers(writes[1]!.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]!.headers).get("Idempotency-Key"))}
 await user.click(within(dialog).getAllByRole("button",{name:"关闭"}).at(-1)!);
 expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
 await user.click(screen.getByRole("button",{name:"更多操作"}));
 expect(screen.getByRole("menuitem",{name:"取消草稿"})).toBeEnabled();
});
