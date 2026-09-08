import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { ManagedTextInput } from "./ManagedTextInput";

function Editor({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial);
  return <><ManagedTextInput label="text" value={value} onChange={setValue} /><output data-testid="raw-value">{JSON.stringify(value)}</output></>;
}

it("keeps raw CRLF and CR read-only until explicit LF conversion", async () => {
  const user = userEvent.setup();
  const raw = "one\r\ntwo\rthree";
  render(<Editor initial={raw} />);
  const input = screen.getByRole("textbox", { name: "text" });
  expect(input).toHaveAttribute("readonly");
  await user.type(input, "must not edit");
  expect(screen.getByTestId("raw-value").textContent).toBe(JSON.stringify(raw));
  await user.click(screen.getByRole("button", { name: "text：转换为 LF 再编辑" }));
  expect(input).not.toHaveAttribute("readonly");
  expect(input).toHaveValue("one\ntwo\nthree");
  expect(screen.getByTestId("raw-value").textContent).toBe(JSON.stringify("one\ntwo\nthree"));
});

it("captures CR clipboard text before textarea normalization and respects the selection", async () => {
  const user = userEvent.setup();
  render(<Editor initial="prefix old suffix" />);
  const input = screen.getByRole("textbox", { name: "text" }) as HTMLTextAreaElement;
  await user.click(input);
  input.setSelectionRange(7, 10);
  await user.paste("first\r\nsecond\rthird");
  expect(input).toHaveAttribute("readonly");
  expect(screen.getByTestId("raw-value").textContent).toBe(JSON.stringify("prefix first\r\nsecond\rthird suffix"));
});
