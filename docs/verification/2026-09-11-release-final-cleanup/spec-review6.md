# Spec re-review — candidate 6

**Residual Spec findings: 0.**

Reviewed candidate 5 tree `f0228e09a08ca55d0c85a946bb25aa80a7974ea3` → candidate 6 tree `8b835f78aea0ad79df7d0ee5e7b4c1323e875dd4`, the changed large-value path and surrounding setup/recovery assertions in `web/e2e/release-multitable.cjs`, and the field-policy contract. Only that E2E path changed; current contents match candidate 6, with no unstaged/untracked changes. The source file SHA-256 is `d6684959e36b0f601943c93cf0ec9b3e1ba3343da10e492e59a15498ebf505a3`. The manifest `source-candidate6.json` SHA-256 is `996f86f5a71fc5e3631bba955e706442e06ac4b852a46c84ec45eea321a84e20`; its source entry matches.

**AC-006 — no old byte budget:** lines 154–176 configure `payload` as textarea through the real administrator UI, await the successful policy PUT, verify GET effective configuration and the actual TEXTAREA, then save the unchanged large payload. This uses the supported configuration described in `docs/admin-field-policies.md:21`; it does not change the default text control or production behavior. Full input and persisted-content equality are added, while the existing request-size assertion remains strictly greater than 9 MiB. The diagnostic probe distinguishes the observed native WebKit single-line insertion cap from textarea capacity; the payload is not shortened to accommodate it.

**AC-012 — manual original-request recovery:** the committed-response loss still occurs after real HTTP 201. Refresh, revocation, account switching, zero additional POSTs before authorized manual retry, identical original key/body digest, version 1, and exact final payload remain required. Storage-failure zero-send and retained-input checks also remain. The added ordinary GET does not clear the pending request or replace replay with a new request.

The formal targeted WebKit log records 10 checks and the shared successful post-check. `large-request-diagnostic.json` records 3,145,728 characters / 9,437,184 payload bytes with identical expected, input, captured and persisted hashes; the complete request is 9,437,328 bytes. Recovery retains the same key and order. These observations support this increment, without replacing other capacity evidence.

No weakened assertion, missing requirement, compatibility residue, or scope expansion was found. Full 34-scope browser and Compose acceptance remain pending. No tests, builds, Go commands, database operations or source edits were performed; only this requested report was written.
