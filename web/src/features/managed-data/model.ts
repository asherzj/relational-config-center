export type ManagedDataColumnType =
  | "uint64"
  | "int64"
  | "decimal"
  | "float64"
  | "string"
  | "boolean"
  | "date"
  | "time"
  | "datetime"
  | "timestamp"
  | "json";

export type ManagedDataColumn = {
  name: string;
  type: ManagedDataColumnType;
  nullable: boolean;
  generated?: boolean;
};

export type QueryOperator =
  | "exact"
  | "contains"
  | "open_range"
  | "closed_range"
  | "in"
  | "not_in"
  | "is_null"
  | "is_not_null";

export type QueryCondition =
  | { field: string; operator: "exact" | "contains"; value: string }
  | { field: string; operator: "open_range" | "closed_range"; from?: string; to?: string }
  | { field: string; operator: "in" | "not_in"; values: string[] }
  | { field: string; operator: "is_null" | "is_not_null" };

export type QueryOrder = { field: string; direction: "ASC" | "DESC" };

export type QuerySpec = {
  conditions: QueryCondition[];
  order?: QueryOrder;
  pageNumber?: number;
  pageSize?: number;
};

export type ManagedDataResult = {
  recordVersions: string[];
  columns: ManagedDataColumn[];
  rows: Record<string, string | null>[];
  page: {
    pageNumber: number;
    pageSize: number;
    totalCount: number;
    totalPages: number;
  };
};

export type MutationContent = Record<string, string | null>;

export type ChangeSetOperation = "ADD" | "MODIFY" | "DELETE";
export type ChangeSetCell =
  | { state: "value"; value: string }
  | { state: "null" | "empty" | "unsubmitted" | "missing" };

export type ChangeSetRow = {
  field: string;
  original: ChangeSetCell;
  next: ChangeSetCell;
  changed: boolean;
  autoFill: boolean;
};

export type ChangeSet = {
  operation: ChangeSetOperation;
  rows: ChangeSetRow[];
};

export type ManagedDataMutationOutcome = {
  recordVersion?: string;
  operation: ChangeSetOperation;
  tableName: string;
  id: string;
  row?: Record<string, string | null>;
  columns?: ManagedDataColumn[];
  retrievalError?: unknown;
};

function cellForValue(value: string | null): ChangeSetCell {
  if (value === null) return { state: "null" };
  if (value === "") return { state: "empty" };
  return { state: "value", value };
}

function cellsEqual(left: ChangeSetCell, right: ChangeSetCell) {
  return left.state === right.state
    && (left.state !== "value" || (right.state === "value" && left.value === right.value));
}

export function buildChangeSet(
  operation: ChangeSetOperation,
  columns: readonly ManagedDataColumn[],
  original: Record<string, string | null> | undefined,
  content: MutationContent,
  autoFillFields: ReadonlySet<string>,
): ChangeSet {
  return {
    operation,
    rows: columns.map((column) => {
      const autoFill = autoFillFields.has(column.name);
      if (operation === "ADD") {
        const next = Object.hasOwn(content, column.name)
          ? cellForValue(content[column.name]!)
          : { state: "unsubmitted" as const };
        return { field: column.name, original: { state: "missing" }, next, changed: true, autoFill };
      }

      const originalCell = cellForValue(original?.[column.name] ?? null);
      if (operation === "DELETE") {
        return { field: column.name, original: originalCell, next: { state: "missing" }, changed: true, autoFill: false };
      }

      const next = autoFill
        ? { state: "unsubmitted" as const }
        : Object.hasOwn(content, column.name)
          ? cellForValue(content[column.name]!)
          : originalCell;
      return { field: column.name, original: originalCell, next, changed: !cellsEqual(originalCell, next), autoFill };
    }),
  };
}

export const queryOperatorLabels: Record<QueryOperator, string> = {
  exact: "exact",
  contains: "contains",
  open_range: "open_range",
  closed_range: "closed_range",
  in: "in",
  not_in: "not_in",
  is_null: "is_null",
  is_not_null: "is_not_null",
};

const commonOperators: QueryOperator[] = ["exact", "in", "not_in", "is_null", "is_not_null"];

export function allowedOperators(column: ManagedDataColumn): QueryOperator[] {
  if (column.type === "string") return ["exact", "contains", "open_range", "closed_range", "in", "not_in", "is_null", "is_not_null"];
  if (column.type === "boolean" || column.type === "json") return commonOperators;
  return ["exact", "open_range", "closed_range", "in", "not_in", "is_null", "is_not_null"];
}

export type QueryConditionDraft = {
  field: string;
  operator: QueryOperator;
  value: string;
  fromEnabled: boolean;
  from: string;
  toEnabled: boolean;
  to: string;
  values: string[];
};

export function createConditionDraft(field: string): QueryConditionDraft {
  return { field, operator: "exact", value: "", fromEnabled: true, from: "", toEnabled: false, to: "", values: [""] };
}

export function conditionFromDraft(draft: QueryConditionDraft): QueryCondition {
  switch (draft.operator) {
    case "exact":
    case "contains":
      return { field: draft.field, operator: draft.operator, value: draft.value };
    case "open_range":
    case "closed_range":
      return {
        field: draft.field,
        operator: draft.operator,
        ...(draft.fromEnabled ? { from: draft.from } : {}),
        ...(draft.toEnabled ? { to: draft.to } : {}),
      };
    case "in":
    case "not_in":
      return { field: draft.field, operator: draft.operator, values: draft.values };
    case "is_null":
    case "is_not_null":
      return { field: draft.field, operator: draft.operator };
  }
}

