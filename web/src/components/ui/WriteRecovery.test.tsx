import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { WriteRecovery } from "./WriteRecovery";

it("requires a fresh read for each unknown attempt and keeps NULL/empty/string values distinct", async () => {
  const user = userEvent.setup();
  const first = new ApiError("network_error", "lost", 0);
  const resume = vi.fn();
  const check = vi.fn().mockResolvedValue({ columns: [{ name: "id" }, { name: "value" }, { name: "empty" }], rows: [{ id: "9007199254740993", value: null, empty: "" }] });
  const view = render(<WriteRecovery error={first} onCheck={check} onResume={resume} />);
  expect(screen.queryByRole("button", { name: "我已核对，返回修改" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "只读核对当前状态" }));
  expect(await screen.findByText("9007199254740993")).toBeVisible();
  expect(screen.getByText("NULL")).toBeVisible(); expect(screen.getByText("空字符串")).toBeVisible();
  expect(resume).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "我已核对，返回修改" }));
  expect(resume).toHaveBeenCalledTimes(1);
  view.rerender(<WriteRecovery error={null} onCheck={check} onResume={resume} />);
  const second = new ApiError("mutation_unavailable", "unknown", 503);
  view.rerender(<WriteRecovery error={second} onCheck={check} onResume={resume} />);
  expect(screen.queryByText("9007199254740993")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "我已核对，返回修改" })).not.toBeInTheDocument();
  check.mockRejectedValue(new ApiError("query_unavailable", "down", 503));
  await user.click(screen.getByRole("button", { name: "只读核对当前状态" }));
  await screen.findByText("Managed Table 查询暂时不可用，请稍后重试。");
  expect(screen.queryByRole("button", { name: "我已核对，返回修改" })).not.toBeInTheDocument();
});

it("does not attach a delayed observation of target A to target B", async () => {
  let resolve!: (value: unknown) => void;
  const check = () => new Promise(value => { resolve = value; });
  const first = new ApiError("network_error", "A", 0);
  const view = render(<WriteRecovery error={first} onCheck={check} onResume={vi.fn()} />);
  await userEvent.click(screen.getByRole("button", { name: "只读核对当前状态" }));
  view.rerender(<WriteRecovery error={new ApiError("network_error", "B", 0)} onCheck={check} onResume={vi.fn()} />);
  await act(async () => resolve({ name: "Old A" }));
  expect(screen.queryByText("Old A")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "我已核对，返回修改" })).not.toBeInTheDocument();
});


it("replaces the previous observation when a second read is missing or unavailable", async () => {
  const error = new ApiError("network_error", "lost", 0);
  const check = vi.fn().mockResolvedValueOnce({ name: "Previously observed" })
    .mockRejectedValueOnce(new ApiError("query_policy_not_found", "missing", 404))
    .mockRejectedValueOnce(new ApiError("policy_catalog_unavailable", "unavailable", 503));
  render(<WriteRecovery error={error} onCheck={check} onResume={vi.fn()} />);
  await userEvent.click(screen.getByRole("button", { name: "只读核对当前状态" }));
  expect(await screen.findByText("Previously observed")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "只读核对当前状态" }));
  await screen.findByText("查询规则不存在或已被移除。");
  expect(screen.queryByText("Previously observed")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "我已核对，返回修改" })).toBeEnabled();
  await userEvent.click(screen.getByRole("button", { name: "只读核对当前状态" }));
  await screen.findByText("规则目录暂时不可用，请稍后重试。");
  expect(screen.queryByText("Previously observed")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "我已核对，返回修改" })).not.toBeInTheDocument();
});
