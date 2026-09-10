import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { Table } from "./table";

it("lets a keyboard user scroll the focused table without intercepting keys inside a field", () => {
  render(<Table containerProps={{ role: "region", "aria-label": "字段对比", tabIndex: 0 }}><tbody><tr><td><input aria-label="表内输入" /></td></tr></tbody></Table>);
  const region = screen.getByRole("region", { name: "字段对比" });
  Object.defineProperties(region, { scrollWidth: { value: 800 }, clientWidth: { value: 300 } });
  region.focus();
  fireEvent.keyDown(region, { key: "ArrowRight" });
  expect(region.scrollLeft).toBeGreaterThan(0);
  const position = region.scrollLeft;
  const field = screen.getByRole("textbox", { name: "表内输入" });
  field.focus();
  expect(fireEvent.keyDown(field, { key: "ArrowRight" })).toBe(true);
  expect(region.scrollLeft).toBe(position);
  region.focus();
  expect(fireEvent.keyDown(region, { key: "ArrowLeft", altKey: true })).toBe(true);
  expect(region.scrollLeft).toBe(position);
  fireEvent.keyDown(region, { key: "ArrowLeft" });
  expect(region.scrollLeft).toBe(0);
});
