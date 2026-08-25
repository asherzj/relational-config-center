import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./components/AppShell";
import { QueryPoliciesPage } from "./features/query-policies/QueryPoliciesPage";

export function AppRoutes() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/platform/query-policies" replace />} />
        <Route path="platform/query-policies" element={<QueryPoliciesPage />} />
        <Route path="platform/query-policies/:code" element={<QueryPoliciesPage />} />
        <Route path="*" element={<Navigate to="/platform/query-policies" replace />} />
      </Route>
    </Routes>
  );
}
