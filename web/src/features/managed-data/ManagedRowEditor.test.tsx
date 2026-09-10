import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { TestRouter } from "../../test/TestRouter";
import { LeaveProtectionProvider } from "../../components/ui/LeaveProtection";
import { ManagedRowEditor } from "./ManagedRowEditor";

beforeEach(() => { HTMLElement.prototype.scrollIntoView = vi.fn(); });

it("preserves multiline string and JSON when reopening and editing a row", async () => {
  const user = userEvent.setup();
  const onReview = vi.fn();
  const original = { note: "  中文🙂\nsecond\tline  ", payload: '{\n "large": 9007199254740993\n}' };
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="MODIFY"
    columns={[{ name: "note", type: "string", nullable: true }, { name: "payload", type: "json", nullable: true }]}
    original={original} autoFillFields={new Set()} onClose={() => {}} onReview={onReview} />
  </LeaveProtectionProvider></TestRouter>);
  expect(screen.getByRole("textbox", { name: "note 值" })).toHaveValue(original.note);
  expect(screen.getByRole("textbox", { name: "payload 值" })).toHaveValue(original.payload);
  expect(screen.queryByRole("checkbox", { name: /^包含 / })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "note 值" })).toBeEnabled();
  await user.click(screen.getByRole("textbox", { name: "note 值" }));
  await user.keyboard("{End}{Enter}追加");
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenCalledWith({ note: `${original.note}\n追加`, payload: original.payload });
});

it("submits original values with edits, retaining NULL and empty values and excluding managed columns", async () => {
  const user = userEvent.setup();
  const onReview = vi.fn();
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="MODIFY"
    columns={[
      { name: "id", type: "uint64", nullable: false },
      { name: "name", type: "string", nullable: false },
      { name: "empty", type: "string", nullable: true },
      { name: "note", type: "string", nullable: true },
      { name: "total", type: "int64", nullable: false, generated: true },
      { name: "modifier", type: "string", nullable: false },
    ]}
    original={{ id: "7", name: "原值", empty: "", note: null, total: "12", modifier: "old-actor" }}
    autoFillFields={new Set(["modifier"])} onClose={() => {}} onReview={onReview} />
  </LeaveProtectionProvider></TestRouter>);
  expect(screen.queryByRole("textbox", { name: /^(id|total|modifier) 值$/ })).not.toBeInTheDocument();
  expect(screen.queryByRole("checkbox", { name: "name 使用 NULL" })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "note 值" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenLastCalledWith({ name: "原值", empty: "", note: null });
  await user.clear(screen.getByRole("textbox", { name: "name 值" }));
  await user.type(screen.getByRole("textbox", { name: "name 值" }), "新值");
  await user.click(screen.getByRole("checkbox", { name: "note 使用 NULL" }));
  expect(screen.getByRole("textbox", { name: "note 值" })).toBeEnabled();
  await user.click(screen.getByRole("checkbox", { name: "empty 使用 NULL" }));
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenLastCalledWith({ name: "新值", empty: null, note: "" });
  await user.type(screen.getByRole("textbox", { name: "note 值" }), "保留输入");
  await user.click(screen.getByRole("checkbox", { name: "note 使用 NULL" }));
  expect(screen.getByRole("textbox", { name: "note 值" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenLastCalledWith({ name: "新值", empty: null, note: null });
  await user.click(screen.getByRole("checkbox", { name: "note 使用 NULL" }));
  expect(screen.getByRole("textbox", { name: "note 值" })).toHaveValue("保留输入");
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenLastCalledWith({ name: "新值", empty: null, note: "保留输入" });
});

it("offers an omitted-by-default id for ADD while keeping id immutable on MODIFY", async () => {
  const user = userEvent.setup();
  const onReview = vi.fn();
  const columns = [{ name: "id", type: "uint64", nullable: false }, { name: "label", type: "string", nullable: true }] as const;
  const view = render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="ADD"
    columns={columns} autoFillFields={new Set()} onClose={() => {}} onReview={onReview} />
  </LeaveProtectionProvider></TestRouter>);
  expect(screen.getByRole("checkbox", { name: "包含 id" })).not.toBeChecked();
  expect(screen.queryByRole("checkbox", { name: "id 使用 NULL" })).not.toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: "label 使用 NULL" })).toBeDisabled();
  await user.click(screen.getByRole("checkbox", { name: "包含 id" }));
  await user.type(screen.getByRole("textbox", { name: "id 值" }), "9007199254740993");
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenCalledWith({ id: "9007199254740993" });
  view.unmount();
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="MODIFY"
    columns={columns} original={{ id: "9007199254740993", label: "old" }} autoFillFields={new Set()} onClose={() => {}} onReview={onReview} />
  </LeaveProtectionProvider></TestRouter>);
  expect(screen.queryByRole("checkbox", { name: "包含 id" })).not.toBeInTheDocument();
});

