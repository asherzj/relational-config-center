import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { TestRouter } from "../../test/TestRouter";
import { LeaveProtectionProvider } from "../../components/ui/LeaveProtection";
import { ManagedRowEditor } from "./ManagedRowEditor";

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
  await user.click(screen.getByRole("checkbox", { name: "包含 note" }));
  await user.click(screen.getByRole("textbox", { name: "note 值" }));
  await user.keyboard("{End}{Enter}追加");
  await user.click(screen.getByRole("checkbox", { name: "包含 payload" }));
  await user.click(screen.getByRole("button", { name: "查看 Change Set" }));
  expect(onReview).toHaveBeenCalledWith({ note: `${original.note}\n追加`, payload: original.payload });
});

it("offers an omitted-by-default id for ADD while keeping id immutable on MODIFY", async () => {
  const user = userEvent.setup();
  const onReview = vi.fn();
  const columns = [{ name: "id", type: "uint64", nullable: false }, { name: "label", type: "string", nullable: true }] as const;
  const view = render(<TestRouter initialEntries={["/"]}><LeaveProtectionProvider><ManagedRowEditor open tableName="items" operation="ADD"
    columns={columns} autoFillFields={new Set()} onClose={() => {}} onReview={onReview} />
  </LeaveProtectionProvider></TestRouter>);
  expect(screen.getByRole("checkbox", { name: "包含 id" })).not.toBeChecked();
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
