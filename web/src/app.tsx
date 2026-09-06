import { AccountPage } from "./features/accounts/AccountPage";
import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components/AppShell";
import { QueryPoliciesPage } from "./features/query-policies/QueryPoliciesPage";
import { MutationPoliciesPage } from "./features/mutation-policies/MutationPoliciesPage";
import { TablePoliciesPage } from "./features/table-policies/TablePoliciesPage";
import { ManagedDataPage } from "./features/managed-data/ManagedDataPage";

export function AppRoutes() {
  return (
    <Routes>
      <Route path="login" element={<AccountPage key="login" mode="login" />} />
      <Route path="register" element={<AccountPage key="register" mode="register" />} />
      <Route path="account" element={<AccountPage key="account" mode="account" />} />
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/platform/query-policies" replace />} />
        <Route path="platform/query-policies" element={<QueryPoliciesPage />} />
        <Route path="platform/query-policies/:code" element={<QueryPoliciesPage />} />
        <Route path="platform/mutation-policies" element={<MutationPoliciesPage />} />
        <Route path="platform/mutation-policies/:code" element={<MutationPoliciesPage />} />
        <Route path="platform/table-policies" element={<TablePoliciesPage />} />
        <Route path="platform/table-policies/:tableName" element={<TablePoliciesPage />} />
        <Route path="configuration/managed-data" element={<ManagedDataPage />} />
        <Route path="*" element={<Navigate to="/platform/query-policies" replace />} />
      </Route>
    </Routes>
  );
}
