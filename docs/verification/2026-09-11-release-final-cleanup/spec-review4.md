# Spec re-review — candidate 4

**Residual Spec findings: 0.**

Reviewed candidate 3 tree `44b4241b2605df6b7641f28d15a3f3cf7ca66d6c` → candidate 4 tree `00ea9518131727e429d0ffefbb4ba8c077d8c07d`, the complete surrounding rejection/320px/keyboard assertions, and the existing `release-detail-pages.cjs` helper. The delta is exactly one line in `web/e2e/browser-accessibility.cjs:325`. Current contents match candidate 4 and the entry in `source-candidate4.json`. The source file `web/e2e/browser-accessibility.cjs` has SHA-256 `b3dd9164ee49a1430338d50300d9a01eb85172ec418959bab63f28149fb17bb7`; the manifest file `source-candidate4.json` has SHA-256 `b2dec0a3d531d90f989e1aac65a3c1935bb62236349b818101206e91bf885a3f`. Unstaged/untracked inspection is empty.

**AC-001 / AC-015 and the final paged-read contract:** reading the header through `readAllReleaseDetailPages` makes the fixture obtain actual server details instead of assuming an obsolete embedded array. The helper requires a header without `items`, binds every page to the same order/version/count/offset, verifies page completeness, and rejects failed or inconsistent reads. It does not synthesize application content or conceal a failed assertion.

**AC-011 / AC-012 and inherited field interaction:** the following assertions still require `APPROVED`, exact raw-CR input, no successful execution, and zero matching SQL rows after the real 422 publication rejection. Cancellation/copy, explicit LF conversion, unsaved exit, and the preceding 320px surface/footer bounds and real Tab/Escape checks remain intact. There is no new compatibility path, automatic write retry, or scope expansion in this fixture correction.

Browser 3 stopped at the obsolete fixture and did not reach the shared completion guard; its preceding scope successes are not a full-run PASS. Browser 4 three-engine acceptance remains under the coordinator's verification. This review makes no browser-success or final-delivery claim.

No source edits, tests, Go commands, builds, or database operations were performed. Only this requested review report was written.
