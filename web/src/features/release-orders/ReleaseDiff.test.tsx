import {render,screen} from "@testing-library/react";
import {expect,it} from "vitest";
import {ReleaseValue} from "./ReleaseDiff";
it.each([['SQL NULL','sql_null'],['未提交','omitted'],['不存在','absent'],['发布时生成','automatic']] as const)('值 %s 与同名特殊状态有不同语义', (text,state)=>{
 const view=render(<ReleaseValue state="value" value={text}/>);
 const actual=view.container.textContent;
 expect(screen.getByText(text)).toBeVisible();
 view.rerender(<ReleaseValue state={state} value={null}/>);
 expect(view.container.textContent).not.toBe(actual);
});
