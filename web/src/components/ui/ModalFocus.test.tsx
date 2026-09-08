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

  it("advances every modal Tab step so browser focus preferences cannot skip actions", () => {
    render(<Drawer open title="发布草稿" eyebrow="草稿" onClose={() => undefined}>
      <input aria-label="发布单标题" />
      <button>选择已有草稿</button>
    </Drawer>);
    const title = screen.getByRole("textbox", { name: "发布单标题" });
    const choose = screen.getByRole("button", { name: "选择已有草稿" });
    title.focus();

    fireEvent.keyDown(title, { key: "Tab" });
    expect(choose).toHaveFocus();

    fireEvent.keyDown(choose, { key: "Tab", shiftKey: true });
    expect(title).toHaveFocus();
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
  it("skips controls disabled by a pending fieldset when trapping Tab", async () => {
    const user = userEvent.setup();
    render(<Drawer open title="正在保存" eyebrow="规则" onClose={() => undefined}>
      <fieldset disabled><input aria-label="正在提交的输入" /><button>禁用操作</button></fieldset>
    </Drawer>);
    const close = screen.getByRole("button", { name: "关闭" });
    await user.tab({ shift: true });
    expect(close).toHaveFocus();
    await user.tab();
    expect(close).toHaveFocus();
  });

  it("makes the page behind a modal inert and rejects programmatic focus outside it", () => {
    render(<><button data-testid="background-action">Background action</button><Drawer open title="编辑记录" eyebrow="记录" onClose={() => undefined}><button>Drawer action</button></Drawer></>);
    const background = screen.getByTestId("background-action");
    const drawer = screen.getByRole("dialog", { name: "编辑记录" });

    expect(background.closest("[inert]")).not.toBeNull();
    background.focus();
    expect(drawer).toContainElement(document.activeElement as HTMLElement);
  });

  it("skips hidden controls at both ends of the keyboard loop", async () => {
    const user = userEvent.setup();
    render(
      <Drawer open title="编辑记录" eyebrow="记录" onClose={() => undefined} footer={<><button>Visible footer</button><button style={{ display: "none" }}>Hidden footer</button></>}>
        <button style={{ visibility: "hidden" }}>Hidden body</button>
        <button>Visible body</button>
      </Drawer>,
    );
    const visibleBody = screen.getByRole("button", { name: "Visible body" });
    const visibleFooter = screen.getByRole("button", { name: "Visible footer" });

    await user.tab({ shift: true });
    expect(visibleFooter).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "关闭" })).toHaveFocus();
    visibleBody.focus();
    await user.tab();
    expect(visibleFooter).toHaveFocus();
  });

  it("locks scrolling for a confirmation without a drawer and restores it after close", () => {
    const { rerender } = render(<ConfirmDialog open title="确认？" description="确认说明" confirmLabel="确认" onCancel={() => undefined} onConfirm={() => undefined} />);
    expect(document.body).toHaveClass("modal-open");
    expect(document.documentElement).toHaveClass("modal-open");
    rerender(<ConfirmDialog open={false} title="确认？" description="确认说明" confirmLabel="确认" onCancel={() => undefined} onConfirm={() => undefined} />);
    expect(document.body).not.toHaveClass("modal-open");
    expect(document.documentElement).not.toHaveClass("modal-open");
  });

  it("focuses the dialog surface when its preferred action is disabled", () => {
    render(<ConfirmDialog open pending title="正在处理" description="请稍候" confirmLabel="确认" onCancel={() => undefined} onConfirm={() => undefined} />);
    expect(screen.getByRole("alertdialog", { name: "正在处理" })).toHaveFocus();
  });

  it("restores focus to the lower modal trigger after a nested dialog closes", async () => {
    const user = userEvent.setup();
    function NestedExample() {
      const [confirming, setConfirming] = useState(false);
      return <><Drawer open title="规则详情" eyebrow="规则" onClose={() => undefined}><button onClick={() => setConfirming(true)}>打开确认</button></Drawer><ConfirmDialog open={confirming} title="确认？" description="确认说明" confirmLabel="确认" onCancel={() => setConfirming(false)} onConfirm={() => undefined} /></>;
    }
    render(<NestedExample />);
    const trigger = screen.getByRole("button", { name: "打开确认" });
    await user.click(trigger);
    const confirmation = screen.getByRole("alertdialog", { name: "确认？" });
    expect(trigger.closest("[inert]")).not.toBeNull();
    await user.keyboard("{Escape}");
    expect(confirmation).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
    expect(document.body).toHaveClass("modal-open");
  });

  it("falls back to the lower modal when the original trigger becomes disabled", async () => {
    const user = userEvent.setup();
    function DisabledTriggerExample() {
      const [confirming, setConfirming] = useState(false);
      const [disabled, setDisabled] = useState(false);
      return <><Drawer open title="规则详情" eyebrow="规则" onClose={() => undefined}><button disabled={disabled} onClick={() => setConfirming(true)}>打开确认</button></Drawer><ConfirmDialog open={confirming} title="确认？" description="确认说明" confirmLabel="确认" onCancel={() => setConfirming(false)} onConfirm={() => undefined}><button onClick={() => setDisabled(true)}>禁用下层触发按钮</button></ConfirmDialog></>;
    }
    render(<DisabledTriggerExample />);
    await user.click(screen.getByRole("button", { name: "打开确认" }));
    await user.click(screen.getByRole("button", { name: "禁用下层触发按钮" }));
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: "规则详情" })).toContainElement(document.activeElement as HTMLElement);
  });

  it("does not trap login keyboard input in a workspace drawer hidden by session expiry", async () => {
    const user = userEvent.setup();
    const closeDrawer = vi.fn();
    const contents = (hidden: boolean) => <>
      <div hidden={hidden} aria-hidden={hidden}>
        <Drawer open title="未提交的规则" eyebrow="规则" onClose={closeDrawer}><input aria-label="规则名称" /></Drawer>
      </div>
      {hidden && <form><input aria-label="登录用户名" /><input aria-label="登录密码" /></form>}
    </>;
    const view = render(contents(false));
    view.rerender(contents(true));
    screen.getByLabelText("登录用户名").focus();
    await user.tab();
    expect(screen.getByLabelText("登录密码")).toHaveFocus();
    expect(screen.getByLabelText("登录用户名").closest("[inert]")).toBeNull();
    expect(document.body).not.toHaveClass("modal-open");
    expect(document.documentElement).not.toHaveClass("modal-open");
    await user.keyboard("{Escape}");
    expect(closeDrawer).not.toHaveBeenCalled();
  });

});
