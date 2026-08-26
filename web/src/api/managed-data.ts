import {
  managedDataQueryResponseDtoSchema,
  type ManagedDataQueryResponseDto,
} from "./contracts";
import { request } from "./client";
import type { ManagedDataResult, QuerySpec } from "../features/managed-data/model";

function fromDto(dto: ManagedDataQueryResponseDto): ManagedDataResult {
  return {
    columns: dto.columns,
    rows: dto.rows,
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
