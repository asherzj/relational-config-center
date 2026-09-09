import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getFieldPolicies, saveFieldPolicies, uiTypes, operators, type FieldPolicy, type FieldPolicies } from "../../api/field-policies";
import { ApiError, isUncertainWriteError } from "../../api/client";
import { useAccountRole } from "../accounts/roles";
import { Button } from "../../components/ui/Button";
import { Drawer } from "../../components/ui/Drawer";
import { useDraftProtection } from "../../components/ui/LeaveProtection";
import { LoadingState, ErrorState } from "../../components/ui/Feedback";
import { useToast } from "../../components/ui/Toast";
import { Input } from "../../components/shadcn/input";
import { Label } from "../../components/shadcn/label";
import { Textarea } from "../../components/shadcn/textarea";
import { NativeSelect } from "../../components/shadcn/native-select";
import { Checkbox } from "../../components/shadcn/checkbox";

const uiLabels: Record<string, string> = { text: "文本", textarea: "多行文本", number: "数字", boolean: "布尔", date: "日期", datetime: "日期时间", select: "下拉（允许自定义值）", radio: "单选组" };
const opLabels: Record<string, string> = { exact: "等于", contains: "包含", open_range: "开区间", closed_range: "闭区间", in: "属于集合", not_in: "不属于集合", is_null: "为空值", is_not_null: "不为空值" };
const stateLabels = { missing: "未配置", active: "已启用", disabled: "已停用", incompatible: "与当前结构不兼容" };
const policiesOf = (data: FieldPolicies) => data.fields.flatMap(f => f.policy ? [f.policy] : []);
function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <Label className="field-policy-toggle"><Checkbox checked={checked} onCheckedChange={value => onChange(value === true)} /><span>{label}</span></Label>;
}

