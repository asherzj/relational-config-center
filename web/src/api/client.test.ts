import { z } from "zod";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, request, shouldRetryQuery, isUncertainWriteError } from "./client";

afterEach(() => vi.unstubAllGlobals());

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
  it.each(["duplicate_key", "invalid_mutation_content", "query_policy_exists", "invalid_policy_transition"])("allows correction after recognized %s rejection", (code) => {
    expect(isUncertainWriteError(new ApiError(code, "rejected", 422))).toBe(false);
  });
  it("does not infer a rejection from an unknown exception or absent error", () => {
    expect(isUncertainWriteError(new TypeError("unknown"))).toBe(true);
    expect(isUncertainWriteError(null)).toBe(false);
  });
});
