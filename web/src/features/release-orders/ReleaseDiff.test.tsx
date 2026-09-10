import {render,screen,within} from "@testing-library/react";
import {expect,it} from "vitest";
import userEvent from "@testing-library/user-event";
import type {ReleaseOrder} from "../../api/release-orders";
import {ReleaseDiff,ReleaseValue} from "./ReleaseDiff";
it.each([['SQL NULL','sql_null'],['未提交','omitted'],['不存在','absent'],['发布时生成','automatic']] as const)('值 %s 与同名特殊状态有不同语义', (text,state)=>{
 const view=render(<ReleaseValue state="value" value={text}/>);
 const actual=view.container.textContent;
 expect(screen.getByText(text)).toBeVisible();
 view.rerender(<ReleaseValue state={state} value={null}/>);
 expect(view.container.textContent).not.toBe(actual);
});

it("仅看变更默认开启，隐藏 MODIFY 未提交及确定同值字段，完整视图保留状态区别",async()=>{
 const field=(name:string,before_state:string,before:string|null,proposed_state:string,proposed:string|null)=>({name,type:name==="payload"?"json":"string",nullable:true,editable:true,before_state,before,proposed_state,proposed});
 const item={operation:"MODIFY",id:"17",expected_record_version:"3",content:{},before:{},fields:[field("same","value","equal","value","equal"),field("same_null","sql_null",null,"sql_null",null),field("empty","value","before","value",""),field("nullable","value","before","sql_null",null),field("payload","sql_null",null,"value","null"),field("omitted","value","before","omitted",null),field("stamp","value","old","automatic",null),field("derived","value","old","generated",null)]};
 render(<ReleaseDiff order={{items:[item] as ReleaseOrder["items"]}}/>);
 const user=userEvent.setup();
 expect(screen.getByRole("checkbox",{name:"仅看变更"})).toBeChecked();
 expect(screen.queryByText("same")).not.toBeInTheDocument();expect(screen.queryByText("same_null")).not.toBeInTheDocument();
 expect(screen.getByText('空字符串（""）')).toBeVisible();expect(screen.getAllByText("SQL NULL").length).toBeGreaterThan(0);
 expect(screen.getByText("JSON：null")).toBeVisible();expect(screen.queryByText("omitted")).not.toBeInTheDocument();expect(screen.queryByText("未提交")).not.toBeInTheDocument();
 expect(screen.getByText("发布时生成")).toBeVisible();expect(screen.getByText("数据库生成（发布后确认）")).toBeVisible();
 await user.click(screen.getByRole("checkbox",{name:"仅看变更"}));expect(screen.getByText("same")).toBeVisible();expect(screen.getByText("same_null")).toBeVisible();expect(screen.getByText("omitted")).toBeVisible();expect(screen.getByText("未提交")).toBeVisible();
 await user.click(screen.getByRole("checkbox",{name:"仅看变更"}));expect(screen.queryByText("omitted")).not.toBeInTheDocument();expect(screen.getByText("发布时生成")).toBeVisible();expect(screen.getByText("数据库生成（发布后确认）")).toBeVisible();
});
it("千项混合差异每页20项，仅展开首项且 ADD/DELETE 保持完整",async()=>{
 const items=Array.from({length:1000},(_,index)=>({operation:index%2?"DELETE":"ADD",id:String(index+1),expected_record_version:"1",content:{},before:{},fields:[{name:`field_${index+1}`,type:"string",nullable:false,editable:true,before_state:"value",before:"same",proposed_state:"value",proposed:"same"}]})) as ReleaseOrder["items"];
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
 const items=[{operation:"MODIFY",id:"17",expected_record_version:"1",content:{label:"next"},before:{label:"previous"},fields:[{name:"label",type:"string",nullable:false,editable:true,before_state:"value",before:"previous",proposed_state:"value",proposed:"next"}]}] as ReleaseOrder["items"];
 render(<ReleaseDiff order={{items}}/>);
 const scroll=screen.getByRole("region",{name:"明细 1 字段对比，可横向滚动"});expect(scroll).toHaveAttribute("tabindex","0");expect(scroll).toHaveAttribute("data-slot","table-container");expect(scroll).toContainElement(screen.getByRole("table"));
});