export function FieldPolicyDrawer({ tableName }: { tableName: string }) {
  const navigate = useNavigate();
  const canManage = useAccountRole("ADMIN");
  const { showToast } = useToast();
  const detail = useQuery({ queryKey: ["field-policies", tableName], queryFn: () => getFieldPolicies(tableName), refetchOnWindowFocus: false, retry: false });
  const [snapshot, setSnapshot] = useState<FieldPolicies | null>(null);
  const [policies, setPolicies] = useState<FieldPolicy[]>([]);
  const [baseline, setBaseline] = useState<FieldPolicy[]>([]);
  const [selected, setSelected] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);
  const [checking, setChecking] = useState(false);
  const [checked, setChecked] = useState<FieldPolicies | null>(null);
  const [checkError, setCheckError] = useState<unknown>(null);
  const inFlight = useRef(false);
 const errorRef=useRef<HTMLDivElement>(null);
 useEffect(()=>{if(error!=null){errorRef.current?.focus();errorRef.current?.scrollIntoView({block:"nearest"});}},[error]);
  const uncertain = isUncertainWriteError(error);
  const dirty = JSON.stringify(policies) !== JSON.stringify(baseline);
  const protection = useDraftProtection(dirty || uncertain, pending, inFlight);
  useEffect(() => {
    if (!snapshot && detail.data && !detail.isFetching && !detail.isError) {
      setSnapshot(detail.data); setPolicies(policiesOf(detail.data)); setBaseline(policiesOf(detail.data)); setSelected(detail.data.fields[0]?.field_name ?? "");
    }
  }, [detail.data, detail.isFetching, detail.isError, snapshot]);
  const field = snapshot?.fields.find(f => f.field_name === selected);
  const policy = policies.find(p => p.field_name === selected);
  const update = (change: Partial<FieldPolicy>) => setPolicies(current => current.map(p => p.field_name === selected ? { ...p, ...change } : p));
  const close = () => navigate("/platform/table-policies");
  const save = async () => {
    if (!canManage || inFlight.current || uncertain || !dirty) return;
    inFlight.current = true; setPending(true); setError(null); setChecked(null);
    try {
      const result = await saveFieldPolicies(tableName, policies);
      setSnapshot(result); setPolicies(policiesOf(result)); setBaseline(policiesOf(result)); showToast("字段配置已保存");
    } catch (cause) { setError(cause); if(cause instanceof ApiError && cause.fieldName && snapshot?.fields.some(f=>f.field_name===cause.fieldName))setSelected(cause.fieldName); }
    finally { inFlight.current = false; setPending(false); protection.submissionSettled(); }
  };
  const check = async () => {
    if (checking) return; setChecking(true); setCheckError(null);
    try { setChecked(await getFieldPolicies(tableName)); } catch (cause) { setCheckError(cause); } finally { setChecking(false); }
  };
  return <Drawer open title="字段配置" eyebrow={tableName} onClose={close} footer={<><Button variant="primary" disabled={!canManage || !dirty || pending || uncertain || !snapshot} onClick={() => void save()}>{pending ? "正在保存…" : "保存全部字段配置"}</Button><Button onClick={close} disabled={pending}>关闭</Button></>}>
    {!snapshot ? detail.isError ? <ErrorState error={detail.error} onRetry={() => void detail.refetch()} /> : <LoadingState label="正在读取真实字段与配置…" /> : <div className="policy-form">
      <p className="field-policy-metadata">真实业务表：<code>{tableName}</code></p>
      <p className="form-note">按真实字段维护交互规则。一次保存全部配置，停用保留原值。配置内容页面的查询、录入与显示将在后续版本接入这些规则。</p>
      {!canManage && <p role="status">当前账号没有管理员权限，字段配置只读。</p>}
      <Label className="field"><span>真实字段</span><NativeSelect aria-label="真实字段" value={selected} onChange={event => setSelected(event.target.value)}>{snapshot.fields.map(f => <option key={f.field_name} value={f.field_name}>{f.field_name} · {policies.some(p => p.field_name === f.field_name) ? policies.find(p => p.field_name === f.field_name)!.enabled ? "已启用" : "已停用" : "未配置"}</option>)}</NativeSelect></Label>
      {error != null && !uncertain && <div ref={errorRef} tabIndex={-1}><p>请检查字段：<code>{error instanceof ApiError ? error.fieldName ?? selected : selected}</code></p><ErrorState error={error} /></div>}
      {field && <><p className="field-policy-metadata"><code>{field.field_name}</code> · {field.column_type} · {field.nullable ? "允许 NULL" : "不允许 NULL"}{field.generated ? " · 数据库计算字段" : ""}{field.has_default ? " · 有数据库默认值" : ""}</p>
      {field.state === "incompatible" && <p role="alert" className="inline-alert">原配置与当前结构不兼容，使用时回退文本。请调整后保存。{field.warning}</p>}
      {!policy ? <div className="feedback-state"><strong>此字段尚未配置</strong><span>默认使用文本和等于查询。</span><Button disabled={!canManage || pending || uncertain} onClick={() => setPolicies(current => [...current, { ...field.effective, enabled: true }])}>配置此字段</Button></div> : <fieldset className="form-controls policy-form" disabled={!canManage || pending || uncertain}>
        <Toggle label="启用此字段规则" checked={policy.enabled} onChange={enabled => update({ enabled })} />
        <section className="field-policy-section"><h3>显示</h3>
          <Label className="field"><span>显示名称</span><Input maxLength={200} value={policy.display_name} onChange={e => update({ display_name: e.target.value })} placeholder={field.field_name} /></Label>
          <Label className="field"><span>字段说明</span><Textarea aria-label="字段说明" maxLength={500} value={policy.description} onChange={e => update({ description: e.target.value })} /></Label>
          <Label className="field"><span>显示顺序</span><Input type="number" min={0} max={4294967295} step={1} value={policy.display_order} onChange={e => update({ display_order: Number(e.target.value) })} /></Label>
          <Toggle label="在数据列表中显示" checked={policy.is_visible} onChange={is_visible => update({ is_visible })} />
        </section>
        <section className="field-policy-section"><h3>查询</h3>
          <Toggle label="允许在查询表单中筛选" checked={policy.is_queryable} onChange={is_queryable => update({ is_queryable })} />
          <div className="field"><span>查询运算符{policy.is_queryable ? "（至少选择一项）" : ""}</span><div className="field-policy-checks">{policy.query_operators.filter(op=>!operators.includes(op as typeof operators[number])).map(op=><Toggle key={op} label={`未知运算符：${op}（取消以移除）`} checked onChange={()=>update({query_operators:policy.query_operators.filter(value=>value!==op)})}/>)}{operators.map(op => <Toggle key={op} label={opLabels[op]} checked={policy.query_operators.includes(op)} onChange={on => update({ query_operators: on ? [...policy.query_operators, op] : policy.query_operators.filter(value => value !== op) })} />)}</div></div>
        </section>
        <section className="field-policy-section"><h3>录入</h3>
          <Label className="field"><span>录入控件</span><NativeSelect aria-label="录入控件" value={policy.ui_type} onChange={e => update({ ui_type: e.target.value as FieldPolicy["ui_type"], ui_options: e.target.value === "select" || e.target.value === "radio" ? { options: policy.ui_options.options } : { options: [] } })}>{!uiTypes.includes(policy.ui_type as typeof uiTypes[number]) && <option value={policy.ui_type}>未知控件：{policy.ui_type}（请重新选择）</option>}{uiTypes.map(type => <option key={type} value={type}>{uiLabels[type]}</option>)}</NativeSelect></Label>
          {(policy.ui_type === "select" || policy.ui_type === "radio") && <div className="field-policy-options"><h4>静态选项</h4><p>名称用于显示，实际值按真实字段类型保存。{policy.ui_type === "select" ? "用户仍可输入目录之外的合法值。" : "单选组不提供自由输入。"}</p>{policy.ui_options.options.map((option, index) => <div className="field-policy-option" key={index}><Label className="field"><span>选项名称 {index + 1}</span><Input maxLength={100} value={option.label} onChange={e => update({ ui_options: { options: policy.ui_options.options.map((item, i) => i === index ? { ...item, label: e.target.value } : item) } })} /></Label><Label className="field"><span>实际值 {index + 1}</span><Input value={option.value} onChange={e => update({ ui_options: { options: policy.ui_options.options.map((item, i) => i === index ? { ...item, value: e.target.value } : item) } })} /></Label><Button variant="ghost" aria-label={`删除选项 ${index + 1}`} onClick={() => update({ ui_options: { options: policy.ui_options.options.filter((_, i) => i !== index) } })}>删除</Button></div>)}<Button disabled={policy.ui_options.options.length >= 1000} onClick={() => update({ ui_options: { options: [...policy.ui_options.options, { label: "", value: "" }] } })}>添加选项</Button></div>}
          {policy.ui_type === "number" && <div className="field-policy-options">{([["min", "最小值"], ["max", "最大值"], ["step", "步长"]] as const).map(([key, label]) => <Label className="field" key={key}><span>{label}（可选）</span><Input inputMode="decimal" value={policy.ui_options[key] ?? ""} onChange={e => update({ ui_options: { ...policy.ui_options, [key]: e.target.value || undefined } })} /></Label>)}</div>}
          <Toggle label="新增时可编辑" checked={policy.editable_on_add} onChange={editable_on_add => update({ editable_on_add })} />
          <Toggle label="修改时可编辑" checked={policy.editable_on_modify} onChange={editable_on_modify => update({ editable_on_modify })} />
          <Toggle label="录入时必填" checked={policy.is_required} onChange={is_required => update({ is_required })} />
          <Label className="field"><span>新增预填方式</span><NativeSelect aria-label="新增预填方式" value={policy.default_value === undefined ? "none" : policy.default_value === null ? "null" : "value"} onChange={e => update({ default_value: e.target.value === "none" ? undefined : e.target.value === "null" ? null : "" })}><option value="none">未配置（保留数据库默认值）</option><option value="value">指定值（可以是空字符串）</option><option value="null">显式 NULL</option></NativeSelect></Label>
          {typeof policy.default_value === "string" && <Label className="field"><span>新增预填值</span><Input value={policy.default_value} onChange={e => update({ default_value: e.target.value })} /></Label>}
          <p className="field-policy-metadata">必填、可编辑仅引导表单；主键、自动填写及数据库限制继续生效。</p>
        </section>
      </fieldset>}</>}
      {uncertain && <section className="write-recovery" role="alert"><div ref={errorRef} tabIndex={-1} /><strong>提交结果尚未确认</strong><p>输入已保留。请先重新读取服务器配置，再决定是否再次保存。</p><ErrorState error={error} /><Button disabled={checking} onClick={() => void check()}>{checking ? "正在核对…" : "只读核对当前配置"}</Button>{checkError != null && <ErrorState error={checkError} />}{checked && <><strong>服务器当前配置</strong><div className="field-policy-server">{checked.fields.map(f => <details key={f.field_name}><summary>{f.field_name} · {f.policy?.display_name || f.field_name} · {stateLabels[f.state]}</summary>{f.warning && <p className="inline-alert">{f.warning}</p>}<FieldPolicySummary policy={f.policy} /></details>)}</div><p>核对不会覆盖本地输入；完整规则再次保存会替换当前服务器配置。</p><Button onClick={() => { setError(null); setChecked(null); }}>已核对，保留输入并返回编辑</Button></>}</section>}
    </div>}
  </Drawer>;
}

