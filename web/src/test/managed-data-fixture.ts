// Existing managed-row fixtures represent legacy records at the implicit zero baseline.
// Keep explicit metadata untouched so tests can choose another version or an invalid response.
export function withDefaultRecordVersions(value: unknown): unknown {
  if (value && typeof value === "object" && "rows" in value && Array.isArray(value.rows) && !("record_versions" in value)) {
    return { ...value, record_versions: value.rows.map(() => "0") };
  }
  return value;
}
