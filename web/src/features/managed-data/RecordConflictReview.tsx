import { Button } from "../../components/ui/Button";
import type { ManagedDataMutationOutcome } from "./model";

export type RecordConflictReviewProps = {
  recordConflict?: boolean;
  latest?: ManagedDataMutationOutcome | null;
  pending?: boolean;
  onInspectLatest?: () => void;
  onRebuildLatest?: () => void;
};

export function RecordConflictReview({ recordConflict, latest, pending, onInspectLatest, onRebuildLatest }: RecordConflictReviewProps) {
  if (!recordConflict && !latest) return null;
  return <section className="record-conflict-review" aria-label="记录版本冲突">
    <p>输入和原差异已保留。查看最新值后，可重建差异并再次确认。</p>
    <Button disabled={pending} onClick={onInspectLatest}>查看最新值</Button>
    {latest?.row && <>
      <dl aria-label="最新记录">{Object.entries(latest.row).map(([field, value]) => <div key={field}><dt>{field}</dt><dd>{value === null ? "NULL" : value === "" ? '""' : value}</dd></div>)}</dl>
      <Button disabled={pending} onClick={onRebuildLatest}>基于最新值重建差异</Button>
    </>}
  </section>;
}