const decimalDigits = "[0-9](?:_?[0-9])*";
const hexadecimalDigits = "[0-9a-fA-F](?:_?[0-9a-fA-F])*";
const decimalFloatPattern = new RegExp(`^[+-]?(?:(?:${decimalDigits}\\.(?:${decimalDigits})?|\\.${decimalDigits})(?:[eE][+-]?${decimalDigits})?|${decimalDigits}(?:[eE][+-]?${decimalDigits})?)$`);
const hexadecimalFloatPattern = new RegExp(`^[+-]?0[xX]_?(?:(?:${hexadecimalDigits}\\.(?:${hexadecimalDigits})?|\\.${hexadecimalDigits}|${hexadecimalDigits}))[pP][+-]?${decimalDigits}$`);

function validFloat64(value: string): boolean {
  if (decimalFloatPattern.test(value)) return Number.isFinite(Number(value.replaceAll("_", "")));
  if (!hexadecimalFloatPattern.test(value)) return false;

  const normalized = value.replaceAll("_", "");
  const unsigned = normalized.replace(/^[+-]/, "").slice(2);
  const [mantissa, exponentText] = unsigned.split(/[pP]/);
  const [integerPart = "0", fractionalPart = ""] = mantissa!.split(".");
  const digits = `${integerPart}${fractionalPart}`.replace(/^0+/, "");
  if (digits === "") return true;
  const significand = BigInt(`0x${digits}`);
  const binaryScale = BigInt(exponentText!) - 4n * BigInt(fractionalPart.length);
  const topExponent = BigInt(significand.toString(2).length - 1) + binaryScale;
  if (topExponent < 1023n) return true;
  if (topExponent > 1023n) return false;

  // ParseFloat rounds to nearest-even. Values at or above the midpoint
  // between MaxFloat64 and +Inf produce ErrRange and must fail closed here.
  const overflowMidpoint = (2n ** 54n - 1n);
  const overflowScale = 970n;
  const atOrAboveOverflow = binaryScale >= overflowScale
    ? significand << (binaryScale - overflowScale) >= overflowMidpoint
    : significand >= overflowMidpoint << (overflowScale - binaryScale);
  return !atOrAboveOverflow;
}

function validDateParts(year: number, month: number, day: number): boolean {
  if (month < 1 || month > 12 || day < 1) return false;
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const days = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return day <= days[month - 1]!;
}

function validDate(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  return Boolean(match && validDateParts(Number(match[1]), Number(match[2]), Number(match[3])));
}

function validDateTime(value: string, separator: " " | "T", utcSuffix: boolean): boolean {
  const suffix = utcSuffix ? "Z" : "";
  const pattern = new RegExp(`^(\\d{4})-(\\d{2})-(\\d{2})${separator}(\\d{2}):(\\d{2}):(\\d{2})(?:\\.\\d{1,6})?${suffix}$`);
  const match = pattern.exec(value);
  return Boolean(match
    && validDateParts(Number(match[1]), Number(match[2]), Number(match[3]))
    && Number(match[4]) <= 23
    && Number(match[5]) <= 59
    && Number(match[6]) <= 59);
}

function validValue(column: ManagedDataColumn, value: string): boolean {
  switch (column.type) {
    case "string":
      return true;
    case "uint64":
      try { return /^[0-9]+$/.test(value) && BigInt(value) <= 18_446_744_073_709_551_615n; } catch { return false; }
    case "int64":
      try {
        const parsed = BigInt(value);
        return /^[+-]?[0-9]+$/.test(value) && parsed >= -9_223_372_036_854_775_808n && parsed <= 9_223_372_036_854_775_807n;
      } catch { return false; }
    case "decimal":
      return /^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$/.test(value);
    case "float64":
      return validFloat64(value);
    case "boolean":
      return value === "0" || value === "1";
    case "date":
      return validDate(value);
    case "time":
      return /^(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]{1,6})?$/.test(value);
    case "datetime":
      return validDateTime(value, " ", false);
    case "timestamp":
      return validDateTime(value, "T", true);
    case "json":
      try { JSON.parse(value); return true; } catch { return false; }
  }
}

export function validateQueryDraft(columns: ManagedDataColumn[], drafts: QueryConditionDraft[], pageSize: string): string | null {
  if (drafts.length > 20) return "AND 条件不能超过 20 个。";
  if (pageSize !== "" && (!/^[0-9]+$/.test(pageSize) || Number(pageSize) < 1 || Number(pageSize) > 200)) {
    return "每页数量必须是 1 到 200 的整数。";
  }
  for (let index = 0; index < drafts.length; index += 1) {
    const draft = drafts[index]!;
    const column = columns.find((item) => item.name === draft.field);
    if (!column || !allowedOperators(column).includes(draft.operator)) return `条件 ${index + 1} 的字段或操作符已失效，请重新选择。`;
    if ((draft.operator === "open_range" || draft.operator === "closed_range") && !draft.fromEnabled && !draft.toEnabled) {
      return `条件 ${index + 1}：Range 至少需要一个边界。`;
    }
    const values = draft.operator === "exact" || draft.operator === "contains"
      ? [draft.value]
      : draft.operator === "open_range" || draft.operator === "closed_range"
        ? [...(draft.fromEnabled ? [draft.from] : []), ...(draft.toEnabled ? [draft.to] : [])]
        : draft.operator === "in" || draft.operator === "not_in" ? draft.values : [];
    if ((draft.operator === "in" || draft.operator === "not_in") && (values.length === 0 || values.length > 100)) {
      return `条件 ${index + 1}：集合值数量必须是 1 到 100。`;
    }
    if (values.some((value) => !validValue(column, value))) return `条件 ${index + 1} 的值不符合 ${column.name} 的 ${column.type} 格式。`;
  }
  return null;
}