function FieldPolicySummary({ policy: p }: { policy: FieldPolicy | null }) {
 if (!p) return <p>未配置，使用默认文本与等于查询。</p>;
 const flag = (value: boolean) => value ? "是" : "否";
 return <dl className="field-policy-summary">
  <div><dt>说明</dt><dd>{p.description || "无"}</dd></div><div><dt>显示顺序 / 列表显示</dt><dd>{p.display_order} / {flag(p.is_visible)}</dd></div>
  <div><dt>可查询 / 运算符</dt><dd>{flag(p.is_queryable)} / {p.query_operators.map(op => opLabels[op] ?? `未知运算符：${op}`).join("、") || "无"}</dd></div>
  <div><dt>控件</dt><dd>{uiLabels[p.ui_type] ?? `未知控件：${p.ui_type}`}</dd></div><div><dt>新增可编辑 / 修改可编辑 / 必填</dt><dd>{flag(p.editable_on_add)} / {flag(p.editable_on_modify)} / {flag(p.is_required)}</dd></div>
  <div><dt>新增预填</dt><dd>{p.default_value === undefined ? "未配置" : p.default_value === null ? "NULL" : p.default_value === "" ? "空字符串" : p.default_value}</dd></div>
  <div><dt>静态选项（名称 → 实际值）</dt><dd>{p.ui_options.options.length ? p.ui_options.options.map((o,i) => <div key={i}>{o.label} → <code>{o.value === "" ? "空字符串" : o.value}</code></div>) : "无"}</dd></div>
  <div><dt>最小值 / 最大值 / 步长</dt><dd>{p.ui_options.min ?? "未配置"} / {p.ui_options.max ?? "未配置"} / {p.ui_options.step ?? "未配置"}</dd></div>
 </dl>;
}
