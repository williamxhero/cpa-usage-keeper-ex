# SPEC-001 implementation evidence

Parent specification: https://github.com/williamxhero/cpa-usage-keeper-ex/issues/1

Delivery branch: `spec-001/provider-credential-pricing`

Baseline: `ced1c4316c6a83d366253d2efdd0ffec5e573f6d`

## Delivery boundary

This change is source-only. It does not modify CPA, installed services or databases, deploy, merge the delivery PR, or modify the parent specification issue. Existing model prices and legacy rules remain separate from new credential/channel configuration. All test identities, credentials, tokens and databases are synthetic and isolated.

## Ticket frontier

| Ticket | Scope | Blockers | Status | Commit / evidence |
| --- | --- | --- | --- | --- |
| [#2](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/2) | Safe credential selection and registration | — | Complete | `683f2db979f2dafbc80b5f109f9acaa1d3ec578a` |
| [#3](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/3) | Credential-wide default multiplier | #2 | Complete with historical-evidence limitation | `85351fd6`, `68354c5e`, `b586f11f` |
| [#4](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/4) | Named channels and channel default multiplier | #3 | Complete | `46836784`, `8809bb6e`, `e6bbbca8`, `7bd8074d`; integrated `869e4e00` |
| [#5](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/5) | Credential model-specific multiplier | #3 | Complete | `f50d1f53`, `c3ad0880`; integrated `f5a2f383` |
| [#6](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/6) | Credential model-specific fixed four-part tariff | #5 | Complete | `220a534b`, `f0c10bd9`, `86c99805`; integrated `e2c2d80f` |
| [#7](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/7) | Channel model exceptions and full precedence | #4, #6 | Complete | `aa4ef321`, `e712b4e4`, `965ffa9b`; integrated `ecea1ddd` |
| [#8](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/8) | Dual costs and request pricing explanation | #7 | In progress | — |
| [#9](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/9) | Stale binding and explicit identity migration | #4 | Complete | `d2059aff` through `de4dc552`; integrated `69b25f15` |

## Required verification

`make verify` was attempted in the integration worktree on 2026-10-07 and exited 127: `make: command not found`. The repository's Makefile defines the following six commands; each completed ticket and the final integration must run them directly while this environment lacks `make`:

1. `go test ./cmd/... ./internal/...`
2. `npm --prefix ./web ci`
3. `npm --prefix ./web run test`
4. `npm --prefix ./web run lint`
5. `npm --prefix ./web run typecheck`
6. `npm --prefix ./web run build`

### Ticket #2 checkpoint

All six equivalent checks passed on `683f2db9`. Full frontend tests passed with process-only `NODE_OPTIONS=--no-experimental-webstorage` and `--maxWorkers=2` (178 files, 1,366 tests). Node v26.7.0 exposes Web Storage globals incompatible with some unchanged existing tests; the vanilla run failed, and an unrestricted-worker rerun hit unrelated test timeouts. The bounded-worker compatible rerun passed without skipping tests, weakening assertions, raising timeouts, or changing machine/project configuration. An existing quota concurrency test initially failed under heavy simultaneous suites; it passed isolated and on the full backend rerun. Lint, typecheck, and build passed. Generated outputs were not committed.

## Acceptance evidence

### #2 — safe credential registration

- Stable Keeper `cred_` IDs persist exact stored identity/type relationships. Selection references are not advertised as permanent upstream IDs.
- Real administrator API tests cover save/readback, isolated SQLite close/reopen, refreshed metadata, rotated identities without automatic migration, and unchanged downstream-group independence.
- Missing/unverified/shared/conflicting identities are rejected without altering prior bindings; metadata ambiguity is retained before legacy directory deduplication.
- Safe allowlist tests cover secrets in names, auth filename/basename/stem, token-shaped labels, endpoint userinfo/query/fragment, and error responses; management routes preserve administrator-only access.
- Before/after registration tests compare unchanged legacy prices/rules/request attributes, Overview, comparison summaries, request details, cost availability, and match metadata.
- Component tests cover actual selection/submission/readback, permissions, load retry, save/conflict errors, and failed readback after a committed save. en/zh/zh-TW copy is complete.
- Browser acceptance used the actual built application router and UI against a new isolated SQLite fixture, with no background ingestion/metadata/quota/service runners and no real CPA connections. In Edge 154, explicit selection and registration succeeded on desktop (1344 CSS px) and mobile (375 CSS px); API readback returned the same saved opaque subject. Shared/ambiguous credentials were disabled. Screenshot review confirmed long names and stable IDs wrap in saved summaries and key controls remain usable. en, zh, and zh-TW were exercised; no missing translation keys were observed. The console contained only `chrome-extension://invalid/` resource failures, not application exceptions. The dedicated browser session and isolated app process were stopped after verification.

Next frontier: #3. No channel, new price override, or identity migration is claimed by #2.

### #3 — credential-wide defaults

Integrated final tip `b586f11f2f868f80d4302ce04d4cebffe1fc91cc`: backend `85351fd63c2cf771788d63260c723e8948826882`, frontend `68354c5e89768d2c194bd942a634ec9c233911e3` (cherry-picked from `6b94eac6`), and exact-projection corrections `b586f11f`.

- Administrator GET/PUT/DELETE `/api/v1/pricing/credentials/:subjectID/default` persists finite nonnegative multipliers, returns canonical numeric/null values and snapshot IDs, and explicitly clears inheritance. Decimal/x/X/% forms, active 0/1, invalid candidates, baseline overflow hidden by legacy zero, and later model-price writes are tested.
- One selected multiplier replaces all legacy multipliers/rules and scales the unadjusted four-part baseline. Missing baseline with billable tokens remains unavailable even at zero; no-billable-token available-zero semantics remain.
- Real isolated service/database/HTTP tests cover save/readback/clear/close-reopen restart, transaction rollback and deferred COMMIT failure, concurrent writers/pinned readers, metadata ambiguity and failure/recovery, request list/export, raw/cache realtime, hourly/daily Overview/series/analysis/comparisons/credential compositions, grouped/long quota windows, and persisted Codex quota efficiency.
- Review findings were corrected and regression-tested: exact original identity/type separate from normalized legacy dimensions; unknown raw type cannot borrow type-less inference; rejected typed quota rows cannot reselect defaults; mixed credentials retain independent fees/token buckets/failure/reasoning facts; committed metadata reload failure publishes an immutable unattributed fallback; retained evidence excludes post-aggregation rows and deduplicates archived IDs. No-default legacy projections and proven rejected-default grouped legacy fees are preserved.
- Historical reconciliation reads rollups, the committed Overview ID-prefix checkpoint, and hot/archive evidence in one reader transaction. Equal counts/token sums are accepted only for retained subsets of that committed prefix. Genuinely pruned original membership cannot be reconstructed: affected billable cohorts report `retained_pricing_evidence_incomplete`, retaining known unrelated subtotals. This is an explicit historical acceptance limitation. Zero-billable facts remain available and are not dropped merely because retained evidence is missing. Missing/deleted exact directory identities retain existing composition visibility limitations without borrowing another identity's fee.
- All six equivalent gates passed on the final tip: full Go tests, npm ci, full 179-file / 1,430-test suite (95.26 seconds; process-only Node compatibility flag and two workers), lint, typecheck, and build. `make verify` was actually retried and remained unavailable. Focused real service/API tests and final diff check passed; the ticket worktree was clean. No generated build output was committed.
- Actual browser acceptance against the built app/router with a new isolated synthetic SQLite fixture passed in Edge 154: select registered credential; save `20%` and read back 0.2; save active `0x` on mobile; explicitly clear to null/inheritance; reject negative `-1x`; save `120%` and read back 1.2. The actual Overview API returned $10 after clearing and $12 after 1.2, with the same current snapshot ID as the save readback. Desktop 1344 CSS px and mobile 375 CSS px screenshots were inspected; long names, IDs, input help, field errors, save/clear controls wrap and remain usable. en/zh/zh-TW were exercised. Console showed only a `chrome-extension://invalid/` resource failure. Dedicated browser session/app process and all temporary fixture databases/screenshots/harness files were removed; the shared browser daemon was not stopped.

Neither channel defaults nor credential model exceptions are claimed by #3.

### #4 — named channels and channel defaults

Accepted backend `46836784641b1308545820a4088fc06ac6c59fb1`, safety `8809bb6e86dbdefa491ea6894e9af157ea3b1e2b`, UI `e6bbbca8e51310f0f99f4c7ec58a6d73df09832c`, and normal accepted-#5 composition `7bd8074d87fdefb4a4c3bbed2be105ca7736421e`; integrated/pushed through `869e4e00`.

- Persisted opaque channels, safe names, explicitly selected exclusive credential-subject members, administrator CRUD/default save/readback, rename, inheritance clear, and dependency-confirmed deletion. Existing historical credential identities/prices are not cascaded away.
- Shared resolver precedence is credential model > credential default > channel default > complete legacy; provider type/name/endpoint never identifies a channel. Channel-only configuration activates evidence guards. Genuine named-channel comparisons retain distinct $2/$5/$7 amounts, exact typed membership and explicit unknown/incomplete historical attribution.
- Real service/admin HTTP/compiler tests cover member conflicts, credential .3 priority, composed Model-before-Alias selection, clear-through-all-layers, four buckets, hot/archive reconciliation, query-family agreement, pinned readers, candidate/COMMIT failures and restart.
- Independent review's known-secret channel-name defect was corrected: save validates against all known directory/subject evidence; successful reload sanitizes outward snapshot names; committed metadata followed by failed reload publishes generic names and disables stale attribution without mutating pinned snapshots/configurations. Actual metadata failure/recovery privacy regression was reproduced red then green and the narrow correction reviewed.
- Final composed focused backend passed 52 top-level / 71 including-subtest cases. Full Go suite passed 31 test packages, 1,696 top-level / 3,014 including subtests, zero failures. npm ci, full frontend tests (181 files / 1,493 tests with process-only Node compatibility flag/two workers), lint, typecheck and build passed. Actual make verify remained unavailable, and all recipes were executed individually. Passing frontend fixture socket stderr and existing bundle-size warning remain. Final worktree/diff checks clean; tracked dist placeholder restored.

### #5 — credential model multipliers

Accepted backend `f50d1f53c114b98b06a6d814c6afc530822d0fd8` and UI `c3ad08803d9bf1288b3118add922a7ea1f8ec464`, integrated and pushed via `f5a2f383`.

- Unique normalized subject/model configurations; independent scoped Model-then-ModelAlias override selection above the credential default, without changing the baseline's existing Model-then-Alias lookup. Selected multiplier scales that same unadjusted reference; no legacy/default stacking or unavailable-baseline downgrade.
- Administrator list and GET/PUT/DELETE model routes support canonical readback, active 0/1, explicit clearing, persistence/restart, and exact safe selection metadata. Model-only configuration activates all existing attribution/evidence guards even when no credential-wide default exists.
- Six new real service tests and two HTTP tests cover all cost families, future baseline validation, exact typed identity, missing baseline/zero, failure and COMMIT atomicity, concurrency, inheritance and restart. A deliberate default-only guard mutation failed (33.24 instead of expected 33.44), then passed when the model-aware guard was restored. Bounded independent backend review found no verified actionable blocker.
- Focused backend checks passed across five packages. Focused frontend tests passed 75 tests in three files, including 32 new card cases, translations and responsive layout. Full Go suite passed (31 successful package results); npm ci, full frontend tests (180 files / 1,464 tests, 98.90 seconds, process-only Node compatibility flag/two workers), lint, typecheck and build passed. Actual `make verify` exited 127 because make remains unavailable. No retries, skipped assertions or configuration changes were needed. Nonfatal frontend socket-fixture stderr and build-size warnings remain. Final diff checks passed; the ticket tree was clean and generated assets were removed.

### #6 — complete credential model fixed tariffs

Accepted backend `220a534b7789d5815d3df1e0f7b852d15d17f7bc`, UI `f0c10bd9c5f03a2a5da157f0400ead73be5541ea`, and normal channel composition `86c99805822c3166abc384ea562f6953868320b9`; integrated/pushed through `e2c2d80f`.

- One subject/model row has mutually exclusive multiplier or fixed mode. Four explicit finite nonnegative rates are required; omitted/null/blank/invalid portions reject the complete candidate, while all-zero tariffs remain active. Complete mode switches discard hidden inactive fields and clearing restores inheritance.
- Fixed pricing uses the existing independently matched request baseline style when available; otherwise it requires a saved explicit existing style. Fixed estimates can be available without a baseline, whose separate request reference remains null/unavailable. Higher selected multipliers (including zero) with no usable baseline do not downgrade to alias fixed tariffs/channel defaults.
- Real persisted service/admin HTTP and composed-channel regressions cover $10/$20/$30 across cost families and channel comparisons, exact identity, per-event four-bucket clamping, mode replacement, future baseline/style validation, partial/missing evidence, restart, failed COMMIT and concurrent pinned readers. Initial backend and final narrow #4+#6 compiler/loader/resolver/guard reviews found no verified blockers.
- Final composed Go suite passed (31 successful package results); npm ci passed (239 packages); focused frontend passed 151 tests in four files; full frontend passed 181 files / 1,540 tests (85.80 seconds, process-only compatibility flag/two workers). Lint passed with zero warnings, typecheck and build passed (250 modules). Both pre/post-composition make verify attempts failed because make remains unavailable; individual recipes all passed. No full-suite retries. Nonfatal ECONNRESET fixture stderr and existing bundle/plugin warnings remain. Own new-test control-helper corrections and a hook warning were fixed rather than suppressing warnings/changing valid tests or configuration. Final tree/diff checks clean; generated assets removed and tracked placeholder restored.

### #7 — channel model exceptions and full precedence

Accepted backend `aa4ef321c8deb7226cd7b6ded2a574e077a3ba46`, supplemental acceptance `e712b4e4`, and UI/final tip `965ffa9be5a479cbc332395b504aadca1ec7fa1e`; integrated/pushed through `ecea1ddd`.

- Channel/model multiplier or complete fixed tariffs persist in one mutually exclusive scope. Exact scoped Model-before-Alias and scope-before-candidate selection compose the full order: credential model > credential default > channel model > channel default > complete legacy. Baseline/style/reference lookup stays independently unchanged.
- Real persisted matrix covers legacy $30, channel default $2, channel model $11, credential default $3 and credential model $12, clear-through-every-layer, credential Alias over channel Model, both modes, four-bucket clamping, all cost families, model-only guards, hot/archive duplicate membership, padded/unknown/opposite raw identity types, missing-baseline multiplier-zero no downgrade, candidate/COMMIT rollback, concurrency, restart and dependency-confirmed deletion preserving credential prices/history.
- API/UI copy in all languages reflects next-layer inheritance rather than incorrectly promising legacy after an individual clear. Bounded independent backend review found no verified actionable blockers.
- Final full Go passed 31 packages, 1,716 top-level / 3,042 including subtests; npm ci (239 packages), full frontend (182 files / 1,643 tests), lint, typecheck and build (251 modules) passed. Actual make verify remained unavailable; equivalent gates were executed. Final ticket tree/diff checks clean; tracked placeholder restored and own build outputs removed.

### #9 — explicit identity migration and correction

Accepted backend `d2059affc180c3648eeadd4fdc8e4b6d4747b61c`, UI `91da01a7`, accepted-#6 composition `266c3b55`, fixed-mode migration acceptance `402421795ea8aea554e0e04429af1b9d1bc6b6bc`, and rollback/provider evidence/final tip `de4dc552b3e6535026d3053a76082b410ce7061d`; integrated/pushed through `69b25f15`.

- Additive exact enabled/disabled association overlays retain original subject relations. Confirmed migration appends a new exact identity to the same stable subject without rewriting history/configuration/channel membership. Separately confirmed correction unbinds/rebinds one opaque association; disabled origins cannot resurrect via ordinary registration or snapshot reload.
- Safe administrator state/selection references and localized responsive UI distinguish unbound/current/stale/history, require explicit confirmation, retain committed receipts after failed readback, and invalidate stale selections. Names/endpoints/types never trigger automatic migration; controlled metadata timeout/partial scopes do not delete or erase historical ownership.
- Real service/admin HTTP tests cover old/new identity fees and channel sums across cost families, exact type/whitespace, occupied/shared/duplicate selection conflicts, missing/stale state, candidate/deferred-COMMIT rollback, concurrent pinned readers, actual close/reopen restart, correction, fixed/default/model/channel row preservation, retained clamping/archive/completeness and scoped provider restore/failure. Independent backend/final #9+#6 seam review and coordinator UI inspection found no blockers. Unknown raw types and type-less shared-index ambiguity remain conservative, not inferred.
- All six gates passed on the final ticket tip: full Go, npm ci, frontend 182 files / 1,551 tests, lint, typecheck and build. Actual make verify exited 127 because make remains unavailable; final tree/diff checks clean.
- The coordinator resolved four mechanical #7+#9 conflicts by retaining both migration registrations and both API import/test sets, without inventing behavior. On composed integration source: focused backend passed four packages, focused frontend passed 364 tests in 12 files; full Go, npm ci, full frontend (183 files / 1,654 tests, 72.92 seconds), lint, typecheck and build (253 modules) passed. No tests were skipped or weakened. Existing fixture socket stderr and bundle-size warning remained nonfatal.

Current frontier: #8, backend and display-only frontend in parallel, based on the fully verified #2–#7+#9 integration. Draft delivery PR: https://github.com/williamxhero/cpa-usage-keeper-ex/pull/10 (not ready to merge).

## Remaining limitations

Implementation is not complete. No deployment, production-data validation, or runtime-service change is part of this delivery. The snapshot identifier is for current-query consistency, not historical billing versions.
