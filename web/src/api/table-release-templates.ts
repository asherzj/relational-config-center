import {z} from "zod";
import {request} from "./client";

const associationSchema=z.object({table_name:z.string(),type:z.enum(["STANDARD","EMERGENCY"]),template_code:z.string(),template_name:z.string(),template_enabled:z.boolean(),enabled:z.boolean(),version:z.string(),creator:z.string(),modifier:z.string(),created_at:z.string(),updated_at:z.string()});
export type TableReleaseTemplate=z.infer<typeof associationSchema>;
export type AssociationWrite={table:string;type:TableReleaseTemplate["type"];templateCode:string;enabled:boolean;version:string;key:string};
const path=(table:string)=>`/api/v1/table-policies/${encodeURIComponent(table)}/release-templates`;
export async function listTableReleaseTemplates(table:string){return(await request(path(table),{schema:z.object({associations:z.array(associationSchema)})})).associations;}
export async function putTableReleaseTemplate(write:AssociationWrite){return request(`${path(write.table)}/${write.type}`,{method:"PUT",headers:{"Idempotency-Key":write.key},body:JSON.stringify({template_code:write.templateCode,enabled:write.enabled,expected_version:write.version}),schema:associationSchema});}
