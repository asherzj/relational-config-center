import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../api/client";
import { addManagedRow, deleteManagedRow, modifyManagedRow, queryManagedTable } from "../../api/managed-data";
import type { ChangeSetOperation, ManagedDataColumn, MutationContent, QuerySpec } from "./model";

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

export type ManagedDataMutationCommand = {
  operation: ChangeSetOperation;
  tableName: string;
  id?: string;
  content: MutationContent;
};

export type ManagedDataMutationOutcome = {
  operation: ChangeSetOperation;
  tableName: string;
  id: string;
  row?: Record<string, string | null>;
  columns?: ManagedDataColumn[];
  retrievalError?: unknown;
};

async function readManagedRow(tableName: string, id: string) {
  const result = await queryManagedTable(tableName, {
    conditions: [{ field: "id", operator: "exact", value: id }],
    pageNumber: 1,
    pageSize: 1,
  });
  const row = result.rows.find((candidate) => candidate.id === id);
  if (!row) throw new ApiError("contract_mismatch", "Admin did not return the mutated row.", 200);
  return { row, columns: result.columns };
}

async function executeMutation(command: ManagedDataMutationCommand): Promise<ManagedDataMutationOutcome> {
  let id = command.id;
  if (command.operation === "ADD") {
    id = (await addManagedRow(command.tableName, command.content)).id;
  } else {
    if (id === undefined) throw new ApiError("invalid_request", "Managed Table row id is required.", 400);
    if (command.operation === "MODIFY") await modifyManagedRow(command.tableName, id, command.content);
    else await deleteManagedRow(command.tableName, id);
  }

  if (command.operation === "DELETE") return { operation: command.operation, tableName: command.tableName, id: id! };
  try {
    return { operation: command.operation, tableName: command.tableName, id: id!, ...await readManagedRow(command.tableName, id!) };
  } catch (retrievalError) {
    return { operation: command.operation, tableName: command.tableName, id: id!, retrievalError };
  }
}

export function useManagedDataRowRefetch() {
  return useMutation({
    mutationFn: async ({ operation, tableName, id }: { operation: "ADD" | "MODIFY"; tableName: string; id: string }): Promise<ManagedDataMutationOutcome> => ({
      operation,
      tableName,
      id,
      ...await readManagedRow(tableName, id),
    }),
  });
}

export function useManagedDataMutation() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: executeMutation,
    onSuccess() {
      void client.invalidateQueries({ queryKey: managedDataKeys.root });
    },
  });
}
