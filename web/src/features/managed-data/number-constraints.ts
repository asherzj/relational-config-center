import type { FieldPolicyField } from "../../api/field-policies";

type Decimal = { coefficient: bigint; scale: number };
function decimal(value: string): Decimal | undefined {
  const match = /^([+-]?(?:\d+(?:\.\d*)?|\.\d+))(?:[eE]([+-]?\d+))?$/.exec(value);
  if (!match) return;
  const exponent = Number(match[2] ?? "0");
  // Only the bounded exponent is numeric; the actual value stays a BigInt.
  if (!Number.isSafeInteger(exponent) || Math.abs(exponent) > 1000) return;
  const mantissa = match[1]!;
  return { coefficient: BigInt(mantissa.replace(".", "")), scale: (mantissa.split(".")[1]?.length ?? 0) - exponent };
}
function aligned(left: Decimal, right: Decimal): [bigint, bigint] {
  const scale = Math.max(left.scale, right.scale);
  return [left.coefficient * 10n ** BigInt(scale - left.scale), right.coefficient * 10n ** BigInt(scale - right.scale)];
}
export function numberConstraintError(value: string, options: FieldPolicyField["effective"]["ui_options"]): string | undefined {
  if (value === "") return;
  const number = decimal(value);
  if (!number) return "请输入有效数字";
  for (const bound of ["min", "max"] as const) {
    const limit = options[bound] === undefined ? undefined : decimal(options[bound]);
    if (options[bound] !== undefined && !limit) return "字段数字约束无效，请重新打开表单";
    if (limit) {
      const [actual, threshold] = aligned(number, limit);
      if (bound === "min" ? actual < threshold : actual > threshold) return `数值${bound === "min" ? "不能小于" : "不能大于"} ${options[bound]}`;
    }
  }
  const step = options.step === undefined ? undefined : decimal(options.step);
  if (options.step !== undefined && (!step || step.coefficient <= 0n)) return "字段数字步长无效，请重新打开表单";
  if (step && step.coefficient > 0n) {
    const base = decimal(options.min ?? "0");
    if (!base) return "字段数字约束无效，请重新打开表单";
    const scale = Math.max(number.scale, base.scale, step.scale);
    const units = (item: Decimal) => item.coefficient * 10n ** BigInt(scale - item.scale);
    if ((units(number) - units(base)) % units(step) !== 0n) return `数值须符合步长 ${options.step}（起点 ${options.min ?? "0"}）`;
  }
}
