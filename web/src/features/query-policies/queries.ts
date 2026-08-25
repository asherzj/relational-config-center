import { useQuery } from "@tanstack/react-query";
import { shouldRetryQuery } from "../../api/client";
import {
  activateQueryPolicy,
  createQueryPolicy,
  deleteQueryPolicy,
  deprecateQueryPolicy,
  getQueryPolicy,
  listQueryPolicies,
  listQueryPolicyTypes,
  replaceQueryPolicy,
  updateQueryPolicyMetadata,
} from "../../api/query-policies";
import type { QueryPolicy, QueryPolicyDraft, QueryPolicyMetadata } from "./model";
import { useDeletePolicyCommand, usePolicyCommand } from "../policies/queries";

export const queryPolicyKeys = {
  all: ["query-policies"] as const,
  list: ["query-policies", "list"] as const,
  types: ["query-policy-types"] as const,
  detail: (code: string) => ["query-policies", "detail", code] as const,
};

export function useQueryPolicies() {
  return useQuery({ queryKey: queryPolicyKeys.list, queryFn: listQueryPolicies, retry: shouldRetryQuery });
}

export function useQueryPolicyTypes() {
  return useQuery({ queryKey: queryPolicyKeys.types, queryFn: listQueryPolicyTypes, retry: shouldRetryQuery });
}

export function useQueryPolicy(code?: string) {
  return useQuery({
    queryKey: queryPolicyKeys.detail(code ?? ""),
    queryFn: () => getQueryPolicy(code!),
    enabled: Boolean(code),
    retry: shouldRetryQuery,
  });
}

function useQueryPolicyCommand<TVariables>(command: (variables: TVariables) => Promise<QueryPolicy>) {
  return usePolicyCommand(command, queryPolicyKeys.detail, queryPolicyKeys.list);
}

export function useCreateQueryPolicy() {
  return useQueryPolicyCommand(createQueryPolicy);
}

export function useReplaceQueryPolicy() {
  return useQueryPolicyCommand(({ code, draft }: { code: string; draft: QueryPolicyDraft }) =>
    replaceQueryPolicy(code, draft),
  );
}

export function useActivateQueryPolicy() {
  return useQueryPolicyCommand(activateQueryPolicy);
}

export function useDeprecateQueryPolicy() {
  return useQueryPolicyCommand(deprecateQueryPolicy);
}

export function useUpdateQueryPolicyMetadata() {
  return useQueryPolicyCommand(({ code, metadata }: { code: string; metadata: QueryPolicyMetadata }) =>
    updateQueryPolicyMetadata(code, metadata),
  );
}

export function useDeleteQueryPolicy() {
  return useDeletePolicyCommand(deleteQueryPolicy, queryPolicyKeys.detail, queryPolicyKeys.list);
}
