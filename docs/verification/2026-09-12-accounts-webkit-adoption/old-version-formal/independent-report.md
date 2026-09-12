# PR111 old-version formal browser verification

Run34684793011, job103529792278, head4b45aa1b3f16a9ca8401cc7aa0c887c751d0aae4. Formal package is still Playwright1.62.1/WebKit2336, distinct from isolated1.63 comparison. Raw selected17:16PASS, accounts.mjs@webkit FAIL, remaining17 unexecuted. All3 release-rollbacks cases passed.

First publication confirmation accounts.mjs322 (diagnostic wrapper preserves click) raised TargetClosed after22388ms, with reopened-persistent page-crash35262ms, confirmation-end/test-failure35264ms, cleanup-start35267ms. Not the600s case deadline. No native trace in this formal job, so no attribution to the exact previously captured compositor fault. The failure retains original semantics; it is not evidence against the isolated2359 candidate.

Raw log /tmp/rcc-111-version-formal-browser.log; artifact /tmp/rcc-111-version-formal-artifacts. Artifact10295382369 zip digest a0e5276127e336126a89624f98f52e03cbfb27eb772c161599caa9040684ff43. Root independently checked case matrix and lifecycle. Formal gate FAIL; original other jobs still running at read time.
