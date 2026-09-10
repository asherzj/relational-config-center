import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { shouldRetryQuery } from "../../api/client";
import {
  createTablePolicy,
  disableTablePolicy,
  enableTablePolicy,
  getTablePolicy,
  listDatabaseTables,
  listTablePolicies,
  replaceTablePolicy,
} from "../../api/table-policies";
import type { TablePolicy } from "./model";

export const tablePolicyKeys = {
  list: ["table-policies", "list"] as const,
  discovery: ["database-tables"] as const,
  detail: (tableName: string) => ["table-policies", "detail", tableName] as const,
};

export function useDatabaseTables(enabled = true) {
  return useQuery({ queryKey: tablePolicyKeys.discovery, queryFn: listDatabaseTables, retry: shouldRetryQuery, enabled });
}

export function useTablePolicies() {
  return useQuery({ queryKey: tablePolicyKeys.list, queryFn: listTablePolicies, retry: shouldRetryQuery });
}

export function useTablePolicy(tableName?: string) {
  return useQuery({
    queryKey: tablePolicyKeys.detail(tableName ?? ""),
    queryFn: () => getTablePolicy(tableName!),
    enabled: Boolean(tableName),
    retry: shouldRetryQuery,
  });
}

function useTablePolicyCommand<TVariables>(command: (variables: TVariables) => Promise<TablePolicy>) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: command,
    onSuccess(policy) {
      void client.invalidateQueries({ queryKey: tablePolicyKeys.detail(policy.tableName) });
      void client.invalidateQueries({ queryKey: tablePolicyKeys.list });
      void client.invalidateQueries({ queryKey: tablePolicyKeys.discovery });
    },
  });
}

export function useCreateTablePolicy() { return useTablePolicyCommand(createTablePolicy); }
export function useReplaceTablePolicy() {
  return useTablePolicyCommand(replaceTablePolicy);
}
export function useEnableTablePolicy() { return useTablePolicyCommand(enableTablePolicy); }
export function useDisableTablePolicy() { return useTablePolicyCommand(disableTablePolicy); }
