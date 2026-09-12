import { releaseTemplateDtoSchema, releaseTemplateListDtoSchema, type ReleaseTemplateDto } from "./contracts";
import { request } from "./client";
import type { ReleaseTemplate, ReleaseTemplateDraft } from "../features/release-templates/model";

const root = "/api/v1/release-templates";
const fromDto = (dto: ReleaseTemplateDto): ReleaseTemplate => ({
  code: dto.code, name: dto.name, description: dto.description, type: dto.type,
  nodes: dto.node_list.map(node => ({ code: node.code, type: node.type, name: node.name, requiredRole: node.required_role })),
  enabled: dto.enabled, version: dto.version, creator: dto.creator, modifier: dto.modifier, createdAt: dto.created_at, updatedAt: dto.updated_at,
});
const body = (draft: ReleaseTemplateDraft) => ({ code: draft.code, name: draft.name, description: draft.description, type: draft.type, node_list: draft.nodes.map(node => ({ code: node.code, type: node.type, name: node.name, required_role: node.requiredRole })) });

export async function listReleaseTemplates() { return (await request(root, { schema: releaseTemplateListDtoSchema })).templates.map(fromDto); }
export async function getReleaseTemplate(code: string) { return fromDto(await request(`${root}/${encodeURIComponent(code)}`, { schema: releaseTemplateDtoSchema })); }
export async function createReleaseTemplate(draft: ReleaseTemplateDraft, key: string) { return fromDto(await request(root, { method: "POST", headers: { "Idempotency-Key": key }, body: JSON.stringify(body(draft)), schema: releaseTemplateDtoSchema })); }
export async function replaceReleaseTemplate(code: string, draft: ReleaseTemplateDraft, version: string, key: string) { return fromDto(await request(`${root}/${encodeURIComponent(code)}`, { method: "PUT", headers: { "Idempotency-Key": key }, body: JSON.stringify({ ...body(draft), expected_version: version }), schema: releaseTemplateDtoSchema })); }
export async function setReleaseTemplateEnabled(code: string, enabled: boolean, version: string, key: string) { return fromDto(await request(`${root}/${encodeURIComponent(code)}/${enabled ? "enable" : "disable"}`, { method: "POST", headers: { "Idempotency-Key": key }, body: JSON.stringify({ expected_version: version }), schema: releaseTemplateDtoSchema })); }
export async function deleteReleaseTemplate(code: string, version: string, key: string) { await request(`${root}/${encodeURIComponent(code)}`, { method: "DELETE", headers: { "Idempotency-Key": key }, body: JSON.stringify({ expected_version: version }) }); }
