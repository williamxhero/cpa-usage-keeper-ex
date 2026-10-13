# SPEC-003 API test disposition (#21)

No API test was deleted or skipped. Pure retired-management tests are renamed with `Removed` and rewritten to require the fixed 410 body and no service dispatch. Mixed cost tests keep their original cost/explanation assertions and use direct retained service operations only to prepare compatibility fixtures. This is not a mock HTTP success and does not make retired routes usable.

All filenames in the tables are relative to `internal/api/test/`. Names identify the **original** tests.

## Old global price/sync tests (`pricing_test.go`)

| Original test | Disposition and reason |
| --- | --- |
| `TestPricingSyncPreviewSelectsSource` | Rewrite + Removed: old source-selection preview is retired; every original source query returns 410 without consulting the provider. |
| `TestPricingSyncLiteLLMTimeoutNamesSelectedSource` | Rewrite + Removed: old upstream timeout mapping is obsolete; configured timeout is never consulted and response remains fixed 410. |
| `TestPricingSyncPreviewRoute` | Rewrite + Removed: old successful Models.dev preview is removed; require 410/no preview call. |
| `TestPricingSyncPreviewRouteMapsErrors` | Rewrite + Removed: deadline, wrapped network timeout and internal error fixtures must not be consulted; all return safe fixed 410. |
| `TestUpdatePricingRoutePreservesOpenAICacheWritePrice` | Rewrite + Removed: old four-rate write is unavailable; keep request, require 410/no write. Four-rate fidelity remains tested in service/migration/new APIs. |
| `TestUpdatePricingRouteRejectsLegacyCachePriceFields` | Rewrite + Removed: old payload parser no longer runs; even obsolete cache fields receive 410/no write, not 400. |
| `TestUpdatePricingRoute` | Rewrite + Removed: old model/multiplier write success is obsolete; original request receives 410/no write. |
| `TestBatchUpdatePricingRouteUsesOneProviderCall` | Rewrite + Removed: old batch dispatch is removed; require zero provider calls, not one. |
| `TestBatchUpdatePricingRouteMapsInvalidInputToBadRequest` | Rewrite + Removed: configured invalid-input error must never be consulted; unsafe old batch gets 410/no call. |
| `TestUpdatePricingRouteAllowsZeroPriceMultiplier` | Rewrite + Removed: zero is no exception to retirement; original zero request gets 410/no write. Explicit known-zero service/new tariff tests remain. |
| `TestUpdatePricingRouteMapsPriceMultiplierValidationToBadRequest` | Rewrite + Removed: old negative-multiplier validator is unreachable; request gets fixed 410, not echoed provider error. |
| `TestUpdatePricingRouteMapsInvalidSnapshotInputToBadRequest` | Rewrite + Removed: invalid/overflow old write cannot invoke candidate compilation; require 410/no call. |
| `TestUpdatePricingRouteAcceptsModelInBody` | Rewrite + Removed: body-model legacy write is removed; original slash-model payload gets 410/no call. |
| `TestDeletePricingRoute` | Rewrite + Removed: old global deletion is removed; original encoded model query gets 410/no deletion. |
| `TestPricingRoutesReturnEmptyResponsesWithoutProvider` | Selective rewrite: keep empty 200 used-model/global-price GET assertions; old sync-preview entry is now covered by fixed 410 regression. |
| `TestPricingRoutesReturnConfiguredData` | Preserve unchanged: used-model and global-price read compatibility is not retired. |

## Old rules (`pricing_rules_test.go`)

| Original test | Disposition and reason |
| --- | --- |
| `TestPricingRulesGETReturnsModelAndNormalizedRules` | Rewrite + Removed: rule-management read is retired; original query gets 410/no call. |
| `TestPricingRulesPUTPreservesOmittedNullAndExplicitZeroMultipliers` | Rewrite + Removed: all original omitted/null/zero requests get 410/no replacement; internal source fidelity remains covered. |
| `TestPricingRulesPUTAllowsEmptyArrayToClearRules` | Rewrite + Removed: empty replacement cannot clear retained rules; require 410/no call. |
| `TestPricingRulesRoutesMapValidationAndMissingModelErrors` | Rewrite + Removed: old 400/404 provider-error dispatch is gone; original requests get fixed 410. |
| `TestPricingRulesPUTRejectsNonJSONFiniteNumbers` | Rewrite + Removed: invalid JSON/nonfinite old payload cannot reach decoder/service; require 410/no call. |
| `TestPricingRulesRoutesRequireAdminAuthentication` | Preserve unchanged: authentication and administrator role boundary still precede retirement. |