it("AC-008/009 uses the configured numeric control and preserves an exact ADD prefill", async () => {
  const onReview = vi.fn();
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="ADD"
    columns={[{ name: "amount", type: "decimal", nullable: false }]} autoFillFields={new Set()}
    fieldPolicies={[fieldPolicy("amount", { ui_type: "number", default_value: "9007199254740993.123456789" })]}
    onClose={() => {}} onReview={onReview} /></LeaveProtectionProvider></TestRouter>);
  expect(screen.getByRole("textbox", { name: "amount 值" })).toHaveAttribute("inputmode", "decimal");
  expect(screen.getByRole("textbox", { name: "amount 值" })).toHaveValue("9007199254740993.123456789");
  await userEvent.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenCalledWith({ amount: "9007199254740993.123456789" });
});

import type { FieldPolicyField, FieldPolicy } from "../../api/field-policies";
function fieldPolicy(name: string, patch: Partial<FieldPolicy> = {}): FieldPolicyField {
  const effective = { field_name: name, display_name: name, description: "", display_order: 0, is_visible: true, is_queryable: true,
    query_operators: ["exact"], ui_type: "text", ui_options: { options: [] }, editable_on_add: true, editable_on_modify: true, is_required: false, enabled: true, ...patch } as FieldPolicyField["effective"];
  return { field_name: name, column_type: "decimal", nullable: false, generated: false, auto_increment: false, has_default: false, state: "active", warning: "", policy: effective, effective, audit: null };
}

it("AC-009/010 keeps omitted, explicit NULL and empty distinct; rejects required empty but accepts zero and false", async () => {
  const user = userEvent.setup(), onReview = vi.fn();
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="ADD"
    columns={["required", "nullable", "empty", "optional", "hidden", "flag"].map(name => ({ name, type: name === "flag" ? "boolean" : "string", nullable: true }))}
    fieldPolicies={[fieldPolicy("required", { is_required: true }), fieldPolicy("nullable", { default_value: null }), fieldPolicy("empty", { default_value: "" }), fieldPolicy("hidden", { editable_on_add: false, default_value: "never submit" }), fieldPolicy("flag", { ui_type: "boolean", is_required: true })]}
    autoFillFields={new Set()} onClose={() => {}} onReview={onReview} /></LeaveProtectionProvider></TestRouter>);
  expect(screen.queryByRole("textbox", { name: "hidden 值" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).not.toHaveBeenCalled();
  expect(screen.getByRole("textbox", { name: "required 值" })).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByRole("textbox", { name: "required 值" })).toHaveFocus();
  await user.type(screen.getByRole("textbox", { name: "required 值" }), "0");
  expect(screen.getByRole("textbox", { name: "required 值" })).toHaveFocus();
  await user.selectOptions(screen.getByRole("combobox", { name: "flag 值" }), "0");
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenLastCalledWith({ required: "0", nullable: null, empty: "", flag: "0" });
});

