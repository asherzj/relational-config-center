import {z} from "zod";
import {
  databaseTableListDtoSchema,
  tablePolicyDtoSchema,
  tablePolicyListDtoSchema,
  type DatabaseTableDto,
  type TablePolicyDto,
} from "./contracts";
import { request } from "./client";
import type { DatabaseTable, TablePolicy, TablePolicyAssignment } from "../features/table-policies/model";

const root = "/api/v1/table-policies";

function databaseTableFromDto(dto: DatabaseTableDto): DatabaseTable {
  return {
    tableName: dto.table_name,
    tableComment: dto.table_comment,
    policyExists: dto.policy_exists,
    policyEnabled: dto.policy_enabled,
    compatible: dto.compatible,
    incompatibilityReason: dto.incompatibility_reason,
  };
}

function tablePolicyFromDto(dto: TablePolicyDto): TablePolicy {
  return {
    version: dto.version,
    tableName: dto.table_name,
    queryPolicyCode: dto.query_policy_code,
    mutationPolicyCode: dto.mutation_policy_code,
    concurrencyKey: dto.concurrency_key,
    enabled: dto.enabled,
    creator: dto.creator,
    modifier: dto.modifier,
    createdAt: dto.created_at,
    modifiedAt: dto.updated_at,
  };
}

function assignmentToDto(assignment: TablePolicyAssignment) {
  return {
    table_name: assignment.tableName,
    query_policy_code: assignment.queryPolicyCode,
    mutation_policy_code: assignment.mutationPolicyCode,
    concurrency_key: assignment.concurrencyKey ?? [],
  };
}

export async function listDatabaseTables(): Promise<DatabaseTable[]> {
  const response = await request("/api/v1/database-tables", { schema: databaseTableListDtoSchema });
  return response.tables.map(databaseTableFromDto);
}

export async function listTablePolicies(): Promise<TablePolicy[]> {
  const response = await request(root, { schema: tablePolicyListDtoSchema });
  return response.policies.map(tablePolicyFromDto);
}

export async function getTablePolicy(tableName: string): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(`${root}/${encodeURIComponent(tableName)}`, { schema: tablePolicyDtoSchema }));
}

export async function createTablePolicy({assignment,key}: {assignment: TablePolicyAssignment;key:string}): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(root, { method: "POST", headers:{"Idempotency-Key":key}, body: JSON.stringify(assignmentToDto(assignment)), schema: tablePolicyDtoSchema }));
}

export async function replaceTablePolicy({tableName,assignment,version,key}: {tableName:string;assignment:TablePolicyAssignment;version:string;key:string}): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(`${root}/${encodeURIComponent(tableName)}`, { method: "PUT", headers:{"Idempotency-Key":key}, body: JSON.stringify({...assignmentToDto(assignment),expected_version:version}), schema: tablePolicyDtoSchema }));
}

export async function enableTablePolicy({tableName,version,key}: {tableName:string;version:string;key:string}): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(`${root}/${encodeURIComponent(tableName)}/enable`, { method: "POST", headers:{"Idempotency-Key":key},body:JSON.stringify({expected_version:version}), schema: tablePolicyDtoSchema }));
}

export async function disableTablePolicy({tableName,version,key}: {tableName:string;version:string;key:string}): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(`${root}/${encodeURIComponent(tableName)}/disable`, { method: "POST", headers:{"Idempotency-Key":key},body:JSON.stringify({expected_version:version}), schema: tablePolicyDtoSchema }));
}

export const concurrencyKeyFields=(table:string,mutation:string)=>request(`${root}/${encodeURIComponent(table)}/concurrency-key-fields?${new URLSearchParams({mutation_policy_code:mutation})}`,{schema:z.object({fields:z.array(z.object({name:z.string(),type:z.string(),eligible:z.boolean()}))})});
