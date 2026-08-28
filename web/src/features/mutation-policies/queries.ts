import { useQuery } from "@tanstack/react-query";
import { shouldRetryQuery } from "../../api/client";
import {
  activateMutationPolicy,
  createMutationPolicy,
  deleteMutationPolicy,
  deprecateMutationPolicy,
  getMutationPolicy,
  listMutationPolicies,
  listMutationPolicyTypes,
  replaceMutationPolicy,
  updateMutationPolicyMetadata,
} from "../../api/mutation-policies";
import type { MutationPolicy, MutationPolicyDraft, MutationPolicyMetadata } from "./model";
import { useDeletePolicyCommand, usePolicyCommand as useSharedPolicyCommand } from "../policies/queries";

export const mutationPolicyKeys = {
  list: ["mutation-policies", "list"] as const,
  types: ["mutation-policy-types"] as const,
  detail: (code: string) => ["mutation-policies", "detail", code] as const,
};

export function useMutationPolicies(enabled = true) {
  return useQuery({ queryKey: mutationPolicyKeys.list, queryFn: listMutationPolicies, retry: shouldRetryQuery, enabled });
}

export function useMutationPolicyTypes(enabled = true) {
  return useQuery({ queryKey: mutationPolicyKeys.types, queryFn: listMutationPolicyTypes, retry: shouldRetryQuery, enabled });
}

export function useMutationPolicy(code?: string) {
  return useQuery({
    queryKey: mutationPolicyKeys.detail(code ?? ""),
    queryFn: () => getMutationPolicy(code!),
    enabled: Boolean(code),
    retry: shouldRetryQuery,
  });
}

function useMutationPolicyCommand<TVariables>(command: (variables: TVariables) => Promise<MutationPolicy>) {
  return useSharedPolicyCommand(command, mutationPolicyKeys.detail, mutationPolicyKeys.list);
}

export function useCreateMutationPolicy() { return useMutationPolicyCommand(createMutationPolicy); }
export function useActivateMutationPolicy() { return useMutationPolicyCommand(activateMutationPolicy); }
export function useDeprecateMutationPolicy() { return useMutationPolicyCommand(deprecateMutationPolicy); }

export function useReplaceMutationPolicy() {
  return useMutationPolicyCommand(({ code, draft }: { code: string; draft: MutationPolicyDraft }) => replaceMutationPolicy(code, draft));
}

export function useUpdateMutationPolicyMetadata() {
  return useMutationPolicyCommand(({ code, metadata }: { code: string; metadata: MutationPolicyMetadata }) => updateMutationPolicyMetadata(code, metadata));
}

export function useDeleteMutationPolicy() {
  return useDeletePolicyCommand(deleteMutationPolicy, mutationPolicyKeys.detail, mutationPolicyKeys.list);
}
