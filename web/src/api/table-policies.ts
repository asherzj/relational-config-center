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
    tableName: dto.table_name,
    queryPolicyCode: dto.query_policy_code,
    mutationPolicyCode: dto.mutation_policy_code,
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

export async function createTablePolicy(assignment: TablePolicyAssignment): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(root, { method: "POST", body: JSON.stringify(assignmentToDto(assignment)), schema: tablePolicyDtoSchema }));
}

export async function replaceTablePolicy(tableName: string, assignment: TablePolicyAssignment): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(`${root}/${encodeURIComponent(tableName)}`, { method: "PUT", body: JSON.stringify(assignmentToDto(assignment)), schema: tablePolicyDtoSchema }));
}

export async function enableTablePolicy(tableName: string): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(`${root}/${encodeURIComponent(tableName)}/enable`, { method: "POST", schema: tablePolicyDtoSchema }));
}

export async function disableTablePolicy(tableName: string): Promise<TablePolicy> {
  return tablePolicyFromDto(await request(`${root}/${encodeURIComponent(tableName)}/disable`, { method: "POST", schema: tablePolicyDtoSchema }));
}