## Mixed usage, persistence, security and authorization tests

| File / original test | Disposition and preserved assertions |
| --- | --- |
| `pricing_credential_defaults_test.go::TestCredentialOnlyAmbiguityServiceAndHTTPExplanation` | Rewrite fixture setup through services and retired management read to 410. Keep costs 2/30, dual costs, safe service/HTTP explanation, ambiguity and immutable prior resolver. |
| `pricing_credential_defaults_test.go::TestCredentialDefaultAdminHTTPPersistenceCostQueriesAndSafeValidation` | Original valid/invalid/secret/missing-subject bodies now require 410. Direct fixtures preserve all original cost transitions 30→3 (five formats)→zero/10/12→30→restart 2 and retained-history unavailable checks. Numeric JSON -0 remains a rejected HTTP request; service known-zero fixture uses canonical `0`, because retained text parser does not accept -0. |
| `pricing_credential_defaults_test.go::TestCredentialDefaultRoutesKeepAdminReadOnlyAndUnauthenticatedBoundaries` | Preserve unchanged authentication/role assertions. |
| `pricing_credential_models_test.go::TestCredentialModelAdminHTTPPersistedSelectionQueriesRestartClear` | Direct setup preserves used-model/alias, costs 30→12→restart 12→clear 30, selection and privacy. Old list/read/invalid/missing-model/missing-target requests now require 410. Align restarted service/catalog/router. |
| `pricing_credential_models_test.go::TestCredentialModelRoutesAdminOnly` | Preserve unchanged administrator boundary. |
| `pricing_credential_fixed_test.go::TestCredentialFixedAdminHTTPNoBaselinePersisted` | Direct fixed fixtures; original invalid mode/style/missing/overflow/field bodies and readback now require 410. Keep original 10→restart 10→zero 0→missing-baseline unavailable 0→restored 10→clear unavailable 0 cost assertions. |
| `pricing_channels_test.go::TestPricingChannelsAdminHTTPRealPersistenceQueriesReadbackAndDeletion` | Direct bind/channel/default/rename fixtures; keep overview/series/comparison/channel/provider/event costs (2,5)→(3,5)→(2,5), restart and privacy. Old readback/invalid/delete-confirmation requests get 410; final (2,5) proves rejected deletion cannot alter costs. |
| `pricing_channels_test.go::TestPricingChannelRoutesKeepAdminReadOnlyAndAnonymousBoundaries` | Preserve unchanged role/anonymous boundary. |
| `pricing_channel_models_test.go::TestChannelModelsAdminHTTPFiveLayerMatrixReadbackSwitchClearRestart` | Direct old-layer fixtures; old management readbacks get 410. Preserve every original arithmetic transition across hour/day queries, comparisons, events, exports, dual costs and privacy. Rebuild service with restarted catalog. These are pre-adoption/rollback compatibility assertions, not new strict precedence. |
| `pricing_channel_models_test.go::TestChannelFixedAdminHTTPAtomicInvalidInputsNoBaselineAndInactiveFields` | Direct old-layer fixtures; every original invalid body requires 410 and unchanged snapshot. Preserve independent fixed 10→credential missing 0→fallback 10→zero 0→multiplier unavailable 0→restart 10→clear unavailable 0. |
| `pricing_channel_models_test.go::TestChannelModelRoutesAdminAuthorizationAndSafeMissingTargets` | Preserve role assertions; administrator blank/absent targets now receive 410 instead of old 400/404. |
| `pricing_identity_migration_test.go::TestIdentityMigrationAdminHTTPRealSaveQueriesCorrectionPrivacyAndCompleteness` | Direct bind/default/channel/migrate/correct fixtures; keep costs 7→migrate 4→unbind 7→rebind 4, privacy and retained-history incomplete evidence. Old state/migration/channel read and invalid writes get 410; retain internal read-only identity-state checks. |
| `pricing_identity_migration_test.go::TestIdentityMigrationHTTPAdminBoundaries` | Preserve unchanged administrator boundary. |
| `pricing_credentials_test.go::TestPricingCredentialDirectoryAndErrorsUseSafeAllowlist` | Preserve every directory redaction/name/endpoint assertion; invalid manual bind POST gets 410 instead of parser 400. |
| `pricing_credentials_test.go::TestPricingCredentialProviderTypesUseSupportedSafeAliases` | Preserve unchanged supported safe aliases. |
| `pricing_credentials_test.go::TestPricingCredentialNamesNeverExposeAuthFileBasenamesOrTokenShapedText` | Preserve unchanged basename/token-shaped name redaction. |
| `pricing_credentials_test.go::TestPricingCredentialRoutesKeepAdministratorAndReadOnlyBoundaries` | Preserve unchanged directory/registration authorization boundaries. |
| `pricing_credentials_test.go::TestPricingCredentialSavedSubjectSurvivesRotationWithoutAutoMigration` | Retired POST gets 410; direct saved-subject fixture retains rotation/stale/no-auto-migration GET assertions. |
| `pricing_credentials_test.go::TestPricingCredentialSavedSubjectRemainsVisibleButNeverResolvesAnAmbiguousIdentity` | Direct bind fixture; ambiguous manual POST gets 410. Preserve subject visibility/ambiguity/no-auto-binding. |
| `pricing_credentials_test.go::TestPricingCredentialMissingOrUnverifiedIdentityCannotBeBound` | Retired POST gets 410; retain unknown/unbound/no-row assertions. |
| `pricing_credentials_test.go::TestPricingCredentialConflictingTypeDeclarationCannotBeBound` | Retired POST gets 410; retain unknown/unbound directory assertions. |
| `pricing_credentials_test.go::TestPricingCredentialResolutionRequiresTrustedUpstreamIdentityType` | Direct bind fixture; keep all exact trusted-type/ambiguity/downstream-key/whitespace resolution assertions. |
| `pricing_credentials_test.go::TestPricingCredentialAdministratorCanSelectBindAndReadAfterRestart` | Manual POST gets 410, then direct fixture. Preserve actual DB close/reopen, exact relation, names and rotation assertions. |
| `pricing_credentials_test.go::TestPricingCredentialBindingDoesNotChangeLegacyCostsOrRequestAttributes` | Manual POST/rules GET get 410; same-catalog fixture preserves publication, arithmetic 3.875, availability, dual costs, explanation privacy, payload equality, request attributes and upstream/downstream distinctions. Rules GET removed only from successful-read list and explicitly checked as 410. |
| `pricing_credentials_key_hint_test.go::TestPricingCredentialAPIsExposeOnlyMaskedKeyHints` | Preserve five key cases and directory/subject GET hint/privacy assertions. Manual POST gets 410 instead of obsolete successful DTO; service fixture enables subsequent saved-subject GET checks. |

