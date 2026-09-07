import {
  managedDataAddResponseDtoSchema,
  managedDataMutationResponseDtoSchema,
  managedDataQueryResponseDtoSchema,
  type ManagedDataQueryResponseDto,
} from "./contracts";
import { request } from "./client";
import type { ManagedDataResult, MutationContent, QuerySpec } from "../features/managed-data/model";

function fromDto(dto: ManagedDataQueryResponseDto): ManagedDataResult {
  return {
    columns: dto.columns,
    rows: dto.rows,
    recordVersions: dto.record_versions,
    page: {
      pageNumber: dto.page.page_number,
      pageSize: dto.page.page_size,
      totalCount: dto.page.total_count,
      totalPages: dto.page.total_pages,
    },
  };
}

export async function queryManagedTable(tableName: string, querySpec: QuerySpec): Promise<ManagedDataResult> {
  const payload = {
    conditions: querySpec.conditions,
    ...(querySpec.order ? { order: querySpec.order } : {}),
    ...(querySpec.pageNumber !== undefined ? { page_number: querySpec.pageNumber } : {}),
    ...(querySpec.pageSize !== undefined ? { page_size: querySpec.pageSize } : {}),
  };
  return fromDto(await request(`/api/v1/tables/${encodeURIComponent(tableName)}/query`, {
    method: "POST",
    body: JSON.stringify(payload),
    schema: managedDataQueryResponseDtoSchema,
  }));
}

export function addManagedRow(tableName: string, content: MutationContent) {
  return request(`/api/v1/tables/${encodeURIComponent(tableName)}/rows`, {
    method: "POST",
    body: JSON.stringify({ content }),
    schema: managedDataAddResponseDtoSchema,
  });
}

export function modifyManagedRow(tableName: string, id: string, content: MutationContent, expectedVersion: string) {
  return request(`/api/v1/tables/${encodeURIComponent(tableName)}/rows/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify({ content, expected_version: expectedVersion }),
    schema: managedDataMutationResponseDtoSchema,
  });
}

export function deleteManagedRow(tableName: string, id: string, expectedVersion: string) {
  return request(`/api/v1/tables/${encodeURIComponent(tableName)}/rows/${encodeURIComponent(id)}`, {
    method: "DELETE",
    body: JSON.stringify({ expected_version: expectedVersion }),
    schema: managedDataMutationResponseDtoSchema,
  });
}
