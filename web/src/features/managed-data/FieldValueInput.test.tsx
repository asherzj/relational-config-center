import { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { FieldValueInput } from "./FieldValueInput";
import type { FieldPolicyField } from "../../api/field-policies";
import type { ManagedDataColumn } from "./model";

function Example({ type, ui, initial }: { type: ManagedDataColumn["type"]; ui: FieldPolicyField["effective"]["ui_type"]; initial: string }) {
 const [value,setValue]=useState(initial);
 return <><FieldValueInput column={{name:"value",type,nullable:false}} policy={{ui_type:ui,ui_options:{options:[]}}} label="value" value={value} onChange={setValue}/><output>{JSON.stringify(value)}</output></>;
}
it("AC-008 edits timestamp microseconds without local timezone or precision conversion", async()=>{
 render(<Example type="timestamp" ui="datetime" initial="2026-09-09T23:59:59.123456Z"/>);
 expect(screen.getByLabelText("value 日期")).toHaveValue("2026-09-09");
 expect(screen.getByRole("textbox",{name:"value 时间"})).toHaveValue("23:59:59.123456");
 await userEvent.clear(screen.getByRole("textbox",{name:"value 时间"}));
 await userEvent.type(screen.getByRole("textbox",{name:"value 时间"}),"00:00:00.000001");
 expect(screen.getByRole("status")).toHaveTextContent('"2026-09-09T00:00:00.000001Z"');
});
it("AC-008 changes the date of a MySQL datetime without changing its microseconds",()=>{
 render(<Example type="datetime" ui="datetime" initial="2026-09-09 23:59:59.123456"/>);
 fireEvent.change(screen.getByLabelText("value 日期"),{target:{value:"2026-09-10"}});
 expect(screen.getByRole("status")).toHaveTextContent('"2026-09-10 23:59:59.123456"');
});
it("AC-008 numeric edits retain large integers, high precision decimals and exponent strings",()=>{
 render(<Example type="decimal" ui="number" initial="9007199254740993.123456789"/>);
 const input=screen.getByRole("textbox",{name:"value"});
 for(const value of ["18446744073709551615","9007199254740993.123456789","1.234567890123456789e20"]){
  fireEvent.change(input,{target:{value}});expect(screen.getByRole("status")).toHaveTextContent(JSON.stringify(value));
 }
});
it("AC-008 renders explicit date, single-line text and multiline controls",()=>{
 const view=render(<Example type="date" ui="date" initial="2026-09-09"/>);
 expect(screen.getByLabelText("value")).toHaveAttribute("type","date");view.unmount();
 const text=render(<Example type="string" ui="text" initial="single"/>);
 expect(screen.getByRole("textbox").tagName).toBe("INPUT");text.unmount();
 render(<Example type="string" ui="textarea" initial="many\nlines"/>);
 expect(screen.getByRole("textbox").tagName).toBe("TEXTAREA");
});
it("preserves pasted CRLF from the initially single-line text control", async()=>{
 const user=userEvent.setup();render(<Example type="string" ui="text" initial="prefixsuffix"/>);
 const input=screen.getByRole("textbox",{name:"value"}) as HTMLInputElement;
 await user.click(input);input.setSelectionRange(6,6);await user.paste("one\r\ntwo");
 expect(screen.getByRole("status")).toHaveTextContent(JSON.stringify("prefixone\r\ntwosuffix"));
 expect(screen.getByRole("textbox",{name:"value"})).toHaveAttribute("readonly");
});
