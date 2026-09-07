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
  let response: Response;

  try {
    response = await fetch(path, {
      ...init,
      headers: {
        Accept: "application/json",
        ...(init.body ? { "Content-Type": "application/json" } : {}),
        ...headers,
      },
    });
  } catch (cause) {
    throw new ApiError("network_error", "无法连接 Admin，请检查服务状态后重试。", 0, undefined, { cause });
  }

  const payload = await parseJson(response);
  if (!response.ok) {
    const parsedError = adminErrorDtoSchema.safeParse(payload);
    if (parsedError.success) {
      throw new ApiError(
        parsedError.data.error.code,
        parsedError.data.error.message,
        response.status,
        parsedError.data.error.request_id,
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
  "invalid_policy_code", "invalid_query_policy_definition", "query_policy_exists",
  "invalid_policy_transition", "unknown_policy_type", "invalid_query_policy_rules",
  "invalid_mutation_policy_definition", "mutation_policy_exists", "invalid_mutation_policy_rules",
  "invalid_policy_definition", "database_table_not_found", "table_policy_exists",
  "query_policy_not_assignable", "mutation_policy_not_assignable", "incompatible_policy_definition", "incompatible_table",
  "protected_table", "table_policy_disabled", "invalid_policy_snapshot", "mutation_not_allowed", "mutation_row_not_found",
  "invalid_mutation_content", "missing_required_field", "duplicate_key", "request_body_too_large", "invalid_request",
  "unauthorized", "cors_origin_forbidden", "cors_preflight_forbidden",
]);

export function isUncertainWriteError(error: unknown): boolean {
  if (error === null || error === undefined) return false;
  return !(error instanceof ApiError && error.status >= 400 && error.status < 500
    && definiteWriteRejections.has(error.code));
}
