import {
  mutationPolicyDtoSchema,
  mutationPolicyListDtoSchema,
  mutationPolicyTypeListDtoSchema,
  type MutationPolicyDto,
  type PutMutationPolicyDto,
} from "./contracts";
import { request } from "./client";
import type {
  MutationPolicy,
  MutationPolicyDraft,
  MutationPolicyMetadata,
  MutationPolicyType,
} from "../features/mutation-policies/model";

const root = "/api/v1/mutation-policies";

function fromDto(dto: MutationPolicyDto): MutationPolicy {
  return {
    code: dto.code,
    name: dto.name,
    description: dto.description,
    typeCode: dto.type_code,
    allowAdd: dto.allow_add,
    allowModify: dto.allow_modify,
    allowDelete: dto.allow_delete,
    createOperatorField: dto.create_operator_field,
    createTimeField: dto.create_time_field,
    modifyOperatorField: dto.modify_operator_field,
    modifyTimeField: dto.modify_time_field,
    status: dto.status,
    creator: dto.creator,
    modifier: dto.modifier,
    createdAt: dto.created_at,
    modifiedAt: dto.updated_at,
  };
}

function toPutDto(policy: MutationPolicyDraft): PutMutationPolicyDto {
  return {
    code: policy.code,
    name: policy.name,
    description: policy.description,
    type_code: policy.typeCode,
    allow_add: policy.allowAdd,
    allow_modify: policy.allowModify,
    allow_delete: policy.allowDelete,
    create_operator_field: policy.createOperatorField,
    create_time_field: policy.createTimeField,
    modify_operator_field: policy.modifyOperatorField,
    modify_time_field: policy.modifyTimeField,
  };
}

export async function listMutationPolicies(): Promise<MutationPolicy[]> {
  const response = await request(root, { schema: mutationPolicyListDtoSchema });
  return response.policies.map(fromDto);
}

export async function listMutationPolicyTypes(): Promise<MutationPolicyType[]> {
  const response = await request("/api/v1/mutation-policy-types", { schema: mutationPolicyTypeListDtoSchema });
  return response.types.map((type) => ({ code: type.code, operations: [...type.operations] }));
}

export async function getMutationPolicy(code: string): Promise<MutationPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}`, { schema: mutationPolicyDtoSchema }));
}

export async function createMutationPolicy(draft: MutationPolicyDraft): Promise<MutationPolicy> {
  return fromDto(await request(root, { method: "POST", body: JSON.stringify(toPutDto(draft)), schema: mutationPolicyDtoSchema }));
}

export async function replaceMutationPolicy(code: string, draft: MutationPolicyDraft): Promise<MutationPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}`, { method: "PUT", body: JSON.stringify(toPutDto(draft)), schema: mutationPolicyDtoSchema }));
}

export async function activateMutationPolicy(code: string): Promise<MutationPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}/activate`, { method: "POST", schema: mutationPolicyDtoSchema }));
}

export async function deprecateMutationPolicy(code: string): Promise<MutationPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}/deprecate`, { method: "POST", schema: mutationPolicyDtoSchema }));
}

export async function updateMutationPolicyMetadata(code: string, metadata: MutationPolicyMetadata): Promise<MutationPolicy> {
  return fromDto(await request(`${root}/${encodeURIComponent(code)}/metadata`, { method: "PATCH", body: JSON.stringify(metadata), schema: mutationPolicyDtoSchema }));
}

export async function deleteMutationPolicy(code: string): Promise<void> {
  await request<void>(`${root}/${encodeURIComponent(code)}`, { method: "DELETE" });
}
