import { createElement, type PropsWithChildren } from "react";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../../api/client";
import { LeaveProtectionProvider } from "../../components/ui/LeaveProtection";
import { ToastProvider } from "../../components/ui/Toast";
import { TestRouter } from "../../test/TestRouter";
import { policyActionAvailability, policyTypeAvailabilityHint, requestedPolicyFormMode, resolvePolicyFormMode, usePolicyLifecycleCommands, type PolicyCommandCopy } from "./lifecycle";

describe("policy lifecycle rules", () => {
  it("keeps safe display metadata editable when execution support is unknown", () => {
    expect(policyActionAvailability("ACTIVE", false)).toEqual({
      replace: false,
      activate: false,
      delete: false,
      deprecate: false,
      metadata: true,
    });
    expect(resolvePolicyFormMode("metadata", "ACTIVE", false)).toBe("metadata");
    expect(resolvePolicyFormMode("replace", "DRAFT", false)).toBe("view");
  });

  it("derives form modes from the same lifecycle rules for direct URLs", () => {
    expect(requestedPolicyFormMode(false, "edit")).toBe("replace");
    expect(resolvePolicyFormMode("replace", "ACTIVE", true)).toBe("view");
    expect(resolvePolicyFormMode("metadata", "DRAFT", true)).toBe("view");
    expect(resolvePolicyFormMode("metadata", "DEPRECATED", true)).toBe("metadata");
  });

  it("describes the same safe fallback that the row actions expose", () => {
    expect(policyTypeAvailabilityHint("DRAFT", false)).toBe("仅可查看");
    expect(policyTypeAvailabilityHint("ACTIVE", false)).toBe("仅可修改名称和描述");
    expect(policyTypeAvailabilityHint("DEPRECATED", false)).toBe("仅可修改名称和描述");
    expect(policyTypeAvailabilityHint("ACTIVE", true)).toBeNull();
  });
});

const commandCopy: PolicyCommandCopy = {
  activate: { title: "激活？", description: "激活规则", label: "确认激活", success: "已激活" },
  deprecate: { title: "弃用？", description: "弃用规则", label: "确认弃用", success: "已弃用" },
  delete: { title: "删除？", description: "删除草稿", label: "确认删除", success: "已删除" },
};

function lifecycleWrapper({ children }: PropsWithChildren) {
  return createElement(TestRouter, {
    initialEntries: ["/policies"],
    children: createElement(ToastProvider, null, createElement(LeaveProtectionProvider, null, children)),
  });
}

function renderCommands() {
  const calls: { code: string; succeed: () => void; fail: (error: unknown) => void }[] = [];
  const run = vi.fn((code: string, succeed: () => void, fail: (error: unknown) => void) => {
    calls.push({ code, succeed, fail });
  });
  const onUncertainWrite = vi.fn();
  const hook = renderHook(({ blocked }) => usePolicyLifecycleCommands({
    collectionPath: "/policies",
    copy: commandCopy,
    runners: {
      activate: { run, pending: false },
      deprecate: { run, pending: false },
      delete: { run, pending: false },
    },
    blocked,
    onUncertainWrite,
  }), { initialProps: { blocked: false }, wrapper: lifecycleWrapper });
  return { ...hook, calls, run, onUncertainWrite };
}

describe("policy lifecycle command submissions", () => {
  it("sends one command before pending renders and allows a new command after success", () => {
    const { result, run, calls } = renderCommands();
    act(() => result.current.request("activate", "draft_v1"));
    act(() => {
      result.current.execute();
      result.current.cancel();
      result.current.request("delete", "other_v1");
      result.current.execute();
    });
    expect(run).toHaveBeenCalledTimes(1);
    expect(calls[0].code).toBe("draft_v1");
    expect(result.current.confirm).toBe(commandCopy.activate);

    act(() => {
      calls[0].succeed();
      result.current.execute();
    });
    expect(run).toHaveBeenCalledTimes(1);
    expect(result.current.confirm).toBeNull();
    act(() => result.current.request("deprecate", "draft_v1"));
    act(() => result.current.execute());
    expect(run).toHaveBeenCalledTimes(2);
  });

  it("releases a rejected command for explicit retry without allowing late callbacks to unlock the retry", () => {
    const { result, run, calls } = renderCommands();
    act(() => result.current.request("activate", "draft_v1"));
    act(() => result.current.execute());
    act(() => calls[0].fail(new ApiError("invalid_policy_transition", "当前规则不可激活", 409, "req-conflict")));
    expect(result.current.confirm).toBeNull();
    act(() => result.current.request("activate", "draft_v1"));
    act(() => result.current.execute());
    expect(run).toHaveBeenCalledTimes(2);

    act(() => {
      calls[0].succeed();
      result.current.execute();
    });
    expect(run).toHaveBeenCalledTimes(2);
    expect(result.current.confirm).toBe(commandCopy.activate);
    act(() => calls[1].succeed());
    expect(result.current.confirm).toBeNull();
  });

  it("keeps an uncertain stale-session result blocked until a read-only check permits a newly confirmed command", () => {
    const { result, rerender, run, calls, onUncertainWrite } = renderCommands();
    act(() => result.current.request("activate", "draft_v1"));
    act(() => result.current.execute());
    const error = new ApiError("stale_session", "登录状态已变化，请重新查询。", 0);
    act(() => calls[0].fail(error));
    expect(onUncertainWrite).toHaveBeenCalledExactlyOnceWith(error, "draft_v1");
    rerender({ blocked: true });
    act(() => {
      result.current.request("activate", "draft_v1");
      result.current.execute();
    });
    expect(run).toHaveBeenCalledTimes(1);
    expect(result.current.confirm).toBeNull();

    rerender({ blocked: false });
    act(() => result.current.execute());
    expect(run).toHaveBeenCalledTimes(1);
    act(() => result.current.finishCheck());
    act(() => result.current.request("activate", "draft_v1"));
    act(() => result.current.execute());
    expect(run).toHaveBeenCalledTimes(2);
  });
});
