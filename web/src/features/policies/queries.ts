import { useMutation, useQueryClient } from "@tanstack/react-query";

type CatalogPolicy = { code: string };
type PolicyKey = readonly unknown[];

export function usePolicyCommand<TPolicy extends CatalogPolicy, TVariables>(
  command: (variables: TVariables) => Promise<TPolicy>,
  detailKey: (code: string) => PolicyKey,
  listKey: PolicyKey,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: command,
    retry: false,
    onSuccess(policy) {
      client.setQueryData(detailKey(policy.code), policy);
      void client.invalidateQueries({ queryKey: listKey });
    },
  });
}

export function useDeletePolicyCommand(
  command: (code: string) => Promise<void>,
  detailKey: (code: string) => PolicyKey,
  listKey: PolicyKey,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: command,
    retry: false,
    onSuccess(_, code) {
      client.removeQueries({ queryKey: detailKey(code) });
      void client.invalidateQueries({ queryKey: listKey });
    },
  });
}
