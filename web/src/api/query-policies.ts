import {
  queryPolicyDtoSchema,
  queryPolicyListDtoSchema,
  queryPolicyTypeListDtoSchema,
  type PutQueryPolicyDto,
  type QueryPolicyDto,
} from "./contracts";
import { request } from "./client";
import type { QueryPolicy, QueryPolicyDraft, QueryPolicyMetadata } from "../features/query-policies/model";

const root = "/api/v1/query-policies";

function fromDto(dto: QueryPolicyDto): QueryPolicy {
  return {
    code: dto.code,
    name: dto.name,
    description: dto.description,
    typeCode: dto.type_code,
    defaultOrderField: dto.default_order_field,
    defaultOrderDirection: dto.default_order_direction,
    defaultPageSize: dto.default_page_size,
    maxPageSize: dto.max_page_size,
    status: dto.status,
    creator: dto.creator,
    modifier: dto.modifier,
    createdAt: dto.gmt_created,
    modifiedAt: dto.gmt_modified,
  };
}

function toPutDto(policy: QueryPolicyDraft): PutQueryPolicyDto {
  return {
    code: policy.code,
    name: policy.name,
    description: policy.description,
    type_code: policy.typeCode,
    default_order_field: policy.defaultOrderField,
    default_order_direction: policy.defaultOrderDirection,
    default_page_size: policy.defaultPageSize,
    max_page_size: policy.maxPageSize,
  };
}

export async function listQueryPolicies(): Promise<QueryPolicy[]> {
  const response = await request(root, { schema: queryPolicyListDtoSchema });
  return response.policies.map(fromDto);
}

export async function listQueryPolicyTypes(): Promise<string[]> {
  const response = await request("/api/v1/query-policy-types", { schema: queryPolicyTypeListDtoSchema });
  return response.types.map(({ code }) => code);
}

export async function getQueryPolicy(code: string): Promise<QueryPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}`, { schema: queryPolicyDtoSchema }));
}

export async function createQueryPolicy(draft: QueryPolicyDraft): Promise<QueryPolicy> {
  return fromDto(await request(root, { method: "POST", body: JSON.stringify(toPutDto(draft)), schema: queryPolicyDtoSchema }));
}

export async function replaceQueryPolicy(code: string, draft: QueryPolicyDraft): Promise<QueryPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}`, { method: "PUT", body: JSON.stringify(toPutDto(draft)), schema: queryPolicyDtoSchema }));
}

export async function activateQueryPolicy(code: string): Promise<QueryPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}/activate`, { method: "POST", schema: queryPolicyDtoSchema }));
}

export async function deprecateQueryPolicy(code: string): Promise<QueryPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}/deprecate`, { method: "POST", schema: queryPolicyDtoSchema }));
}

export async function updateQueryPolicyMetadata(code: string, metadata: QueryPolicyMetadata): Promise<QueryPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}/metadata`, { method: "PATCH", body: JSON.stringify(metadata), schema: queryPolicyDtoSchema }));
}

export async function deleteQueryPolicy(code: string): Promise<void> {
  await request<void>(`${root}/${encodeURIComponent(code)}`, { method: "DELETE" });
}
