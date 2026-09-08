import { ApiError } from "../../api/client";
import { LeaveProtectionProvider } from "../../components/ui/LeaveProtection";
import { act, renderHook, screen } from "@testing-library/react";
import { useLocation, useNavigate } from "react-router-dom";
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


it("blocks back navigation as soon as the lifecycle request starts, before pending is published", async () => {
  const run = vi.fn();
  const runner = { run, pending: false };
  const detail = "/platform/query-policies/rapid_v1";
  const wrapper = ({ children }: { children: ReactNode }) => <TestRouter initialEntries={["/platform/query-policies", detail]}><ToastProvider><LeaveProtectionProvider>{children}</LeaveProtectionProvider></ToastProvider></TestRouter>;
  const hook = renderHook(() => {
    const commands = usePolicyLifecycleCommands({ selectedCode: "rapid_v1", collectionPath: "/platform/query-policies", copy: { activate: copy, deprecate: copy, delete: copy }, runners: { activate: runner, deprecate: runner, delete: runner } });
    return { commands, navigate: useNavigate(), location: useLocation() };
  }, { wrapper });
  act(() => hook.result.current.commands.request("activate", "rapid_v1"));
  act(() => hook.result.current.commands.execute());
  expect(run).toHaveBeenCalledTimes(1);
  // The request is already dispatched while the async mutation observer still
  // reports pending=false. This is the gap reproduced by the native browser.
  act(() => { void hook.result.current.navigate(-1); });
  expect(await screen.findByRole("alertdialog", { name: "正在提交，请稍候" })).toBeVisible();
  expect(hook.result.current.location.pathname).toBe(detail);
  expect(run).toHaveBeenCalledTimes(1);
  act(() => run.mock.calls[0]![1]());
  expect(screen.queryByRole("alertdialog", { name: "正在提交，请稍候" })).not.toBeInTheDocument();
});


it("returns to the catalog after deleting the selected draft while the observer still reports pending", async () => {
  const run = vi.fn();
  const detail = "/platform/query-policies/deleting_v1";
  const wrapper = ({ children }: { children: ReactNode }) => <TestRouter initialEntries={[detail]}><ToastProvider><LeaveProtectionProvider>{children}</LeaveProtectionProvider></ToastProvider></TestRouter>;
  const hook = renderHook(({ pending }: { pending: boolean }) => {
    const runner = { run, pending };
    const commands = usePolicyLifecycleCommands({ selectedCode: "deleting_v1", collectionPath: "/platform/query-policies", copy: { activate: copy, deprecate: copy, delete: copy }, runners: { activate: runner, deprecate: runner, delete: runner } });
    return { commands, location: useLocation() };
  }, { wrapper, initialProps: { pending: false } });
  act(() => hook.result.current.commands.request("delete", "deleting_v1"));
  act(() => hook.result.current.commands.execute());
  hook.rerender({ pending: true });
  expect(hook.result.current.commands.pending).toBe(true);
  act(() => run.mock.calls[0]![1]());
  expect(hook.result.current.location.pathname).toBe("/platform/query-policies");
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  expect(run).toHaveBeenCalledTimes(1);
});


it("returns to the catalog after an uncertain command while the observer still reports pending", () => {
  const run = vi.fn();
  const detail = "/platform/query-policies/uncertain_v1";
  const collection = "/platform/query-policies";
  const wrapper = ({ children }: { children: ReactNode }) => <TestRouter initialEntries={[detail]}><ToastProvider><LeaveProtectionProvider>{children}</LeaveProtectionProvider></ToastProvider></TestRouter>;
  const hook = renderHook(({ pending }: { pending: boolean }) => {
    const navigate = useNavigate();
    const runner = { run, pending };
    const commands = usePolicyLifecycleCommands({
      selectedCode: "uncertain_v1",
      collectionPath: collection,
      copy: { activate: copy, deprecate: copy, delete: copy },
      runners: { activate: runner, deprecate: runner, delete: runner },
      onUncertainWrite: (_error, code) => { if (code === "uncertain_v1") navigate(collection); },
    });
    return { commands, location: useLocation() };
  }, { wrapper, initialProps: { pending: false } });
  act(() => hook.result.current.commands.request("activate", "uncertain_v1"));
  act(() => hook.result.current.commands.execute());
  hook.rerender({ pending: true });
  expect(hook.result.current.commands.pending).toBe(true);
  act(() => run.mock.calls[0]![2](new ApiError("policy_catalog_unavailable", "unknown", 503)));
  expect(hook.result.current.location.pathname).toBe(collection);
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  expect(hook.result.current.commands.recovery.error).toBeTruthy();
  expect(run).toHaveBeenCalledTimes(1);
});
