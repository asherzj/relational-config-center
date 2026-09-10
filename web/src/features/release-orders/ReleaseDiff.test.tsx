import {render,screen,within} from "@testing-library/react";
import {expect,it} from "vitest";
import userEvent from "@testing-library/user-event";
import type {ReleaseField,ReleaseOrder} from "../../api/release-orders";
import {ReleaseDiff,ReleaseValue} from "./ReleaseDiff";
it.each([['SQL NULL','sql_null'],['未提交','omitted'],['不存在','absent'],['发布时生成','automatic']] as const)('值 %s 与同名特殊状态有不同语义', (text,state)=>{
 const view=render(<ReleaseValue state="value" value={text}/>);
 const actual=view.container.textContent;
 expect(screen.getByText(text)).toBeVisible();
 view.rerender(<ReleaseValue state={state} value={null}/>);
 expect(view.container.textContent).not.toBe(actual);
});

it("只显示有变化的字段默认开启，隐藏 MODIFY 未提交及确定同值字段，完整视图保留状态区别",async()=>{
 const field=(name:string,before_state:string,before:string|null,proposed_state:string,proposed:string|null)=>({name,type:name==="payload"?"json":"string",nullable:true,editable:true,before_state,before,proposed_state,proposed});
 const item={detail_id:"1",table_name:"items",operation:"MODIFY",id:"17",expected_record_version:"3",content:{},before:{},fields:[field("same","value","equal","value","equal"),field("same_null","sql_null",null,"sql_null",null),field("empty","value","before","value",""),field("nullable","value","before","sql_null",null),field("payload","sql_null",null,"value","null"),field("omitted","value","before","omitted",null),field("stamp","value","old","automatic",null),field("derived","value","old","generated",null)]};
 render(<ReleaseDiff order={{items:[item] as ReleaseOrder["items"]}}/>);
 const user=userEvent.setup();
 expect(screen.getByRole("checkbox",{name:"只显示有变化的字段"})).toBeChecked();
 expect(screen.queryByText("same")).not.toBeInTheDocument();expect(screen.queryByText("same_null")).not.toBeInTheDocument();
 expect(screen.getByText('空字符串（""）')).toBeVisible();expect(screen.getAllByText("SQL NULL").length).toBeGreaterThan(0);
 expect(screen.getByText("JSON：null")).toBeVisible();expect(screen.queryByText("omitted")).not.toBeInTheDocument();expect(screen.queryByText("未提交")).not.toBeInTheDocument();
 expect(screen.getByText("发布时生成")).toBeVisible();expect(screen.getByText("数据库生成（发布后确认）")).toBeVisible();
 await user.click(screen.getByRole("checkbox",{name:"只显示有变化的字段"}));expect(screen.getByText("same")).toBeVisible();expect(screen.getByText("same_null")).toBeVisible();expect(screen.getByText("omitted")).toBeVisible();expect(screen.getByText("未提交")).toBeVisible();
 for(const name of ["same","same_null","omitted"]){
  const row=screen.getByText(name).closest("tr")!;
  expect(row.querySelector(".release-diff-before,.release-diff-after")).toBeNull();
 }
 await user.click(screen.getByRole("checkbox",{name:"只显示有变化的字段"}));expect(screen.queryByText("omitted")).not.toBeInTheDocument();expect(screen.getByText("发布时生成")).toBeVisible();expect(screen.getByText("数据库生成（发布后确认）")).toBeVisible();
});

