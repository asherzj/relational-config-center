import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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

function usePolicyCommand<TVariables>(command: (variables: TVariables) => Promise<QueryPolicy>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: command,
    retry: false,
    onSuccess(policy) {
      client.setQueryData(queryPolicyKeys.detail(policy.code), policy);
      void client.invalidateQueries({ queryKey: queryPolicyKeys.list });
    },
  });
}

export function useCreateQueryPolicy() {
  return usePolicyCommand(createQueryPolicy);
}

export function useReplaceQueryPolicy() {
  return usePolicyCommand(({ code, draft }: { code: string; draft: QueryPolicyDraft }) =>
    replaceQueryPolicy(code, draft),
  );
}

export function useActivateQueryPolicy() {
  return usePolicyCommand(activateQueryPolicy);
}

export function useDeprecateQueryPolicy() {
  return usePolicyCommand(deprecateQueryPolicy);
}

export function useUpdateQueryPolicyMetadata() {
  return usePolicyCommand(({ code, metadata }: { code: string; metadata: QueryPolicyMetadata }) =>
    updateQueryPolicyMetadata(code, metadata),
  );
}

export function useDeleteQueryPolicy() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: deleteQueryPolicy,
    retry: false,
    onSuccess(_, code) {
      client.removeQueries({ queryKey: queryPolicyKeys.detail(code) });
      void client.invalidateQueries({ queryKey: queryPolicyKeys.list });
    },
  });
}
