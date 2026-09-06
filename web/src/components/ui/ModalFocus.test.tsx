import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";
import { Drawer } from "./Drawer";

describe("modal focus management", () => {
  beforeEach(() => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("does not steal focus from a drawer field when its parent rerenders", () => {
    function Example() {
      const [value, setValue] = useState("");
      return (
        <Drawer open title="编辑记录" eyebrow="记录" onClose={() => undefined}>
          <label>名称<input aria-label="名称" value={value} onChange={(event) => setValue(event.target.value)} /></label>
        </Drawer>
      );
    }

    render(<Example />);
    const input = screen.getByRole("textbox", { name: "名称" });
    input.focus();
    fireEvent.change(input, { target: { value: "alpha" } });

    expect(input).toHaveFocus();
  });

  it("keeps keyboard interaction in the top confirmation dialog", async () => {
    const user = userEvent.setup();
    const closeDrawer = vi.fn();
    const cancelConfirm = vi.fn();
    render(
      <>
        <Drawer open title="规则详情" eyebrow="规则" onClose={closeDrawer} footer={<button>抽屉操作</button>}>
          <button>下层操作</button>
        </Drawer>
        <ConfirmDialog
          open
          title="激活规则？"
          description="激活后不可修改。"
          confirmLabel="确认激活"
          onCancel={cancelConfirm}
          onConfirm={() => undefined}
        />
      </>,
    );

    const cancel = screen.getByRole("button", { name: "取消" });
    const confirm = screen.getByRole("button", { name: "确认激活" });
    expect(cancel).toHaveFocus();

    confirm.focus();
    await user.tab();
    expect(cancel).toHaveFocus();

    await user.tab({ shift: true });
    expect(confirm).toHaveFocus();

    await user.keyboard("{Escape}");
    expect(cancelConfirm).toHaveBeenCalledOnce();
    expect(closeDrawer).not.toHaveBeenCalled();
  });
});
