import { ApiError } from "../../api/client";
import { LeaveProtectionProvider } from "../../components/ui/LeaveProtection";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { TestRouter } from "../../test/TestRouter";
import { ToastProvider } from "../../components/ui/Toast";
import { usePolicyLifecycleCommands } from "./lifecycle";

const copy = { title: "Activate", description: "Activate", label: "Confirm", success: "Done" };
function Wrapper({ children }: { children: ReactNode }) {
  return <TestRouter initialEntries={["/platform/query-policies"]}><ToastProvider><LeaveProtectionProvider>{children}</LeaveProtectionProvider></ToastProvider></TestRouter>;
}
describe("lifecycle submission boundary", () => {
  it("accepts only one confirmation before React publishes pending state", () => {
    const run = vi.fn();
    const runner = { run, pending: false };
    const hook = renderHook(() => usePolicyLifecycleCommands({
      collectionPath: "/platform/query-policies", copy: { activate: copy, deprecate: copy, delete: copy },
      runners: { activate: runner, deprecate: runner, delete: runner },
    }), { wrapper: Wrapper });
    act(() => hook.result.current.request("activate", "test_query_v1"));
    act(() => { hook.result.current.execute(); hook.result.current.execute(); });
    expect(run).toHaveBeenCalledTimes(1);
  });
});


it("remembers an uncertain target without blocking other policies, until explicit recovery", () => {
  const callbacks = new Map<string, { success: () => void; error: (error: unknown) => void }>();
  const run = vi.fn((code: string, success: () => void, error: (error: unknown) => void) => callbacks.set(code, { success, error }));
  const runner = { run, pending: false };
  const hook = renderHook(() => usePolicyLifecycleCommands({
    collectionPath: "/platform/query-policies", copy: { activate: copy, deprecate: copy, delete: copy },
    runners: { activate: runner, deprecate: runner, delete: runner },
  }), { wrapper: Wrapper });
  act(() => hook.result.current.request("activate", "A_v1"));
  act(() => hook.result.current.execute());
  const error = new ApiError("policy_catalog_unavailable", "unknown", 503);
  act(() => callbacks.get("A_v1")!.error(error));
  act(() => hook.result.current.execute());
  expect(run).toHaveBeenCalledTimes(1);
  act(() => hook.result.current.cancel());
  act(() => hook.result.current.request("activate", "B_v1"));
  act(() => hook.result.current.execute());
  expect(run).toHaveBeenCalledTimes(2);
  act(() => callbacks.get("B_v1")!.success());
  act(() => hook.result.current.request("activate", "A_v1"));
  expect(hook.result.current.recovery.error).toBe(error);
  act(() => hook.result.current.execute());
  expect(run).toHaveBeenCalledTimes(2);
  act(() => hook.result.current.finishCheck());
  act(() => hook.result.current.request("activate", "A_v1"));
  act(() => hook.result.current.execute());
  expect(run).toHaveBeenCalledTimes(3);
});
