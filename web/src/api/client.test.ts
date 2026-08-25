import { z } from "zod";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, request, shouldRetryQuery } from "./client";

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

  it("retries only transient reads once", () => {
    expect(shouldRetryQuery(0, new ApiError("network_error", "offline", 0))).toBe(true);
    expect(shouldRetryQuery(0, new ApiError("policy_catalog_unavailable", "down", 503))).toBe(true);
    expect(shouldRetryQuery(1, new ApiError("network_error", "offline", 0))).toBe(false);
    expect(shouldRetryQuery(0, new ApiError("query_policy_not_found", "missing", 404))).toBe(false);
  });
});
