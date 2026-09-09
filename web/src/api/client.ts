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
    public readonly itemIndex?: number,
    public readonly fieldName?: string,
  ) {
    super(message, options);
    this.name = "ApiError";
  }
}

type RequestOptions<T> = RequestInit & {
  schema?: ZodType<T>;
};

async function parseJson(response: Response): Promise<unknown> {
  let text: string;
  try {
    text = await response.text();
  } catch (cause) {
    throw new ApiError("network_error", "Admin 响应传输中断。", response.status,
      response.headers.get("X-Request-ID") ?? undefined, { cause });
  }
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
      if (business && parsedError.data.error.code === "permission_denied") window.dispatchEvent(new Event("rcc:account-roles-changed"));
      if (business && response.status === 401) window.dispatchEvent(new CustomEvent(businessSessionInvalid, { detail: { code: parsedError.data.error.code } }));
      throw new ApiError(
        parsedError.data.error.code,
        parsedError.data.error.message,
        response.status,
        parsedError.data.error.request_id,
        undefined,
        response.headers.has("Retry-After") ? Number(response.headers.get("Retry-After")) : undefined,
        parsedError.data.error.item_index,
        parsedError.data.error.field_name,
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

// Only recognized rejection responses prove that this request was not accepted.
// A 5xx may follow a commit or a failed read of the just-written policy.
// Catalog not-found responses may also originate from that post-write read.
const definiteWriteRejections = new Set([
  "invalid_field_policy", "invalid_policy_code", "invalid_query_policy_definition", "query_policy_exists",
  "invalid_policy_transition", "unknown_policy_type", "invalid_query_policy_rules",
  "invalid_mutation_policy_definition", "mutation_policy_exists", "invalid_mutation_policy_rules",
  "invalid_policy_definition", "database_table_not_found", "table_policy_exists",
  "query_policy_not_assignable", "mutation_policy_not_assignable", "incompatible_policy_definition", "incompatible_table",
  "protected_table", "table_policy_disabled", "invalid_policy_snapshot", "mutation_not_allowed", "mutation_row_not_found",
  "invalid_mutation_content", "missing_required_field", "duplicate_key", "request_body_too_large", "invalid_request",
  "unauthorized", "cors_origin_forbidden", "cors_preflight_forbidden",
  "session_invalid", "account_disabled", "csrf_invalid",
  "account_roles_conflict",
  // Release input and capability checks happen before the request can be
  // accepted. Keep authentication, permission, not-found and idempotency
  // responses conservative because they do not prove the original outcome.
  "release_auto_id_ambiguous", "publication_unsupported", "publication_metadata_permission",
  "release_snapshot_unsupported", "release_metadata_permission", "release_cross_table",
  "release_item_limit", "release_result_limit", "release_field_limit", "release_duplicate_target",
  "release_invalid", "rollback_locked", "rollback_restore_mismatch",
]);

export function isUncertainWriteError(error: unknown): boolean {
  if (error === null || error === undefined) return false;
  return !(error instanceof ApiError && error.status >= 400 && error.status < 500
    && definiteWriteRejections.has(error.code));
}

// An earlier explicit rejection must never hide a later unresolved write.
export function prioritizeUncertainWriteError<T>(errors: readonly T[]): T | undefined {
  return errors.find(isUncertainWriteError) ?? errors.find(Boolean);
}
