import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { defaultFieldPolicies } from "../../test/field-policy-fixture";
import { ChangeSetDialog } from "./ChangeSetDialog";

it("删除确认按当前顺序显示全部实际字段，并保留重复标签对应的真实字段和值", async () => {
  const configuration = defaultFieldPolicies("items", [
    { name: "beta", type: "string", nullable: false },
    { name: "hidden", type: "string", nullable: false },
    { name: "alpha", type: "string", nullable: false },
  ]);
  for (const field of configuration.fields) {
    field.state = "active";
    field.effective = {
      ...field.effective,
      display_name: field.field_name === "hidden" ? "隐藏字段" : "状态",
      display_order: field.field_name === "hidden" ? 0 : 5,
      is_visible: field.field_name !== "hidden",
      ui_type: "select",
      ui_options: { options: [{ label: "启用", value: field.field_name.slice(0, 1) }] },
      enabled: true,
    };
  }
  render(<ChangeSetDialog
    changeSet={{ operation: "DELETE", rows: [
      { field: "beta", original: { state: "value", value: "b" }, next: { state: "missing" }, changed: true, autoFill: false },
      { field: "hidden", original: { state: "value", value: "h" }, next: { state: "missing" }, changed: true, autoFill: false },
      { field: "alpha", original: { state: "value", value: "a" }, next: { state: "missing" }, changed: true, autoFill: false },
    ] }}
    fieldDisplay={{ configuration }}
    pending={false}
    onEdit={vi.fn()}
    onCancel={vi.fn()}
  />);

  const rows = await screen.findAllByRole("row");
  expect(rows.slice(1).map(row => row.textContent)).toEqual([
    expect.stringMatching(/^隐藏字段hidden/),
    expect.stringMatching(/^状态alpha/),
    expect.stringMatching(/^状态beta/),
  ]);
  expect(screen.getAllByText("启用")).toHaveLength(3);
  expect(screen.getByLabelText("真实值：a")).toBeVisible();
  expect(screen.getByLabelText("真实值：b")).toBeVisible();
  expect(screen.getByLabelText("真实值：h")).toBeVisible();
});
