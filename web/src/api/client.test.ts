import { z } from "zod";
import { afterEach, describe, expect, it, vi } from "vitest";
import { businessSessionInvalid, setBusinessSession } from "./business-session";
import { ApiError, request, shouldRetryQuery, isUncertainWriteError } from "./client";

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

  it("preserves response headers when the response body stream fails", async () => {
    const cause = new TypeError("response stream interrupted");
    const body = new ReadableStream({ start(controller) { controller.enqueue(new TextEncoder().encode('{"id":')); controller.error(cause); } });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(body, {
      status: 201, headers: { "X-Request-ID": "req-body-interrupted" },
    })));
    await expect(request("/api/test", { method: "POST" })).rejects.toMatchObject({
      name: "ApiError", code: "network_error", status: 201, requestId: "req-body-interrupted", cause,
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


describe("uncertain writes", () => {
  it.each([
    ["network_error", 0], ["network_error", 201], ["contract_mismatch", 200],
    ["unexpected_response", 400], ["mutation_unavailable", 503], ["policy_catalog_unavailable", 503],
    ["query_policy_not_found", 404], ["mutation_policy_not_found", 404], ["table_policy_not_found", 404],
    ["mutation_timeout", 504], ["internal_error", 500], ["unrecognized_rejection", 422],
  ])("treats %s/%i as potentially committed", (code, status) => {
    expect(isUncertainWriteError(new ApiError(code, "message", status))).toBe(true);
  });
  it.each(["duplicate_key", "invalid_mutation_content", "query_policy_exists", "invalid_policy_transition", "session_invalid", "account_disabled", "csrf_invalid"])("allows correction after recognized %s rejection", (code) => {
    expect(isUncertainWriteError(new ApiError(code, "rejected", 422))).toBe(false);
  });
  it.each([
    "release_auto_id_ambiguous", "publication_unsupported", "publication_metadata_permission",
    "release_snapshot_unsupported", "release_metadata_permission", "release_cross_table",
    "release_item_limit", "release_result_limit", "release_field_limit", "release_duplicate_target",
    "release_invalid", "rollback_locked", "rollback_restore_mismatch",
    "concurrency_key_invalid", "concurrency_key_in_use", "concurrency_key_value_required",
  ])("allows correction after recognized release %s rejection", (code) => {
    expect(isUncertainWriteError(new ApiError(code, "release rejected", 422))).toBe(false);
  });
  it.each(["permission_denied", "authentication_required", "release_not_found", "idempotency_conflict"])("keeps an original request after non-conclusive release %s", (code) => {
    expect(isUncertainWriteError(new ApiError(code, "not conclusive", 422))).toBe(true);
  });
  it("does not infer a rejection from an unknown exception or absent error", () => {
    expect(isUncertainWriteError(new TypeError("unknown"))).toBe(true);
    expect(isUncertainWriteError(null)).toBe(false);
  });
});
