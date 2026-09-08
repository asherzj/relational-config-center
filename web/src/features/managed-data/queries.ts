import { useMutation, useQuery } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { queryManagedTable } from "../../api/managed-data";
import type { ChangeSetOperation, ManagedDataMutationOutcome, QuerySpec } from "./model";

export const managedDataKeys = {
  root: ["managed-data"] as const,
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

async function readManagedRow(tableName: string, id: string) {
  const result = await queryManagedTable(tableName, {
    conditions: [{ field: "id", operator: "exact", value: id }],
    pageNumber: 1,
    pageSize: 1,
  });
  const row = result.rows.length === 1 ? result.rows[0] : undefined;
  if (!row) throw new ApiError("contract_mismatch", "Admin did not return the mutated row.", 200);
  return { row, columns: result.columns, recordVersion: result.recordVersions[0]! };
}

export function useManagedDataRowRefetch() {
  return useMutation({
    mutationFn: async ({ operation, tableName, id }: { operation: ChangeSetOperation; tableName: string; id: string }): Promise<ManagedDataMutationOutcome> => ({
      operation,
      tableName,
      id,
      ...await readManagedRow(tableName, id),
    }),
  });
}
