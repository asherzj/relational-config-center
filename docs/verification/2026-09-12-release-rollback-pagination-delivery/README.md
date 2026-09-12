# Issue #112: locate the rollback target across list pages

This delivery adds two E2E interactions: fill the existing release-order ID filter and submit the existing query before following the target row's detail link. Product code, permissions, SQL, request replay, timeouts and the seven business assertions are unchanged.

Base: `1687423d3e3643c1ed2d64b7691fea3c73aa43dd`.
Verified E2E source SHA-256: `c5b2a1bbc1656b5accab7c88368d4541ef856344cbeca5a0e2bb4b8b89860c00`.

## Validation

- [Integrated CI 34686061556](https://github.com/asherzj/relational-config-center/actions/runs/34686061556) preserved its failure: 11 browser cases passed, rollback Chromium failed before its seventh check, and 22 cases did not run. The visible first page had 20 distinct orders and a next-page control; the target was absent.
- A controlled real Admin/MySQL fixture creates 21 drafts and uses the greatest ID as the target. The existing ascending-ID pagination puts it beyond the first 20 records. The old navigation fails after six business checks.
- The query change returns exactly the target through the real ID-filter endpoint. All seven checks pass, with no document navigation or failed requests; SQL postchecks and cleanup pass.
- The delivered source, without the temporary seed or probe, passes all seven checks on each of Chromium, Firefox and WebKit using Playwright 1.63.0 on macOS arm64. Original same-body/same-key recovery, authorization and competing completion checks remain.
- Independent Standards review passed the two-line E2E change; Spec review verified the retained local evidence. The final Linux amd64 34-case matrix and all eight required CI jobs remain required before merge.

## Evidence retention

The complete raw logs, screenshots and executable diagnostic probes remain in local archive commit `8f22195036bc76f8a7a473b85c4ba367440cc040`, reviewed from tree `5faa5b7841d221670738d506bf8f78a8d581fe2c`.
Its evidence manifest SHA-256 is `6a78029b15cf9ae07c3c939a111aac1edcd385d1e3372ebca3e64e6772359b41`.

This public delivery contains the verified E2E change and this aggregate summary; it does not upload the new raw logs, screenshots or fixture records. The source bytes and verified behavior are unchanged by that packaging decision. Existing historical commits are preserved.
