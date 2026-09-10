import {QueryClient,QueryClientProvider} from "@tanstack/react-query";
import {render,screen,waitFor,within} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {afterEach,describe,expect,it,vi} from "vitest";
import {AppRoutes} from "../../app";
import {TestRouter} from "../../test/TestRouter";
import {withAdminSession} from "../../test/account-session";
import {ToastProvider} from "../../components/ui/Toast";

const audit={creator:"admin",modifier:"admin",created_at:"2026-09-11T00:00:00Z",updated_at:"2026-09-11T00:00:00Z",version:"1"};
const templates=["STANDARD","EMERGENCY","EMERGENCY"].map((type,index)=>({...audit,code:`template_${index}_v1`,name:`模板${index}`,description:"",type,enabled:true,monitor_list:[],node_list:(type==="STANDARD"?["APPROVAL","PUBLICATION","COMPLETION"]:["PUBLICATION","COMPLETION"]).map(kind=>({code:kind.toLowerCase(),type:kind,name:kind,required_role:kind==="APPROVAL"?"TABLE_APPROVER":"PUBLISHER"}))}));
const rows=[{...audit,table_name:"items",type:"STANDARD",template_code:"template_0_v1",template_name:"模板0",enabled:true,template_enabled:true},{...audit,table_name:"items",type:"EMERGENCY",template_code:"template_1_v1",template_name:"模板1",enabled:true,template_enabled:true}];
const json=(value:unknown,status=200)=>new Response(JSON.stringify(value),{status,headers:{"Content-Type":"application/json"}});
function mount(write:(init:RequestInit)=>Promise<Response>, read=async()=>json({associations:rows})){
 vi.stubGlobal("fetch",withAdminSession(vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
  const url=String(input);
  if(init?.method==="PUT")return write(init);
  if(url.endsWith("/items/release-templates"))return read();
  if(url.endsWith("/release-templates"))return json({templates});
  if(url.endsWith("/table-policies"))return json({policies:[]});
  if(url.endsWith("/database-tables"))return json({tables:[]});
  throw new Error(url);
 })));
 render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}})}><TestRouter initialEntries={["/platform/table-policies/items?mode=templates"]}><ToastProvider><AppRoutes/></ToastProvider></TestRouter></QueryClientProvider>);
}
afterEach(()=>vi.unstubAllGlobals());
describe("table release template settings",()=>{
 it("offers only same-type templates and allows standard disable",async()=>{
  const writes:RequestInit[]=[];mount(async init=>{writes.push(init);return json({...rows[0],enabled:false,version:"2"});});
  const standard=await screen.findByRole("combobox",{name:"常规模板"});expect(within(standard).queryByRole("option",{name:/模板1/})).not.toBeInTheDocument();
  expect(within(screen.getByRole("combobox",{name:"应急模板"})).queryByRole("option",{name:/模板0/})).not.toBeInTheDocument();
  await userEvent.click(screen.getByLabelText("启用常规关联"));await userEvent.click(screen.getByRole("button",{name:"保存常规关联"}));await screen.findByText("常规关联已保存");
  expect(JSON.parse(String(writes[0]?.body))).toEqual({template_code:"template_0_v1",enabled:false,expected_version:"1"});
  expect(screen.queryByRole("button",{name:/停用应急|解绑|删除/})).not.toBeInTheDocument();
 });
 it("keeps selected input on version conflict",async()=>{
  mount(async()=>json({error:{code:"table_release_template_conflict",message:"stale",request_id:"association-conflict"}},409));
  await userEvent.selectOptions(await screen.findByRole("combobox",{name:"应急模板"}),"template_2_v1");await userEvent.click(screen.getByRole("button",{name:"保存应急关联"}));
  await screen.findByText(/关联已被其他管理员修改/);expect(screen.getByLabelText("应急模板")).toHaveValue("template_2_v1");expect(within(screen.getByRole("region",{name:"应急发布关联"})).getByRole("alert")).toHaveTextContent("association-conflict");
 });
 it("freezes and manually replays exactly the original request across an intervening denial",async()=>{
  const writes:RequestInit[]=[];mount(async init=>{writes.push(init);if(writes.length===1)throw new TypeError("response lost");if(writes.length===2)return json({error:{code:"permission_denied",message:"revoked",request_id:"revoked"}},403);return json({...rows[1],template_code:"template_2_v1",template_name:"模板2",version:"2"});});
  await userEvent.selectOptions(await screen.findByRole("combobox",{name:"应急模板"}),"template_2_v1");await userEvent.click(screen.getByRole("button",{name:"保存应急关联"}));await screen.findByText(/保存结果未知/);
  expect(writes).toHaveLength(1);expect(screen.getByLabelText("应急模板")).toBeDisabled();
  await userEvent.click(screen.getByRole("button",{name:"重推应急原请求"}));await waitFor(()=>expect(writes).toHaveLength(2));
  await userEvent.click(screen.getByRole("button",{name:"重推应急原请求"}));await screen.findByText("应急关联已保存");
  for(const write of writes.slice(1)){expect(write.body).toBe(writes[0]?.body);expect(new Headers(write.headers).get("Idempotency-Key")).toBe(new Headers(writes[0]?.headers).get("Idempotency-Key"));}
 });
 it("unlocks a rejected original request after a definite version conflict and retains its input",async()=>{
  const writes:RequestInit[]=[];let current=rows;
  mount(async init=>{writes.push(init);if(writes.length===1)return json({error:{code:"dependency_unavailable",message:"unavailable",request_id:"first"}},503);if(writes.length===2)return json({error:{code:"table_release_template_conflict",message:"stale",request_id:"conflict"}},409);return json({...rows[1],template_code:"template_2_v1",version:"4"});},async()=>json({associations:current}));
  await userEvent.selectOptions(await screen.findByLabelText("应急模板"),"template_2_v1");await userEvent.click(screen.getByRole("button",{name:"保存应急关联"}));await screen.findByText(/保存结果未知/);
  await userEvent.click(screen.getByRole("button",{name:"重推应急原请求"}));await screen.findByText(/关联已被其他管理员修改/);
  expect(screen.getByLabelText("应急模板")).toBeEnabled();expect(screen.getByLabelText("应急模板")).toHaveValue("template_2_v1");
  current=rows.map(row=>({...row,version:"3"}));await userEvent.click(screen.getByRole("button",{name:"读取最新关联并保留输入"}));await screen.findByText(/已读取最新关联/);
  await userEvent.click(screen.getByRole("button",{name:"保存应急关联"}));await waitFor(()=>expect(writes).toHaveLength(3));expect(JSON.parse(String(writes[2]?.body)).expected_version).toBe("3");expect(new Headers(writes[2]?.headers).get("Idempotency-Key")).not.toBe(new Headers(writes[0]?.headers).get("Idempotency-Key"));
 });
 it("reads current state after replay instead of displaying a historical result as current",async()=>{
  let current=rows;const writes:RequestInit[]=[];
  mount(async init=>{writes.push(init);if(writes.length===1){current=rows.map(row=>row.type==="EMERGENCY"?{...row,version:"4"}:row);throw new TypeError("lost");}return json({...rows[1],template_code:"template_2_v1",template_name:"模板2",version:"2"});},async()=>json({associations:current}));
  await userEvent.selectOptions(await screen.findByLabelText("应急模板"),"template_2_v1");await userEvent.click(screen.getByRole("button",{name:"保存应急关联"}));await screen.findByText(/保存结果未知/);await userEvent.click(screen.getByRole("button",{name:"重推应急原请求"}));
  await waitFor(()=>expect(screen.getByLabelText("应急模板")).toHaveValue("template_1_v1"));expect(screen.getByRole("region",{name:"应急发布关联"})).toHaveTextContent("v4");
  await userEvent.selectOptions(screen.getByLabelText("应急模板"),"template_2_v1");await userEvent.click(screen.getByRole("button",{name:"保存应急关联"}));await waitFor(()=>expect(writes).toHaveLength(3));expect(JSON.parse(String(writes[2]?.body)).expected_version).toBe("4");
 });

});
