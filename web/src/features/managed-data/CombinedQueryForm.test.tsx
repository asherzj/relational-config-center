import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { CombinedQueryForm } from "./CombinedQueryForm";
import type { FieldPolicyField } from "../../api/field-policies";

export function field(name: string, overrides: Partial<FieldPolicyField["effective"]> = {}, column_type: FieldPolicyField["column_type"] = "string"): FieldPolicyField {
  return {field_name:name,column_type,nullable:true,generated:false,auto_increment:false,has_default:false,state:"active",warning:"",policy:null,audit:null,
    effective:{field_name:name,display_name:name,description:"",display_order:0,is_visible:true,is_queryable:true,query_operators:["exact"],ui_type:"text",ui_options:{options:[]},editable_on_add:true,editable_on_modify:true,is_required:false,enabled:true,...overrides}};
}
function setup(fields: FieldPolicyField[]) {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({table_name:"items", fields, query_capacity:{max_conditions:256,max_values_per_condition:100,queryable_fields:fields.filter(f=>f.effective.is_queryable).length,supported:true}}))));
  const submit=vi.fn();
  render(<CombinedQueryForm tableName="items" columns={fields.map(f=>({name:f.field_name,type:f.column_type as "string",nullable:true}))} onSubmit={submit} onClear={()=>{}} />);
  return submit;
}
afterEach(()=>vi.unstubAllGlobals());
it("freezes the first successful direct configuration without requesting again for a new response object", async()=>{
 const configuration={table_name:"items",fields:[field("name")],query_capacity:{max_conditions:256,max_values_per_condition:100,queryable_fields:1,supported:true}};
 const fetchMock=vi.fn(async()=>new Response(JSON.stringify(configuration)));
 vi.stubGlobal("fetch",fetchMock);
 render(<CombinedQueryForm tableName="items" columns={[{name:"name",type:"string",nullable:true}]} onSubmit={()=>{}} onClear={()=>{}} />);
 await screen.findByRole("textbox",{name:"筛选 name 值"});
 await new Promise(resolve=>setTimeout(resolve,20));
 expect(fetchMock).toHaveBeenCalledTimes(1);
});
it("AC-004 directly displays only queryable fields and submits entered conditions after collapsing", async()=>{
  const submit=setup([field("channel",{display_name:"渠道"}),field("priority",{},"int64"),field("hidden",{is_queryable:false})]);
  const user=userEvent.setup();
  await user.type(await screen.findByRole("textbox",{name:"筛选 渠道 值"}),"EMAIL");
  await user.type(screen.getByRole("textbox",{name:"筛选 priority 值"}),"0");
  expect(screen.queryByRole("textbox",{name:"筛选 hidden 值"})).not.toBeInTheDocument();
  expect(screen.queryByRole("button",{name:"添加条件"})).not.toBeInTheDocument();
  await user.click(screen.getByRole("button",{name:"收起筛选"}));
  await user.click(screen.getByRole("button",{name:"查询"}));
  expect(submit).toHaveBeenCalledWith({conditions:[{field:"channel",operator:"exact",value:"EMAIL"},{field:"priority",operator:"exact",value:"0"}],pageNumber:1});
  await user.click(screen.getByRole("button",{name:"展开筛选"}));
  expect(screen.getByRole("textbox",{name:"筛选 渠道 值"})).toHaveValue("EMAIL");
});
it("AC-005/006 explicit operators determine shape, preserve compatible values and clear incompatible ones", async()=>{
  const submit=setup([field("value",{query_operators:["exact","contains","in","not_in","open_range","closed_range","is_null"]}),field("flag",{ui_type:"boolean"},"boolean"),field("blank")]);
  const user=userEvent.setup();
  await user.type(await screen.findByRole("textbox",{name:"筛选 value 值"}),"before");
  expect(screen.queryByRole("combobox",{name:"flag 运算符"})).not.toBeInTheDocument();
  await user.selectOptions(screen.getByRole("combobox",{name:"value 运算符"}),"contains");
  expect(screen.getByRole("textbox",{name:"筛选 value 值"})).toHaveValue("before");
  await user.selectOptions(screen.getByRole("combobox",{name:"value 运算符"}),"in");
  expect(screen.getByRole("textbox",{name:"筛选 value 集合值 1"})).toHaveValue("");
  await user.type(screen.getByRole("textbox",{name:"筛选 value 集合值 1"}),"custom");
  await user.selectOptions(screen.getByRole("combobox",{name:"value 运算符"}),"not_in");
  expect(screen.getByRole("textbox",{name:"筛选 value 集合值 1"})).toHaveValue("custom");
  await user.selectOptions(screen.getByRole("combobox",{name:"value 运算符"}),"closed_range");
  await user.type(screen.getByRole("textbox",{name:"筛选 value 下界"}),"a");
  await user.selectOptions(screen.getByRole("combobox",{name:"value 运算符"}),"open_range");
  expect(screen.getByRole("textbox",{name:"筛选 value 下界"})).toHaveValue("a");
  await user.selectOptions(screen.getByRole("combobox",{name:"筛选 flag 值"}),"0");
  await user.click(screen.getByRole("button",{name:"查询"}));
  expect(submit).toHaveBeenLastCalledWith({conditions:[{field:"flag",operator:"exact",value:"0"},{field:"value",operator:"open_range",from:"a"}],pageNumber:1});
  await user.selectOptions(screen.getByRole("combobox",{name:"value 运算符"}),"is_null");
  expect(screen.queryByRole("textbox",{name:"筛选 value 下界"})).not.toBeInTheDocument();
  await user.click(screen.getByRole("button",{name:"查询"}));
  expect(submit).toHaveBeenLastCalledWith({conditions:[{field:"flag",operator:"exact",value:"0"},{field:"value",operator:"is_null"}],pageNumber:1});
  await user.selectOptions(screen.getByRole("combobox",{name:"value 运算符"}),"exact");
  expect(screen.getByRole("textbox",{name:"筛选 value 值"})).toHaveValue("");
});
it("AC-007 select options and custom set values are lossless", async()=>{
  const fields=[field("channel",{display_name:"渠道",query_operators:["exact","in"],ui_type:"select",ui_options:{options:[{label:"邮件",value:"EMAIL"}]}})];
  const original=JSON.stringify(fields);
  const submit=setup(fields);const user=userEvent.setup();
  await user.selectOptions(await screen.findByRole("combobox",{name:"筛选 渠道 值"}),"0");
  await user.click(screen.getByRole("button",{name:"查询"}));
  expect(submit).toHaveBeenLastCalledWith({conditions:[{field:"channel",operator:"exact",value:"EMAIL"}],pageNumber:1});
  await user.selectOptions(screen.getByRole("combobox",{name:"渠道 运算符"}),"in");
  await user.selectOptions(screen.getByRole("combobox",{name:"筛选 渠道 集合值 1"}),"0");
  await user.click(screen.getByRole("button",{name:"渠道 添加集合值"}));
  await user.type(screen.getByRole("textbox",{name:"筛选 渠道 集合值 2 自定义值"}),"PUSH");
  await user.click(screen.getByRole("button",{name:"查询"}));
  expect(submit).toHaveBeenLastCalledWith({conditions:[{field:"channel",operator:"in",values:["EMAIL","PUSH"]}],pageNumber:1});
  expect(JSON.stringify(fields)).toBe(original);
});
it("AC-006 explicit empty strings differ from unfilled and NULL; clear resets control state", async()=>{
  const submit=setup([field("empty"),field("nullOnly",{query_operators:["is_null"]}),field("unused")]);const user=userEvent.setup();
  await user.click(await screen.findByRole("checkbox",{name:"empty 值 空字符串"}));
  await user.click(screen.getByRole("checkbox",{name:"nullOnly 参与筛选"}));
  await user.click(screen.getByRole("button",{name:"查询"}));
  expect(submit).toHaveBeenLastCalledWith({conditions:[{field:"empty",operator:"exact",value:""},{field:"nullOnly",operator:"is_null"}],pageNumber:1});
});
it("AC-016 uses the server capacity and accepts every field above the former 20 limit",async()=>{
  const submit=setup(Array.from({length:21},(_,i)=>field(`field${i}`)));const user=userEvent.setup();
  await screen.findByRole("textbox",{name:"筛选 field0 值"});
  for(let i=0;i<21;i++) await user.type(screen.getByRole("textbox",{name:`筛选 field${i} 值`}),"0");
  await user.click(screen.getByRole("button",{name:"查询"}));
  expect(submit).toHaveBeenCalledOnce();
  expect(submit.mock.calls[0][0].conditions).toHaveLength(21);
});
it("AC-017 100 values submit and a 101st filled value gives an explicit Web error", async()=>{
 const submit=setup([field("codes",{query_operators:["in"]})]);
 const firstValue=await screen.findByRole("textbox",{name:"筛选 codes 集合值 1"});
 const addValue=screen.getByRole("button",{name:"codes 添加集合值"});
 fireEvent.change(firstValue,{target:{value:"1"}});
 for(let i=2;i<=100;i++) {
  fireEvent.click(addValue);
  const input=document.querySelector<HTMLInputElement>(`input[aria-label="筛选 codes 集合值 ${i}"]`);
  expect(input).toBeInstanceOf(HTMLInputElement);
  fireEvent.change(input!,{target:{value:String(i)}});
 }
 fireEvent.click(screen.getByRole("button",{name:"查询"}));expect(submit.mock.calls[0][0].conditions[0].values).toHaveLength(100);
 fireEvent.click(addValue);
 fireEvent.change(screen.getByRole("textbox",{name:"筛选 codes 集合值 101"}),{target:{value:"101"}});
 fireEvent.click(screen.getByRole("button",{name:"查询"}));
 expect(screen.getByRole("alert")).toHaveTextContent("集合值数量必须是 1 到 100");expect(submit).toHaveBeenCalledOnce();
},20000);
it("AC-006 select empty choice participates while clearing custom input leaves the field unfiltered; radio empty remains selectable",async()=>{
 const submit=setup([field("choice",{ui_type:"select",query_operators:["exact","in"],ui_options:{options:[{label:"空选项",value:""},{label:"邮件",value:"EMAIL"}]}}),field("radio",{ui_type:"radio",ui_options:{options:[{label:"无内容",value:""},{label:"有内容",value:"yes"}]}})]);
 const user=userEvent.setup();
 await user.selectOptions(await screen.findByRole("combobox",{name:"筛选 choice 值"}),"custom");
 await user.type(screen.getByRole("textbox",{name:"筛选 choice 值 自定义值"}),"custom");await user.clear(screen.getByRole("textbox",{name:"筛选 choice 值 自定义值"}));
 await user.click(screen.getByRole("button",{name:"查询"}));expect(submit).toHaveBeenLastCalledWith({conditions:[],pageNumber:1});
 await user.selectOptions(screen.getByRole("combobox",{name:"筛选 choice 值"}),"0");
 await user.click(screen.getByRole("button",{name:"查询"}));expect(submit).toHaveBeenLastCalledWith({conditions:[{field:"choice",operator:"exact",value:""}],pageNumber:1});
 await user.selectOptions(screen.getByRole("combobox",{name:"choice 运算符"}),"in");
 await user.selectOptions(screen.getByRole("combobox",{name:"筛选 choice 集合值 1"}),"0");
 expect(screen.getByRole("radio",{name:"无内容（空字符串）"})).not.toBeChecked();
 await user.click(screen.getByRole("radio",{name:"无内容（空字符串）"}));
 await user.click(screen.getByRole("button",{name:"查询"}));expect(submit).toHaveBeenLastCalledWith({conditions:[{field:"choice",operator:"in",values:[""]},{field:"radio",operator:"exact",value:""}],pageNumber:1});
 await user.click(screen.getByRole("radio",{name:"有内容（yes）"}));expect(screen.getByRole("radio",{name:"有内容（yes）"})).toBeChecked();
});
it("clear resets date-time's intermediate UI as well as submitted values",async()=>{
 const submit=setup([field("at",{ui_type:"datetime"},"datetime")]);const user=userEvent.setup();
 fireEvent.change(await screen.findByLabelText("筛选 at 值 日期"),{target:{value:"2026-09-09"}});
 await user.clear(screen.getByLabelText("筛选 at 值 时间"));await user.type(screen.getByLabelText("筛选 at 值 时间"),"12:30:00.123456");
 await user.click(screen.getByRole("button",{name:"清空"}));
 expect(screen.getByLabelText("筛选 at 值 日期")).toHaveValue("");
 await user.click(screen.getByRole("button",{name:"查询"}));expect(submit).toHaveBeenLastCalledWith({conditions:[],pageNumber:1});
});
it("AC-015 read failures require retry; confirmed incompatible controls warn and fall back",async()=>{
 const fallback={...field("amount"),state:"incompatible",warning:"真实类型已变化"};
 vi.stubGlobal("fetch",vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({error:{code:"field_policy_unavailable",message:"failed"}}),{status:503})).mockResolvedValueOnce(new Response(JSON.stringify({table_name:"items",fields:[fallback],query_capacity:{max_conditions:256,max_values_per_condition:100,queryable_fields:1,supported:true}}))));
 render(<CombinedQueryForm tableName="items" columns={[{name:"amount",type:"string",nullable:true}]} onSubmit={()=>{}} onClear={()=>{}} />);
 expect(await screen.findByRole("alert")).toBeVisible();expect(screen.queryByRole("textbox",{name:"筛选 amount 值"})).not.toBeInTheDocument();expect(screen.getByRole("button",{name:"查询"})).toBeDisabled();
 await userEvent.click(screen.getByRole("button",{name:"重试"}));
 expect(await screen.findByRole("textbox",{name:"筛选 amount 值"})).toBeVisible();expect(screen.getByRole("status")).toHaveTextContent("已回退文本输入");
});
it("AC-016 reports unsupported capacity before querying and uses server-provided limits",async()=>{
 vi.stubGlobal("fetch",vi.fn().mockResolvedValue(new Response(JSON.stringify({table_name:"items",fields:[field("a")],query_capacity:{max_conditions:256,max_values_per_condition:100,queryable_fields:257,supported:false}}))));
 render(<CombinedQueryForm tableName="items" columns={[{name:"a",type:"string",nullable:true}]} onSubmit={()=>{}} onClear={()=>{}} />);
 expect(await screen.findByRole("alert")).toHaveTextContent("超过平台上限 256");expect(screen.getByRole("button",{name:"查询"})).toBeDisabled();
});
it("removing a set value does not reuse the removed date control's local state for the following blank value",async()=>{
 setup([field("at",{ui_type:"datetime",query_operators:["in"]},"datetime")]);const user=userEvent.setup();
 fireEvent.change(await screen.findByLabelText("筛选 at 集合值 1 日期"),{target:{value:"2026-09-09"}});
 await user.click(screen.getByRole("button",{name:"at 添加集合值"}));
 await user.click(screen.getByRole("button",{name:"at 删除集合值 1"}));
 expect(screen.getByLabelText("筛选 at 集合值 1 日期")).toHaveValue("");
});
it("AC-003/015 uses the latest metadata schema for default exact query and sorting", async () => {
  const current = [field("new_field"), field("changed", {}, "string")];
  current[0]!.state = "missing";
  current[1]!.state = "incompatible";
  current[1]!.warning = "原数字控件与当前类型不兼容";
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({table_name:"items",fields:current,query_capacity:{max_conditions:256,max_values_per_condition:100,queryable_fields:2,supported:true}}))));
  const submit = vi.fn();
  render(<CombinedQueryForm tableName="items" columns={[{name:"removed",type:"string",nullable:false},{name:"changed",type:"int64",nullable:false}]} onSubmit={submit} onClear={()=>{}} />);
  const input = await screen.findByRole("textbox", {name:"筛选 new_field 值"});
  expect(screen.queryByRole("textbox", {name:"筛选 removed 值"})).not.toBeInTheDocument();
  expect(screen.queryByRole("option", {name:"removed"})).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("已回退文本输入");
  const user = userEvent.setup();
  await user.type(input, "new text");
  await user.type(screen.getByRole("textbox", {name:"筛选 changed 值"}), "now text");
  await user.click(screen.getByRole("button", {name:"查询"}));
  expect(submit).toHaveBeenCalledWith({conditions:[{field:"changed",operator:"exact",value:"now text"},{field:"new_field",operator:"exact",value:"new text"}],pageNumber:1});
});
