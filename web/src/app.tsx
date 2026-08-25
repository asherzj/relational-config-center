import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components/AppShell";
import { QueryPoliciesPage } from "./features/query-policies/QueryPoliciesPage";
import { MutationPoliciesPage } from "./features/mutation-policies/MutationPoliciesPage";
import { TablePoliciesPage } from "./features/table-policies/TablePoliciesPage";

export function AppRoutes() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/platform/query-policies" replace />} />
        <Route path="platform/query-policies" element={<QueryPoliciesPage />} />
        <Route path="platform/query-policies/:code" element={<QueryPoliciesPage />} />
        <Route path="platform/mutation-policies" element={<MutationPoliciesPage />} />
        <Route path="platform/mutation-policies/:code" element={<MutationPoliciesPage />} />
        <Route path="platform/table-policies" element={<TablePoliciesPage />} />
        <Route path="platform/table-policies/:tableName" element={<TablePoliciesPage />} />
        <Route path="*" element={<Navigate to="/platform/query-policies" replace />} />
      </Route>
    </Routes>
  );
}
