import { z } from "zod";
import { afterEach, describe, expect, it, vi } from "vitest";
import { businessSessionInvalid, setBusinessSession } from "./business-session";
import { ApiError, request, shouldRetryQuery } from "./client";

afterEach(() => { vi.unstubAllGlobals(); setBusinessSession(null); });

describe("Admin API client", () => {
  it("validates successful responses at the boundary", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ value: "ok" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })));

    await expect(request("/api/test", { schema: z.object({ value: z.string() }) })).resolves.toEqual({ value: "ok" });
  });

  it("preserves stable Admin error codes and request IDs", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "query_policy_not_found", message: "not found", request_id: "req-42" },
    }), { status: 404, headers: { "Content-Type": "application/json" } })));

    await expect(request("/api/test")).rejects.toMatchObject({
      name: "ApiError",
      code: "query_policy_not_found",
      status: 404,
      requestId: "req-42",
    });
  });

  it("fails closed when the response violates the Web contract", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ value: 7 }), {
      status: 200,
      headers: { "Content-Type": "application/json", "X-Request-ID": "req-contract" },
    })));

    await expect(request("/api/test", { schema: z.object({ value: z.string() }) })).rejects.toMatchObject({
      code: "contract_mismatch",
      requestId: "req-contract",
    });
  });

  it("retries only transient reads once", () => {
    expect(shouldRetryQuery(0, new ApiError("network_error", "offline", 0))).toBe(true);
    expect(shouldRetryQuery(0, new ApiError("policy_catalog_unavailable", "down", 503))).toBe(true);
    expect(shouldRetryQuery(1, new ApiError("network_error", "offline", 0))).toBe(false);
    expect(shouldRetryQuery(0, new ApiError("query_policy_not_found", "missing", 404))).toBe(false);
  });

  it("rejects a business response authenticated as a different Cookie account before exposing its body", async () => {
    setBusinessSession({ accountID: "ab09850e-ef9a-4317-a000-d67465416b5b", csrf: "alice-csrf" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ secret: "bob-data" }), {
      status: 200,
      headers: { "Content-Type": "application/json", "X-RCC-Account-ID": "9e5e2b50-6aaa-4eaa-83fb-f56d84240214" },
    })));
    const changed = vi.fn();
    window.addEventListener(businessSessionInvalid, changed, { once: true });

    await expect(request("/api/v1/query-policies")).rejects.toMatchObject({ code: "stale_session" });
    expect(changed).toHaveBeenCalledOnce();
  });

  it("classifies a response body stream failure after a write as an uncertain network result", async () => {
    setBusinessSession({ accountID: "ab09850e-ef9a-4317-a000-d67465416b5b", csrf: "alice-csrf" });
    const body = new ReadableStream({ start(controller) { controller.error(new TypeError("response body disconnected")); } });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(body, {
      status: 201,
      headers: { "Content-Type": "application/json", "X-RCC-Account-ID": "ab09850e-ef9a-4317-a000-d67465416b5b" },
    })));

    await expect(request("/api/v1/query-policies", { method: "POST", body: "{}" })).rejects.toMatchObject({ code: "network_error" });
  });

  it.each([
    ["success", new Response(JSON.stringify({ secret: "new-account-data" }), { status: 200, headers: { "Content-Type": "application/json", "X-RCC-Account-ID": "9e5e2b50-6aaa-4eaa-83fb-f56d84240214" } })],
    ["401", new Response(JSON.stringify({ error: { code: "session_invalid", message: "old", request_id: "old-request" } }), { status: 401, headers: { "Content-Type": "application/json" } })],
    ["network error", new TypeError("old connection failed")],
  ])("turns a late old-account %s into a stale result", async (_kind, result) => {
    setBusinessSession({ accountID: "ab09850e-ef9a-4317-a000-d67465416b5b", csrf: "alice-csrf" });
    let finish!: (value: Response) => void;
    let fail!: (error: unknown) => void;
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((resolve, reject) => { finish = resolve; fail = reject; })));
    const invalid = vi.fn();
    window.addEventListener(businessSessionInvalid, invalid, { once: true });
    const pending = request("/api/v1/query-policies");
    setBusinessSession({ accountID: "9e5e2b50-6aaa-4eaa-83fb-f56d84240214", csrf: "bob-csrf" });
    if (result instanceof Response) finish(result); else fail(result);
    await expect(pending).rejects.toMatchObject({ code: "stale_session" });
    expect(invalid).not.toHaveBeenCalled();
  });
});
