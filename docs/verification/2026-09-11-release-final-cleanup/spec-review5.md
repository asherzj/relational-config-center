# Spec re-review — candidate 5

**Residual Spec findings: 0.**

Reviewed candidate 4 tree `00ea9518131727e429d0ffefbb4ba8c077d8c07d` → candidate 5 tree `f0228e09a08ca55d0c85a946bb25aa80a7974ea3`, the complete `web/e2e/release-drafts.cjs`, and the existing editor/write-hook completion sequence. Only that E2E file changed. Its current SHA-256 is `87c4bc38d1a8fd45dfd43c0cac84c648aec17c9a03910f901ffeb8917e81d8c5`, matching `source-candidate5.json`. The manifest file's SHA-256 is `98424453575ff9ae6c9c3760dd850e1e16ef855394bb03a611d8edfa3bcdfa41`. Current contents match candidate 5; unstaged/untracked inspection is empty.

**AC-005 — “版本冲突保留输入”:** lines 48–51 establish the intended sequence. The first editor opens and is modified before the second window opens. A response listener is installed before the second save; the script requires that order's PUT to return 200, waits for the second editor to close and its saved value to appear, and only then submits the first window. The existing editor closes after awaiting its write hook, whose success path awaits journal cleanup. This barrier covers application handling beyond merely seeing edited text. The first editor retains its original baseline, so the subsequent separately observed PUT must return 409. The exact conflict message, retained first-window value, explicit latest-state inspection, reconstruction, and subsequent save remain required.

**AC-012 and inherited regression:** lost-response original-key/body replay, rejection across refreshes, preserved original intent, and explicit reconstruction remain unchanged. The added failure screenshots/text preserve diagnostics and rethrow the error; they do not turn failure into success or remove an assertion. No production change, compatibility path, automatic write retry, or new business behavior is introduced.

Browser 4's two PUT 200 responses and missing 409 remain a failed attempt, not CAS evidence. Code review confirms the revised ordering and stronger HTTP assertion; it does not establish runtime PASS. Full browser and Compose acceptance remain pending under the coordinator's verification.

No tests, Go commands, builds, or database operations were run, and no source files were edited. Only this requested review report was written.
