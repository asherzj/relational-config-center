import { ApprovalRolesPage } from "./features/approval-roles/ApprovalRolesPage";
import {ReleaseOrdersPage} from "./features/release-orders/ReleaseOrdersPage";
import { AccountRolesPage } from "./features/account-roles/AccountRolesPage";
import { safeReturnDestination } from "./features/accounts/returnDestination";
import { ProtectedWorkspace } from "./features/accounts/ProtectedWorkspace";
import { AccountPage } from "./features/accounts/AccountPage";
import { Navigate, Route, Routes, useNavigate, useSearchParams } from "react-router-dom";
import { AppShell } from "./components/AppShell";
import { QueryPoliciesPage } from "./features/query-policies/QueryPoliciesPage";
import { MutationPoliciesPage } from "./features/mutation-policies/MutationPoliciesPage";
import { TablePoliciesPage } from "./features/table-policies/TablePoliciesPage";
import { ManagedDataPage } from "./features/managed-data/ManagedDataPage";

export function AppRoutes() {
  return (
    <Routes>
      <Route path="login" element={<AccountEntry key="login" mode="login" />} />
      <Route path="register" element={<AccountEntry key="register" mode="register" />} />
      <Route path="account" element={<AccountPage key="account" mode="account" />} />
      <Route element={<ProtectedWorkspace />}>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/platform/query-policies" replace />} />
        <Route path="platform/approval-roles" element={<ApprovalRolesPage />} />
        <Route path="platform/account-roles" element={<AccountRolesPage />} />
        <Route path="platform/query-policies" element={<QueryPoliciesPage />} />
        <Route path="platform/query-policies/:code" element={<QueryPoliciesPage />} />
        <Route path="platform/mutation-policies" element={<MutationPoliciesPage />} />
        <Route path="platform/mutation-policies/:code" element={<MutationPoliciesPage />} />
        <Route path="platform/table-policies" element={<TablePoliciesPage />} />
        <Route path="platform/table-policies/:tableName" element={<TablePoliciesPage />} />
        <Route path="configuration/release-orders" element={<ReleaseOrdersPage />} />
 <Route path="configuration/release-orders/:id" element={<ReleaseOrdersPage />} />
 <Route path="configuration/managed-data" element={<ManagedDataPage />} />
        <Route path="*" element={<Navigate to="/platform/query-policies" replace />} />
      </Route>
      </Route>
    </Routes>
  );
}

function AccountEntry({ mode }: { mode: "login" | "register" }) {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  return <AccountPage mode={mode} onSignedIn={() => navigate(safeReturnDestination(params.get("returnTo")), { replace: true })} />;
}
