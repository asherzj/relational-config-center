import { businessSession, businessSessionInvalid } from "./business-session";
import type { ZodType } from "zod";
import { adminErrorDtoSchema } from "./contracts";

export type ClientErrorCode =
  | "network_error"
  | "contract_mismatch"
  | "unexpected_response"
  | string;

export class ApiError extends Error {
  constructor(
    public readonly code: ClientErrorCode,
    message: string,
    public readonly status: number,
    public readonly requestId?: string,
    options?: ErrorOptions,
    public readonly retryAfter?: number,
  ) {
    super(message, options);
    this.name = "ApiError";
  }
}

type RequestOptions<T> = RequestInit & {
  schema?: ZodType<T>;
};

async function parseJson(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) return undefined;
  try {
    return JSON.parse(text);
  } catch (cause) {
    throw new ApiError(
      "contract_mismatch",
      "Admin 返回了无法解析的响应。",
      response.status,
      response.headers.get("X-Request-ID") ?? undefined,
      { cause },
    );
  }
}

export async function request<T>(path: string, options: RequestOptions<T> = {}): Promise<T> {
  const { schema, headers, ...init } = options;
  const business = path.startsWith("/api/v1/") && !path.startsWith("/api/v1/auth/");
  const session = businessSession();
  const change = !["GET", "HEAD"].includes(init.method ?? "GET");
  let response: Response;

  try {
    response = await fetch(path, {
      ...init,
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        ...(init.body ? { "Content-Type": "application/json" } : {}),
        ...(business && change && session.credentials ? { "X-CSRF-Token": session.credentials.csrf } : {}),
        ...headers,
      },
    });
  } catch (cause) {
    if (business && session.generation !== businessSession().generation) {
      throw new ApiError("stale_session", "登录状态已变化，请重新查询。", 0);
    }
    throw new ApiError("network_error", "无法连接 Admin，请检查服务状态后重试。", 0, undefined, { cause });
  }

  const responseAccountID = response.headers.get("X-RCC-Account-ID");
  if (business && session.generation !== businessSession().generation) {
    throw new ApiError("stale_session", "登录状态已变化，请重新查询。", 0);
  }
  if (business && session.credentials && responseAccountID && responseAccountID !== session.credentials.accountID) {
    window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: "account_changed" } }));
    throw new ApiError("stale_session", "登录账号已变化，请重新查询。", 0);
  }
  let payload: unknown;
  try {
    payload = await parseJson(response);
  } catch (cause) {
    if (business && session.generation !== businessSession().generation) {
      throw new ApiError("stale_session", "登录状态已变化，请重新查询。", 0);
    }
    if (cause instanceof ApiError) throw cause;
    throw new ApiError("network_error", "读取 Admin 响应时连接中断，请重新查询。", response.status, response.headers.get("X-Request-ID") ?? undefined, { cause });
  }
  if (business && session.generation !== businessSession().generation) {
    throw new ApiError("stale_session", "登录状态已变化，请重新查询。", 0);
  }
  if (!response.ok) {
    const parsedError = adminErrorDtoSchema.safeParse(payload);
    if (parsedError.success) {
      if (business && response.status === 401) window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: parsedError.data.error.code } }));
      throw new ApiError(
        parsedError.data.error.code,
        parsedError.data.error.message,
        response.status,
        parsedError.data.error.request_id,
        undefined,
        response.headers.has("Retry-After") ? Number(response.headers.get("Retry-After")) : undefined,
      );
    }
    throw new ApiError(
      "unexpected_response",
      "Admin 返回了未识别的错误。",
      response.status,
      response.headers.get("X-Request-ID") ?? undefined,
    );
  }

  if (!schema) return payload as T;
  const result = schema.safeParse(payload);
  if (!result.success) {
    throw new ApiError(
      "contract_mismatch",
      "Admin 响应与 Web 契约不一致。",
      response.status,
      response.headers.get("X-Request-ID") ?? undefined,
      { cause: result.error },
    );
  }
  return result.data;
}

export function shouldRetryQuery(failureCount: number, error: unknown): boolean {
  if (failureCount >= 1 || !(error instanceof ApiError)) return false;
  return error.code === "network_error" || error.status === 503 || error.status === 504;
}

export function isUncertainWriteError(error: unknown): boolean {
  return error instanceof ApiError
    && (error.code === "network_error"
      || error.code === "contract_mismatch"
      || error.code === "unexpected_response"
      || error.code === "stale_session"
      || error.status === 504);
}
