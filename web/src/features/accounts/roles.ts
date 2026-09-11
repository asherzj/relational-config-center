import type { AccountRole } from "../../api/account-roles";
import { useWorkspaceIdentity } from "./ProtectedWorkspace";

export const accountRolesChanged = "rcc:account-roles-changed";
export const roleLabels: Record<AccountRole,string> = {
 VIEWER:"查看者", EDITOR:"编辑者", PUBLISHER:"发布者", ADMIN:"管理员",
};
export function useAccountRole(role: AccountRole) {
 const roles=useWorkspaceIdentity()?.account.roles ?? [];
 return roles.includes("ADMIN") || roles.includes(role) || (role === "VIEWER" && roles.length > 0);
}
