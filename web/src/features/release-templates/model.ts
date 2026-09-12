export type ReleaseType = "STANDARD" | "EMERGENCY";
export type ReleaseTemplateNodeType = "APPROVAL" | "PUBLICATION" | "COMPLETION";
export type ReleaseTemplateNode = { code: string; type: ReleaseTemplateNodeType; name: string; requiredRole: "TABLE_APPROVER" | "PUBLISHER" };
export type ReleaseTemplate = {
  code: string; name: string; description: string; type: ReleaseType;
  nodes: ReleaseTemplateNode[]; enabled: boolean; version: string;
  creator: string; modifier: string; createdAt: string; updatedAt: string;
};
export type ReleaseTemplateDraft = Pick<ReleaseTemplate, "code" | "name" | "description" | "type" | "nodes">;

export const releaseTypeLabels: Record<ReleaseType, string> = { STANDARD: "常规", EMERGENCY: "应急" };
export const nodeTypeLabels: Record<ReleaseTemplateNodeType, string> = { APPROVAL: "按表审批", PUBLICATION: "发布", COMPLETION: "完结" };
export const roleLabels = { TABLE_APPROVER: "表审批人员", PUBLISHER: "发布人员" } as const;

export function defaultNodes(type: ReleaseType): ReleaseTemplateNode[] {
  const publish: ReleaseTemplateNode = { code: "publication", type: "PUBLICATION", name: type === "EMERGENCY" ? "应急发布" : "发布", requiredRole: "PUBLISHER" };
  const complete: ReleaseTemplateNode = { code: "completion", type: "COMPLETION", name: "完结", requiredRole: "PUBLISHER" };
  return type === "STANDARD" ? [{ code: "approval", type: "APPROVAL", name: "按表审批", requiredRole: "TABLE_APPROVER" }, publish, complete] : [publish, complete];
}

export function validateReleaseTemplate(draft: ReleaseTemplateDraft) {
  const errors: Record<string, string> = {};
  if (!/^[a-z][a-z0-9_]*_v[1-9][0-9]*$/.test(draft.code)) errors.code = "使用小写字母、数字和下划线，并以 _v1 这类版本号结尾。";
  if (!draft.name.trim()) errors.name = "请输入模板名称。";
  else if ([...draft.name.trim()].length > 100) errors.name = "模板名称不能超过 100 个字符。";
  if ([...draft.description.trim()].length > 500) errors.description = "描述不能超过 500 个字符。";
  const expected = draft.type === "STANDARD" ? ["APPROVAL", "PUBLICATION", "COMPLETION"] : ["PUBLICATION", "COMPLETION"];
  if (draft.nodes.length !== expected.length || draft.nodes.some((node, index) => node.type !== expected[index])) errors.nodes = "节点顺序与发布类型不匹配。";
  const codes = new Set<string>();
  draft.nodes.forEach((node, index) => {
    if (!/^[a-z][a-z0-9_]{0,63}$/.test(node.code)) errors[`node-${index}-code`] = "节点编码须为小写字母、数字或下划线。";
    else if (codes.has(node.code)) errors[`node-${index}-code`] = "节点编码不能重复。";
    codes.add(node.code);
    if (!node.name.trim()) errors[`node-${index}-name`] = "请输入节点名称。";
  });
  return errors;
}
