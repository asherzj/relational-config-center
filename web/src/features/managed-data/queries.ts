import { useQuery } from "@tanstack/react-query";
import { queryManagedTable } from "../../api/managed-data";
import type { QuerySpec } from "./model";

export const managedDataKeys = {
  query: (tableName: string, querySpec: QuerySpec) => ["managed-data", tableName, querySpec] as const,
};

export function useManagedDataQuery(tableName: string, querySpec: QuerySpec) {
  return useQuery({
    queryKey: managedDataKeys.query(tableName, querySpec),
    queryFn: () => queryManagedTable(tableName, querySpec),
    enabled: Boolean(tableName),
    retry: false,
  });
}