Original cost assertion closures (`assertExplanation`, `assertHTTPPrice`, `assertQueries`, `assertCosts`, `costs`, `legacyPayload`, `assertPricingMetadata`) were compared byte-for-byte with baseline and preserved. Existing official/matrix tests and usage/export/cost tests outside this table have no content changes. No new strict pricing assertion was removed.

## Added regressions

- `TestRemovedPricingRoutesAlwaysReturnFixedGoneWithoutProvider`: all 30 registered method/path pairs, even with no provider.
- `TestRemovedPricingRoutesIgnoreAllInputsAndNeverCallService`: full optional-provider spy; valid, empty, malformed, invalid, trailing JSON, >4MiB body, oversized query, secret-shaped fields, missing/secret/oversized dynamic paths; fixed body, zero calls, safe responses/logs.
- `TestRemovedPricingRoutesKeepAuthenticationForEveryMethod`: anonymous 401, viewer 403, administrator 410 for every route; no provider dispatch.
- `TestRemovedPricingRoutesLeaveAllDatabaseRowsSnapshotAndMigrationUnchanged`: all variants through real service/isolated SQLite in unadopted and strict modes; every table's rows/content, audit/frozen migration fields, sequences/settings, and catalog pointer unchanged. Internal reports and retained reads remain operational.
- `TestRemovedPricingLayersDoNotChangeStrictUsageCostsOrExports`: overview hour/day, analysis, comparisons, events and JSON export; official 10→matrix 14→known zero 0→clear 10→restart 10, independent reference 10, availability, dual costs, snapshots, privacy and legacy isolation. Every tombstone leaves cost/rows unchanged; legacy fixed row alone stays unavailable.
- `TestPricingCredentialSyncRemainsOperationalAndAuthenticated`: configured sync status 200/refresh 202; anonymous 401/viewer 403; one refresh dispatch.

## Red/green evidence

- Before production changes: `go test ./internal/api/test -run '^TestRemovedPricingRoutesAlwaysReturnFixedGoneWithoutProvider$' -count=1` returned **1** (all 30 old responses violated 410).
- After production changes: the same regression returned **0**.
- Completed API tests: `go test ./internal/api/test -count=1` returned **0**.
- `go test ./internal/api/... -count=1` returned **0**.
- `go vet ./internal/api/...` returned **0**.