it("AC-007/011 preserves old choices and read-only originals, and replaces only actively selected values", async () => {
  const user = userEvent.setup(), onReview = vi.fn();
  const policies = [fieldPolicy("channel", { ui_type: "select", default_value: "new", ui_options: { options: [{ label: "新渠道", value: "new" }] } }),
    fieldPolicy("mode", { ui_type: "radio", ui_options: { options: [{ label: "新模式", value: "new" }] } }),
    fieldPolicy("locked", { editable_on_modify: false, default_value: "prefill", is_required: true })];
  render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="MODIFY"
    columns={["channel", "mode", "locked"].map(name => ({ name, type: "string", nullable: false }))}
    original={{ channel: "legacy", mode: "old-radio", locked: "original" }} fieldPolicies={policies}
    autoFillFields={new Set()} onClose={() => {}} onReview={onReview} /></LeaveProtectionProvider></TestRouter>);
  expect(screen.getByRole("textbox", { name: "channel 值 自定义值" })).toHaveValue("legacy");
  expect(screen.getByText(/当前值：old-radio/)).toBeVisible();
  expect(screen.getByRole("textbox", { name: "locked 值" })).toBeDisabled();
  expect(screen.getByRole("textbox", { name: "locked 值" })).toHaveValue("original");
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenLastCalledWith({ channel: "legacy", mode: "old-radio" });
  await user.selectOptions(screen.getByRole("combobox", { name: "channel 值" }), "0");
  await user.click(screen.getByRole("radio", { name: "新模式（new）" }));
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenLastCalledWith({ channel: "new", mode: "new" });
});

it("validates configured number range and step without rounding decimal strings", async () => {
 const user=userEvent.setup(),onReview=vi.fn();
 render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="MODIFY"
  columns={[{name:"amount",type:"decimal",nullable:false}]} original={{amount:"9007199254740993.000002"}}
  fieldPolicies={[fieldPolicy("amount",{ui_type:"number",ui_options:{options:[],min:"9007199254740993.000001",max:"9007199254740993.000009",step:"0.000002"}})]}
  autoFillFields={new Set()} onClose={()=>{}} onReview={onReview}/></LeaveProtectionProvider></TestRouter>);
 await user.click(screen.getByRole("button",{name:"查看 Change Set"}));expect(onReview).not.toHaveBeenCalled();
 expect(screen.getByRole("alert")).toHaveTextContent("步长");
 await user.clear(screen.getByRole("textbox",{name:"amount 值"}));await user.type(screen.getByRole("textbox",{name:"amount 值"}),"9007199254740993.000003");
 await user.click(screen.getByRole("button",{name:"查看 Change Set"}));expect(onReview).toHaveBeenCalledWith({amount:"9007199254740993.000003"});
});

it("AC-003/015 consumes disabled and incompatible text fallbacks while preserving schema and auto-fill exclusions", async()=>{
 const disabled=fieldPolicy("disabled",{ui_type:"text"});disabled.state="disabled";disabled.policy={...disabled.effective,ui_type:"radio",enabled:false};
 const drift=fieldPolicy("drift",{ui_type:"text"});drift.state="incompatible";drift.warning="数字控件与当前字符串字段不兼容";drift.policy={...drift.effective,ui_type:"number"};
 const onReview=vi.fn();
 render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="MODIFY"
  columns={[{name:"disabled",type:"string",nullable:false},{name:"drift",type:"string",nullable:false},{name:"id",type:"uint64",nullable:false},{name:"computed",type:"string",nullable:false,generated:true},{name:"actor",type:"string",nullable:false}]}
  original={{disabled:"old",drift:"original",id:"1",computed:"generated",actor:"server"}} fieldPolicies={[disabled,drift]}
  autoFillFields={new Set(["actor"])} onClose={()=>{}} onReview={onReview}/></LeaveProtectionProvider></TestRouter>);
 expect(screen.getByRole("textbox",{name:"disabled 值"})).toHaveAttribute("type","text");
 expect(screen.getByRole("textbox",{name:"drift 值"})).toHaveAttribute("type","text");
 expect(screen.getByRole("status")).toHaveTextContent("已回退文本录入");
 expect(screen.queryByRole("textbox",{name:/^(id|computed|actor) 值$/})).not.toBeInTheDocument();
 await userEvent.click(screen.getByRole("button",{name:"查看 Change Set"}));expect(onReview).toHaveBeenCalledWith({disabled:"old",drift:"original"});
});
