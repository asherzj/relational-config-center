import { useId, useState } from "react";
import type { FieldPolicyField } from "../../api/field-policies";
import { Input } from "../../components/shadcn/input";
import { NativeSelect } from "../../components/shadcn/native-select";
import { Label } from "../../components/shadcn/label";
import { ManagedTextInput } from "./ManagedTextInput";
import type { ManagedDataColumn } from "./model";

type Props = {
  column: ManagedDataColumn;
  policy?: Pick<FieldPolicyField["effective"], "ui_type" | "ui_options">;
  label: string;
  value: string;
  disabled?: boolean;
  required?: boolean;
  errorId?: string;
  onChange: (value: string) => void;
};

// Preserve protocol strings and fractional seconds without JS Number or Date.
export function dateTimeParts(value: string) {
  const match = /^(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2}(?:\.\d+)?)Z?$/.exec(value);
  return match ? { date: match[1]!, time: match[2]! } : undefined;
}
export function dateTimeValue(date: string, time: string, type: ManagedDataColumn["type"]) {
  return date && time ? `${date}${type === "timestamp" ? "T" : " "}${time}${type === "timestamp" ? "Z" : ""}` : "";
}

function DateTimeInput({ column, value, onChange, label, disabled, required, errorId }: Props) {
  const parts = dateTimeParts(value);
  const [rawMode] = useState(Boolean(value && !parts));
  const [date, setDate] = useState(parts?.date ?? "");
  const [time, setTime] = useState(parts?.time ?? "00:00:00");
  // Noncanonical historical values stay visible and unchanged until edited.
  if (rawMode) return <Input aria-label={label} aria-required={required} aria-invalid={Boolean(errorId)} aria-describedby={errorId} disabled={disabled} value={value} onChange={event => onChange(event.target.value)} />;
  return <div className="grid min-w-0 gap-2 sm:grid-cols-2">
    <Input aria-label={`${label} 日期`} type="date" disabled={disabled} value={parts?.date ?? date} aria-required={required} aria-invalid={Boolean(errorId)} aria-describedby={errorId}
      onChange={event => { setDate(event.target.value); onChange(dateTimeValue(event.target.value, parts?.time ?? time, column.type)); }} />
    <Input aria-label={`${label} 时间`} disabled={disabled} value={parts?.time ?? time} placeholder="HH:mm:ss.ffffff" aria-required={required} aria-invalid={Boolean(errorId)} aria-describedby={errorId}
      onChange={event => { setTime(event.target.value); onChange(dateTimeValue(parts?.date ?? date, event.target.value, column.type)); }} />
    <span className="form-note sm:col-span-2">{column.type === "timestamp" ? "UTC 时间，保留小数秒" : "数据库日期时间，保留小数秒"}</span>
  </div>;
}

export function FieldValueInput(props: Props) {
  const { column, policy, label, value, disabled, required, errorId, onChange } = props;
  const type = policy?.ui_type ?? "text";
  const options = policy?.ui_options.options ?? [];
  const selectedIndex = options.findIndex(option => option.value === value);
  const [multiline, setMultiline] = useState(/[\r\n]/.test(value));
  const [pasteFocus, setPasteFocus] = useState(false);
  const [custom, setCustom] = useState(selectedIndex < 0);
  const radioName = useId();
  const a11y = { "aria-label": label, "aria-required": required, "aria-invalid": Boolean(errorId), "aria-describedby": errorId, disabled };
  if (type === "select") return <div className="grid min-w-0 gap-2">
    <NativeSelect {...a11y} value={custom || selectedIndex < 0 ? "custom" : String(selectedIndex)} onChange={event => {
      const next = event.target.value; setCustom(next === "custom");
      if (next !== "custom") onChange(options[Number(next)]!.value);
    }}>
      {options.map((option, index) => <option key={option.value} value={index}>{option.label}（{option.value || "空字符串"}）</option>)}
      <option value="custom">自定义值</option>
    </NativeSelect>
    {(custom || selectedIndex < 0) && <ManagedTextInput label={`${label} 自定义值`} disabled={disabled} required={required} errorId={errorId} rows={2} value={value} onChange={onChange} />}
  </div>;
  if (type === "radio") return <div role="radiogroup" {...a11y} className="grid gap-2">
    {selectedIndex < 0 && <p className="form-note">当前值：{value === "" ? "空字符串" : value}（不在选项中；选择新选项才会替换）</p>}
    {options.map(option => <Label key={option.value} className="flex min-w-0 items-center gap-2 break-all"><input className="size-4 shrink-0 accent-primary" type="radio" name={radioName} disabled={disabled} checked={value === option.value} onChange={() => onChange(option.value)} />{option.label}（{option.value || "空字符串"}）</Label>)}
  </div>;
  if (type === "boolean") return <NativeSelect {...a11y} value={value} onChange={event => onChange(event.target.value)}>
    <option value="">请选择</option><option value="0">否（0 / false）</option><option value="1">是（1 / true）</option>
    {value !== "" && value !== "0" && value !== "1" && <option value={value}>{value}（原值）</option>}
  </NativeSelect>;
  if (type === "datetime") return <DateTimeInput {...props} />;
  if (type === "textarea" || (type === "text" && multiline)) return <ManagedTextInput autoFocus={pasteFocus} label={label} disabled={disabled} required={required} errorId={errorId} value={value} onChange={onChange} />;
  return <div className="grid min-w-0 gap-2"><Input {...a11y} onPaste={event => {
      if (type !== "text") return;
      const pasted = event.clipboardData.getData("text/plain");
      if (!/[\r\n]/.test(pasted)) return;
      event.preventDefault();
      setMultiline(true); setPasteFocus(true);
      const { selectionStart, selectionEnd } = event.currentTarget;
      onChange(value.slice(0, selectionStart ?? 0) + pasted + value.slice(selectionEnd ?? value.length));
    }} type={type === "date" && (!value || /^\d{4}-\d{2}-\d{2}$/.test(value)) ? "date" : "text"}
    inputMode={type === "number" ? "decimal" : undefined} value={value} onChange={event => onChange(event.target.value)} />
    {type === "number" && <span className="form-note">{[policy?.ui_options.min !== undefined ? `最小值 ${policy.ui_options.min}` : "", policy?.ui_options.max !== undefined ? `最大值 ${policy.ui_options.max}` : "", policy?.ui_options.step !== undefined ? `步长 ${policy.ui_options.step}` : ""].filter(Boolean).join(" · ")}</span>}
  </div>;
}
