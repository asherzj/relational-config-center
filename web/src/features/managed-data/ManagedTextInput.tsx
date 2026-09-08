import { Textarea } from "../../components/shadcn/textarea";
import { useId } from "react";
import { Button } from "../../components/ui/Button";

type Props = {
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  rows?: number;
};

// A textarea normalizes CRLF and CR in its DOM value. Keep the raw draft
// unchanged until the operator explicitly chooses LF normalization.
export function ManagedTextInput({ label, value, onChange, disabled, rows = 3 }: Props) {
  const explanationId = useId();
  const protectedCR = value.includes("\r");
  return (
    <div className="managed-text-input">
      <Textarea
        aria-label={label}
        aria-describedby={protectedCR ? explanationId : undefined}
        rows={rows}
        disabled={disabled}
        readOnly={protectedCR}
        value={value}
        onChange={(event) => { if (!protectedCR) onChange(event.target.value); }}
        onPaste={(event) => {
          if (protectedCR) { event.preventDefault(); return; }
          const pasted = event.clipboardData.getData("text/plain");
          if (!pasted.includes("\r")) return;
          event.preventDefault();
          const { selectionStart, selectionEnd } = event.currentTarget;
          onChange(value.slice(0, selectionStart) + pasted + value.slice(selectionEnd));
        }}
      />
      {protectedCR && <>
        <p id={explanationId} className="form-note">此值包含 CR 换行，原始内容已保留并设为只读。可以直接提交原值；如需修改，请先明确转换为 LF 换行。</p>
        <Button type="button" variant="ghost" disabled={disabled} aria-label={`${label}：转换为 LF 再编辑`} onClick={() => onChange(value.replace(/\r\n?/g, "\n"))}>转换为 LF 再编辑</Button>
      </>}
    </div>
  );
}
