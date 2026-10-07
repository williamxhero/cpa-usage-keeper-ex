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
| [#6](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/6) | Credential model-specific fixed four-part tariff | #5 | In progress | — |
| [#7](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/7) | Channel model exceptions and full precedence | #4, #6 | Pending | — |
| [#8](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/8) | Dual costs and request pricing explanation | #7 | Pending | — |
| [#9](https://github.com/williamxhero/cpa-usage-keeper-ex/issues/9) | Stale binding and explicit identity migration | #4 | In progress | — |

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

Current frontier: #6 and #9 in parallel. #7 waits for #6; #8 waits for #7. Draft delivery PR: https://github.com/williamxhero/cpa-usage-keeper-ex/pull/10 (not ready to merge).

## Remaining limitations

Implementation is not complete. No deployment, production-data validation, or runtime-service change is part of this delivery. The snapshot identifier is for current-query consistency, not historical billing versions.
