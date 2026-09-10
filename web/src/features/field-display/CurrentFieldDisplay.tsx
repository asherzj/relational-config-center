import { useQuery } from "@tanstack/react-query";
import { createContext, useContext, type ReactNode } from "react";
import { getFieldPolicies, type FieldPolicies } from "../../api/field-policies";
import { ErrorState } from "../../components/ui/Feedback";
import { useWorkspaceIdentity } from "../accounts/ProtectedWorkspace";

export type CurrentFieldDisplay = {
  configuration?: FieldPolicies;
};

type CurrentFieldDisplayContextValue = { display: CurrentFieldDisplay; pending: boolean; error: unknown; retry: () => unknown };
const CurrentFieldDisplayContext = createContext<CurrentFieldDisplayContextValue>({ display: {}, pending: false, error: null, retry: () => undefined });

type FieldDisplayRule = {
  name: string;
  displayName: string;
  displayOrder: number;
  visible: boolean;
  valueLabel(value: string | null): string | undefined;
};

export function fieldDisplay(display: CurrentFieldDisplay, name: string): FieldDisplayRule {
  const effective = display.configuration?.fields.find(field => field.field_name === name)?.effective;
  return {
    name,
    displayName: effective?.display_name || name,
    displayOrder: effective?.display_order ?? 0,
    visible: effective?.is_visible ?? true,
    valueLabel: value => value === null ? undefined : effective?.ui_options.options.find(option => option.value === value)?.label,
  };
}

export function orderDisplayedFields<T>(display: CurrentFieldDisplay, fields: readonly T[], name: (field: T) => string, list = false): T[] {
  return fields
    .filter(field => !list || fieldDisplay(display, name(field)).visible)
    .map((field, index) => ({ field, index, rule: fieldDisplay(display, name(field)) }))
    .sort((left, right) => left.rule.displayOrder - right.rule.displayOrder
      || left.rule.name.localeCompare(right.rule.name)
      || left.index - right.index)
    .map(item => item.field);
}

export function CurrentFieldName({ display, name, detail }: { display: CurrentFieldDisplay; name: string; detail?: ReactNode }) {
  const rule = fieldDisplay(display, name);
  return <><strong>{rule.displayName}</strong>{(rule.displayName !== name || detail) && <small className="block break-all font-mono text-muted-foreground font-normal">{rule.displayName !== name ? name : null}{rule.displayName !== name && detail ? " · " : null}{detail}</small>}</>;
}

export function CurrentFieldValue({ display, name, value, children }: { display: CurrentFieldDisplay; name: string; value: string | null; children: ReactNode }) {
  const label = fieldDisplay(display, name).valueLabel(value);
  if (label === undefined || label === value) return <>{children}</>;
  return <span className="field-display-value"><span className="block break-all">{label}</span><small aria-label={`真实值：${value}`} className="block whitespace-pre-wrap break-all font-mono text-muted-foreground">真实值：{children}</small></span>;
}

export function useCurrentFieldDisplay(tableName: string, enabled = true): CurrentFieldDisplayContextValue {
  const accountID = useWorkspaceIdentity()?.account.id ?? "anonymous";
  const query = useQuery({
    queryKey: ["current-field-display", accountID, tableName],
    queryFn: () => getFieldPolicies(tableName),
    enabled: enabled && Boolean(tableName),
    retry: false,
    staleTime: 0,
    refetchOnMount: "always",
  });
  const fresh = query.isSuccess && !query.isFetching;
  return {
    display: { configuration: fresh ? query.data : undefined },
    pending: enabled && (query.isPending || query.isFetching),
    error: query.error,
    retry: query.refetch,
  };
}

export function CurrentFieldDisplayProvider({ tableName, children }: { tableName: string; children: ReactNode }) {
  return <CurrentFieldDisplayContext value={useCurrentFieldDisplay(tableName)}>{children}</CurrentFieldDisplayContext>;
}

export function useCurrentFieldDisplayContext() {
  return useContext(CurrentFieldDisplayContext);
}

export function CurrentFieldDisplayStatus({ pending, error, onRetry }: { pending: boolean; error: unknown; onRetry: () => unknown }) {
  if (error) return <section className="field-display-status"><p className="text-sm text-muted-foreground">字段显示配置读取失败，以下保留真实字段名与原始值。</p><ErrorState error={error} onRetry={onRetry} /></section>;
  if (pending) return <p role="status" className="field-display-status text-sm text-muted-foreground">正在读取当前字段显示配置；真实字段名与原始值仍可核对。</p>;
  return null;
}