it.each<{
 label:string;
 operation:ReleaseOrder["items"][number]["operation"];
 before_state:ReleaseField["before_state"];
 before:string|null;
 proposed_state:ReleaseField["proposed_state"];
 proposed:string|null;
 highlighted:boolean;
}>([
 {label:"相同字符串",operation:"MODIFY",before_state:"value",before:"alipay",proposed_state:"value",proposed:"alipay",highlighted:false},
 {label:"相同空字符串",operation:"MODIFY",before_state:"value",before:"",proposed_state:"value",proposed:"",highlighted:false},
 {label:"相同 SQL NULL",operation:"MODIFY",before_state:"sql_null",before:null,proposed_state:"sql_null",proposed:null,highlighted:false},
 {label:"未提交现有值",operation:"MODIFY",before_state:"value",before:"existing",proposed_state:"omitted",proposed:null,highlighted:false},
 {label:"未提交 SQL NULL",operation:"MODIFY",before_state:"sql_null",before:null,proposed_state:"omitted",proposed:null,highlighted:false},
 {label:"不同字符串",operation:"MODIFY",before_state:"value",before:"支付宝渠道",proposed_state:"value",proposed:"1支付宝渠道",highlighted:true},
 {label:"保留空白区别",operation:"MODIFY",before_state:"value",before:"alipay",proposed_state:"value",proposed:"alipay ",highlighted:true},
 {label:"保留大整数区别",operation:"MODIFY",before_state:"value",before:"9007199254740992",proposed_state:"value",proposed:"9007199254740993",highlighted:true},
 {label:"空字符串改为 SQL NULL",operation:"MODIFY",before_state:"value",before:"",proposed_state:"sql_null",proposed:null,highlighted:true},
 {label:"SQL NULL 改为空字符串",operation:"MODIFY",before_state:"sql_null",before:null,proposed_state:"value",proposed:"",highlighted:true},
 {label:"发布时自动填写",operation:"MODIFY",before_state:"value",before:"发布时生成",proposed_state:"automatic",proposed:null,highlighted:true},
 {label:"数据库重新生成",operation:"MODIFY",before_state:"sql_null",before:null,proposed_state:"generated",proposed:null,highlighted:true},
 {label:"新增空字符串",operation:"ADD",before_state:"absent",before:null,proposed_state:"value",proposed:"",highlighted:true},
 {label:"新增 SQL NULL",operation:"ADD",before_state:"absent",before:null,proposed_state:"sql_null",proposed:null,highlighted:true},
 {label:"新增默认值",operation:"ADD",before_state:"absent",before:null,proposed_state:"omitted",proposed:null,highlighted:true},
 {label:"删除现有值",operation:"DELETE",before_state:"value",before:"existing",proposed_state:"absent",proposed:null,highlighted:true},
 {label:"删除 SQL NULL",operation:"DELETE",before_state:"sql_null",before:null,proposed_state:"absent",proposed:null,highlighted:true},
])("完整对比的着色正确区分 $label",async({operation,before_state,before,proposed_state,proposed,highlighted})=>{
 const items:ReleaseOrder["items"]=[{detail_id:"1",table_name:"channels",operation,id:"2",expected_record_version:"0",content:{},before:{},fields:[{name:"channel_code",type:"string",nullable:true,editable:true,before_state,before,proposed_state,proposed}]}];
 const view=render(<ReleaseDiff order={{items}}/>);
 await userEvent.setup().click(screen.getByRole("checkbox",{name:"只显示有变化的字段"}));
 const row=screen.getByText("channel_code").closest("tr")!;
 const [beforeCell,proposedCell]=within(row).getAllByRole("cell");
 if(highlighted){
  expect(beforeCell).toHaveClass("release-diff-before");expect(proposedCell).toHaveClass("release-diff-after");
 }else{
  expect(beforeCell).not.toHaveClass("release-diff-before");expect(proposedCell).not.toHaveClass("release-diff-after");
 }
 // The recovery preview changes column labels while retaining the same field semantics.
 view.rerender(<ReleaseDiff order={{items}} beforeLabel="当前值" proposedLabel="恢复值"/>);
 expect(screen.getByRole("columnheader",{name:"恢复值"})).toBeVisible();
 expect(row.querySelectorAll(".release-diff-before,.release-diff-after")).toHaveLength(highlighted?2:0);
});
it("千项混合差异每页20项，仅展开首项且 ADD/DELETE 保持完整",async()=>{
 const items=Array.from({length:1000},(_,index)=>({detail_id:"1",table_name:"items",operation:index%2?"DELETE":"ADD",id:String(index+1),expected_record_version:"1",content:{},before:{},fields:[{name:`field_${index+1}`,type:"string",nullable:false,editable:true,before_state:"value",before:"same",proposed_state:"value",proposed:"same"}]})) as ReleaseOrder["items"];
 const {container}=render(<ReleaseDiff order={{items}}/>);const user=userEvent.setup();
 expect(container.querySelectorAll("details")).toHaveLength(20);expect(container.querySelectorAll("details[open]")).toHaveLength(1);
 expect(screen.getByText("field_1")).toBeVisible();expect(screen.getByText("field_2")).not.toBeVisible();
 await user.click(within(screen.getByLabelText("明细 2")).getByText(/明细 2 · .*DELETE/));expect(screen.getByText("field_2")).toBeVisible();
 await user.click(screen.getByRole("button",{name:"下一页明细"}));expect(screen.getByText("field_21")).toBeVisible();
 await user.type(screen.getByRole("textbox",{name:"定位明细"}),"1000");expect(screen.getByText("field_1000")).toBeVisible();
 expect(container.querySelectorAll("details")).toHaveLength(20);expect(screen.getByRole("button",{name:"下一页明细"})).toBeDisabled();
 expect(screen.getByText(/共 1,000 项，当前展示 981–1000 项/)).toBeVisible();
});
it("字段对比的实际横向滚动容器可经键盘聚焦且有明确名称",()=>{
 const items=[{detail_id:"1",table_name:"items",operation:"MODIFY",id:"17",expected_record_version:"1",content:{label:"next"},before:{label:"previous"},fields:[{name:"label",type:"string",nullable:false,editable:true,before_state:"value",before:"previous",proposed_state:"value",proposed:"next"}]}] as ReleaseOrder["items"];
 render(<ReleaseDiff order={{items}}/>);
 const scroll=screen.getByRole("region",{name:"明细 1 字段对比，可横向滚动"});expect(scroll).toHaveAttribute("tabindex","0");expect(scroll).toHaveAttribute("data-slot","table-container");expect(scroll).toContainElement(screen.getByRole("table"));
});
