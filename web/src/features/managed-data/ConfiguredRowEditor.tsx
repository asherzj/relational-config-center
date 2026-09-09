import { useEffect, useState } from "react";
import { getFieldPolicies, type FieldPolicies } from "../../api/field-policies";
import { Drawer } from "../../components/ui/Drawer";
import { ErrorState, LoadingState } from "../../components/ui/Feedback";
import { ManagedRowEditor, type ManagedRowEditorProps } from "./ManagedRowEditor";

// Each opening reads current configuration once. Background refresh/session
// recovery must not replace an already-open form's interaction rules or input.
export function ConfiguredRowEditor(props: ManagedRowEditorProps) {
  const [configuration, setConfiguration] = useState<FieldPolicies>();
  const [error, setError] = useState<unknown>();
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    void getFieldPolicies(props.tableName).then(value => { if (active) setConfiguration(value); }, failure => { if (active) setError(failure); });
    return () => { active = false; };
  }, [props.tableName, attempt]);
  if (configuration) return <ManagedRowEditor {...props} fieldPolicies={configuration.fields} />;
  return <Drawer eyebrow="字段录入配置" open={props.open} title={`${props.operation === "ADD" ? "新增" : "修改"} ${props.tableName} 记录`} onClose={props.onClose}>
    {error ? <ErrorState error={error} onRetry={() => { setError(undefined); setAttempt(value => value + 1); }} /> : <LoadingState label="正在读取字段录入配置…" />}
  </Drawer>;
}
