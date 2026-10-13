# SPEC-003 frontend test disposition (#21)

## Scope and retained regression coverage

Only dormant old management clients, their exclusive callers and their behavior tests are retired. No pricing snapshot, dual-cost, cost, key-viewer, matrix or official tests are deleted or skipped. Legacy styles/types/translations may remain inert; no broad cleanup or runtime change.

- `pricingApi.test.ts` is rewritten for retained reads, the retired-export boundary, cancellation, credentials/intent and safe HTTP-status errors. `pricingMatrixApi.test.ts` stays intact.
- `useMatrixPricingData.test.tsx` transfers latest callbacks, 401 versus permission failures, safe errors, atomic complete four-price/zero writes, cancellation/stale results on query/source/disable/unmount/permission changes, callback isolation and official preview/explicit commit. It also adds canceled mutation-readback regressions.
- `PricingMatrixCard.test.tsx`, `PricingOfficialCard.test.tsx`, `UsagePagePricing.test.tsx`, `KeyPricingPage.test.tsx`, matrixPricing translations and all snapshot/dual-cost/cost suites remain intact.
- Shared model-ordering cases formerly exercised through `buildPricingModelOptions` are transferred to `utils/test/modelSort.test.ts` against the retained comparator; request-events ordering coverage remains intact.
- Mixed `UsagePage.styles.test.ts`: retain dual-cost/new-page/other-layout assertions, retarget the four-rate responsive check to the matrix, and remove only old subtree checks. The scroll-boundary test retains request/log/API-key/session assertions without requiring old `.pricesGrid`.
- Mixed `i18n/test/pricing-rules.test.ts`: preserve the dual-cost translation test unchanged; remove obsolete five-layer/editor copy expectations, leaving resources unchanged.


## Compact original test definitions (PR-inline inventory)

The exhaustive inventory below includes every expanded case. This compact table groups each original `it`/`it.each` definition once and reports its case count; copy this section into the PR and link the full document. Reasons follow each suite's exhaustive table.

### `src/i18n/test/pricing-rules.test.ts` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| provides separate migration retention and correction ownership/history warnings without fallback | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| has complete nonempty exact-match, inheritance, baseline and history copy in every language | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| has complete localized channel, exact-match, precedence, fixed tariff and history copy | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| has complete nonempty copy without fallback in every language | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| describes every layer, clearing one scope, and restoration only after all new overrides are cleared | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| has complete selector, save, error, identity and status copy in every language | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| provides all management, priority, history and dependency copy in every language | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| keeps the help copy limited to the two approved examples | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| interpolates the selected count in every language | 1 | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |

### `src/lib/test/pricingApi.test.ts` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| uses exact opaque references, bounded safe payload fields, credentials and cancellation | 1 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| requests the selected pricing source %s with cancellation | 2 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| updates one model through the pricing endpoint without sending a pricing snapshot | 1 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| deletes one model through the pricing endpoint without sending a request body | 1 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| updates multiple model prices in one batch request | 1 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| loads saved exceptions with encoded stable channel ID, base path and cancellation | 1 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| uses exact model query and canonical response for %s | 4 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| preserves status %s for safe localized handling | 7 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| loads rules through a query parameter so model names may contain slashes | 1 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| replaces the complete rule set and preserves an explicit empty array | 1 | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |

### `src/pages/test/UsagePage.styles.test.ts` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| keeps fixed tariff fields responsive with four labeled decimal inputs | 1 | Retarget: assert retained matrix four-rate decimal editor and its responsive grid. |
| wraps credential default metadata, options, errors and actions on narrow screens | 1 | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |
| wraps channel names, member labels, readback, errors and actions on narrow screens | 1 | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |
| contains wheel scrolling at overflowing card boundaries without trapping short lists | 1 | Preserve: retain request/log/API-key/session containment; remove only retired pricesGrid requirement. |
| keeps Model Pricing Settings list viewport aligned with API Key Settings without shrinking it behind the form | 1 | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |
| reflows the model pricing form from four to two to one column based on its container width | 1 | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |
| keeps pricing help within the viewport and scrollable | 1 | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |

### `src/components/usage/test/PriceSettingsCard.interaction.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| waits for %s persistence before success and locks the active form | 3 | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |
| locks sync drafts while applying and exposes failed models for retry | 1 | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |
| reports unexpected %s failures through the notice callback | 2 | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |

### `src/components/usage/test/PriceSettingsCard.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| renders $style cache read and write prices | 2 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| shows Rules only for saved prices | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| renders saved prices by family priority, descending version and ascending suffix | 1 | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| uses an exact-name tie-break for naturally equivalent saved model names | 1 | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| shows the sync prices action when sync preview is available | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| marks failed sync drafts and keeps them selected for retry | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| notifies when pricing sync throws an unexpected error | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| shows an actionable notice when Models.dev times out | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| shows the concrete backend reason in the top notice when Models.dev prices cannot be applied | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| keeps explicit zero multipliers when converting sync drafts | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| keeps create and edit draft multipliers when converting to saved prices | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| parses OpenAI cache write prices without inferring missing values | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| defaults new sync matches to multiplier 1 and preserves existing model multipliers | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| builds sync save payloads from selected drafts only | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| keeps Models.dev OpenAI cache write through draft and selected-price conversion | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| keeps unconfigured models first and applies the shared ordering within both groups | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| sorts $name independently of the input order | 8 | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| preserves distinct prefixed model identities and their configured status | 1 | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| uses an exact-name tie-break when natural model names compare equally | 1 | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |

### `src/components/usage/hooks/test/usePricingData.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| keeps the loader stable while using the latest auth callback | 1 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| updates only the saved or deleted model and preserves state on failed mutations | 1 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| notifies once per committed save, delete, rules replacement or nonempty sync, never on reads or failures | 1 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| keeps mutation functions stable and uses the latest callback for an in-flight commit | 1 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| does not turn callback %s into a mutation failure or await refresh | 3 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| does not notify for a late legacy %s after disable or unmount | 3 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| does not notify for canceled or mismatched rule writes | 1 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| keeps only the latest model-specific rule %s response | 2 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| persists every model through one atomic batch update | 1 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| reports one atomic batch failure against every submitted model | 1 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| preserves special pricing values for %s | 2 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| defaults invalid multipliers to 1 while preserving explicit zero | 1 | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |

### `src/components/usage/pricing/test/PriceRulesModal.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| treats the empty row as a placeholder and saves an explicit empty set | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| removes the final persisted rule and submits rules: [] only when saved | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| shows row validation and does not call the backend for a partial rule | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| clears a stale duplicate error after resolving it by %s | 2 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| shows the backend rule validation detail in an assertive alert without clearing the draft | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| does not allow an empty placeholder to overwrite rules after loading fails | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| locks closing, inputs and row actions while the complete set is saving | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| keeps late model A reads and saves from overwriting model B | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| exposes a keyboard-focusable help tooltip with only the approved examples | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| keeps the tooltip open while the pointer crosses from the help button into the portal | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| opens the scrollable tooltip from a touch pointer on a short viewport | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| closes a touch-opened tooltip on the second tap even when the button keeps focus | 1 | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |

### `src/components/usage/pricing/test/PriceSyncPanel.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| remembers the chosen source and saves zero cache prices only after confirmation | 1 | Retired global Models.dev/LiteLLM sync dialog; official preview/commit hooks cover source cancellation and explicit commit. |
| aborts the previous source request and ignores its late preview | 1 | Retired global Models.dev/LiteLLM sync dialog; official preview/commit hooks cover source cancellation and explicit commit. |
| restores the last source and names it in timeout feedback | 1 | Retired global Models.dev/LiteLLM sync dialog; official preview/commit hooks cover source cancellation and explicit commit. |
| falls back for an unknown saved source and cancels requests when leaving pricing | 1 | Retired global Models.dev/LiteLLM sync dialog; official preview/commit hooks cover source cancellation and explicit commit. |

### `src/components/usage/pricing/test/PricingChannelModelsCard.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| does not render or fetch management data for a read-only user | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| hides controls on denied initial read %s without upstream bodies | 2 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| selects saved stable channels and exact used/catalog/saved models including absent baselines | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| shows only the saved member count %s and never guesses or manages members | 2 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| has an empty channel state without inventing a channel or enabling writes | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| saves using PUT and canonical GET readback, refreshes and explicitly deletes to inherit | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| saves active valid grammar %s without computing costs | 4 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| rejects invalid draft %s before mutation | 7 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| handles mutation %s error %s safely | 10 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| retains committed %s when GET readback fails | 2 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| disables writes on model GET error %s and refresh can recover | 4 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| saves the complete fixed target only, then uses canonical GET rates and style | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| ignores invalid hidden fixed drafts when switching back to multiplier | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| rejects incomplete or invalid fixed draft %s=%s without a mutation | 32 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| requires explicit supported fallback without a configured-model baseline, and saves all four zeros as active | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| can omit fallback when the configured model has a baseline and does not invent a style | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| handles fixed save error %s without exposing private bodies | 6 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| retains committed fixed canonical state after failed GET readback and refresh recovers | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| includes fixed-only historical identifiers in exact model choices | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| notifies before readback settles and retains the notification if readback is canceled | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| aborts a fixed mutation on unmount and ignores its late successful response | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| reads an active fixed tariff and clears it despite a null multiplier | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| preserves confirmed rates and drafts when refresh GET fails | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| clears canonical data and drafts when committed readback denies access %s | 2 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| switches models without retaining previous fixed drafts or writing to the previous model | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| switches saved channel IDs and resets exceptions, model selection and drafts | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| clears a removed selected channel on refresh and disables mutations | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| aborts late reads on permission revocation and ignores stale response | 1 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |

### `src/components/usage/pricing/test/PricingChannelsCard.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| does not render or fetch management data for read-only access | 1 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| hides all sensitive controls on denied reads %s | 2 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| creates two independent same-provider channels through explicit safe member selection, renames, refreshes and reloads | 1 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| removes members explicitly and requires a nonblank name without guessing memberships | 1 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| saves %s and displays server canonical active multiplier %s | 6 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| rejects invalid multiplier %s without sending a write | 7 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| clears to inheritance and uses real GET readback rather than a local calculation | 1 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| retains committed %s configuration on readback failure | 3 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| handles mutation error %s safely, without publishing a success | 4 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| notifies only for a commit received before %s cancellation | 2 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| sends dependency confirmation only after the explicit deletion dialog is confirmed | 1 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |

### `src/components/usage/pricing/test/PricingCredentialDefaultsCard.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| does not render or fetch management data for a read-only user | 1 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| hides all controls on denied subject-list read %s without showing upstream secrets | 2 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| retries a failed subject-list read and does not invent a configuration | 1 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| handles selected-default load error %s without permitting writes | 4 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| handles %s error %s without exposing secrets or a success notice | 8 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| retains committed %s canonical state when GET readback fails | 2 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| notifies before readback settles and retains the notification if readback is canceled | 1 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| uses the GET readback rather than calculating a local canonical value | 1 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| submits valid text %s and displays canonical %s as active | 11 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| selects same-named credentials by safe subject ID, not raw identity or directory reference | 1 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| keeps canonical %s editable as plain decimal rather than rejected exponent text | 2 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| aborts a pending save on permission revocation and requires fresh selection after permission returns | 1 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| aborts pending default %s on unmount and ignores its late response | 3 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| shows a safe network error without overwriting saved state | 1 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| shows a field error for backend unsafe numeric rejection %s and keeps the last saved value | 2 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| treats %s as active and clears only through explicit DELETE to inheritance | 2 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| rejects invalid text %j without changing the saved override | 19 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| selects a registered subject, saves text, reads the canonical value, and reloads it | 1 | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |

### `src/components/usage/pricing/test/PricingCredentialModelsCard.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| does not render or fetch management data for a read-only user | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| hides controls on denied initial read %s without upstream bodies | 2 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| selects only registered safe credentials and exact used/catalog/saved models including absent baselines | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| saves using PUT and canonical GET readback, refreshes and explicitly deletes to inherit | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| saves active valid grammar %s without computing costs | 4 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| rejects invalid draft %s before mutation | 7 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| handles mutation %s error %s safely | 9 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| retains committed %s when GET readback fails | 2 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| disables writes on model GET error %s and refresh can recover | 4 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| saves the complete fixed target only, then uses canonical GET rates and style | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| ignores invalid hidden fixed drafts when switching back to multiplier | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| rejects incomplete or invalid fixed draft %s=%s without a mutation | 32 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| requires explicit supported fallback without a configured-model baseline, and saves all four zeros as active | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| can omit fallback when the configured model has a baseline and does not invent a style | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| handles fixed save error %s without exposing private bodies | 6 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| retains committed fixed canonical state after failed GET readback and refresh recovers | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| includes fixed-only historical identifiers in exact model choices | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| notifies before readback settles and retains the notification if readback is canceled | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| aborts a fixed mutation on unmount and ignores its late successful response | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| reads an active fixed tariff and clears it despite a null multiplier | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| aborts late reads on permission revocation and ignores stale response | 1 | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |

### `src/components/usage/pricing/test/PricingCredentialsCard.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| renders $title consistently in candidates and saved subjects | 4 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| retains the %s status translation | 4 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| retains the %s binding translation | 5 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| does not expose or fetch the directory for read-only users | 1 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| handles denied directory reads without exposing response secrets or controls | 1 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| shows a safe load failure and can retry through refresh | 1 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| handles save status %s without displaying response secrets or inventing a subject | 3 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| retains a committed subject and disables rebinding when readback fails | 1 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| notifies only for a commit received before %s cancellation | 2 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| does not treat callback %s as a failed registration or wait for refresh | 3 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| aborts the current refresh when unmounted | 1 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| selects an explicit directory reference, submits, and reads the persistent subject | 1 | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |

### `src/components/usage/pricing/test/PricingIdentityMigrationCard.test.tsx` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| does not fetch or render for read-only access | 1 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| hides management data and does not display upstream error text on denied reads %s | 2 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| requires safe explicit subject and directory selection, blocks occupied/shared/unknown, then confirms migration and reads canonical state | 1 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| keeps correction separate, confirms unbind and rebind to a different owner, using opaque references and current expected ownership | 1 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| refreshes a stale/conflicting selection and requires a new confirmation without retry or raw errors | 1 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| retains the committed receipt after readback failure and disables obsolete selections until refresh | 1 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| notifies only for a commit received before %s cancellation | 2 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| discards controls and pending confirmation when permissions change | 1 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |

### `src/components/usage/pricing/test/ruleDraft.test.ts` — grouped definitions

| Original test definition | Expanded cases | Reason / retained invariant |
| --- | ---: | --- |
| normalizes keys and values while omitting a blank default multiplier | 1 | Retired model-rule draft/submission builder; no rule sets in SPEC-003. |
| ignores a completely empty placeholder row | 1 | Retired model-rule draft/submission builder; no rule sets in SPEC-003. |
| reports partially filled and invalid rows without producing a submission | 1 | Retired model-rule draft/submission builder; no rule sets in SPEC-003. |
| detects duplicates after key and value normalization without folding value case | 1 | Retired model-rule draft/submission builder; no rule sets in SPEC-003. |

## Red/green evidence for transferred lifecycle coverage

Commands were run from this worktree with the frontend prefix.

- Before the focused guards: `NODE_OPTIONS=--no-experimental-webstorage npm --prefix 'D:/WILL/AGENT/CPA/wt-keeper-t21/web' test -- src/components/usage/hooks/test/useMatrixPricingData.test.tsx --maxWorkers=2` — **exit 1**, 23 passed, 3 failed. The failures reproduced canceled readback notifications for matrix save, official save and official commit.
- After the three post-readback controller/permission guards: `NODE_OPTIONS=--no-experimental-webstorage npm --prefix 'D:/WILL/AGENT/CPA/wt-keeper-t21/web' test -- src/components/usage/hooks/test/useMatrixPricingData.test.tsx src/pages/test/UsagePage.styles.test.ts src/i18n/test/pricing-rules.test.ts --maxWorkers=2` — **exit 0**, 65 passed.

## Individual baseline test dispositions

Every resolved original test name below has its own reason. Names come from the pre-retirement Vitest JSON inventory; the interaction suite had no reported assertions, so its six parameterized cases are enumerated directly from the reviewed source. Baseline: exit 1, 432/436 reported tests passed; four old sync-panel tests failed because Node experimental localStorage was unavailable. Final gates use `NODE_OPTIONS=--no-experimental-webstorage` and `--maxWorkers=2`.

### `src/i18n/test/pricing-rules.test.ts`

| Original test name | Disposition and reason |
| --- | --- |
| identity migration and correction translations provides separate migration retention and correction ownership/history warnings without fallback | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| credential model exception translations has complete nonempty exact-match, inheritance, baseline and history copy in every language | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| channel model exception translations has complete localized channel, exact-match, precedence, fixed tariff and history copy | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| credential default multiplier translations has complete nonempty copy without fallback in every language | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| default pricing inheritance copy coherence describes every layer, clearing one scope, and restoration only after all new overrides are cleared | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| pricing credential translations has complete selector, save, error, identity and status copy in every language | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| named pricing channel translations provides all management, priority, history and dependency copy in every language | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| pricing rule translations keeps the help copy limited to the two approved examples | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |
| pricing rule translations interpolates the selected count in every language | Remove: obsolete manual identity/channel/default/model-rule/sync management copy expectation; retain dual-cost translation coverage unchanged. |

### `src/lib/test/pricingApi.test.ts`

| Original test name | Disposition and reason |
| --- | --- |
| identity association API client uses exact opaque references, bounded safe payload fields, credentials and cancellation | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| pricing API client requests the selected pricing source undefined with cancellation | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| pricing API client requests the selected pricing source litellm with cancellation | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| pricing API client updates one model through the pricing endpoint without sending a pricing snapshot | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| pricing API client deletes one model through the pricing endpoint without sending a request body | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| pricing API client updates multiple model prices in one batch request | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API loads saved exceptions with encoded stable channel ID, base path and cancellation | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API uses exact model query and canonical response for GET | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API uses exact model query and canonical response for PUT | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API uses exact model query and canonical response for FIXED | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API uses exact model query and canonical response for DELETE | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API preserves status 400 for safe localized handling | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API preserves status 401 for safe localized handling | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API preserves status 403 for safe localized handling | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API preserves status 404 for safe localized handling | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API preserves status 409 for safe localized handling | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API preserves status 422 for safe localized handling | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| channel model pricing API preserves status 500 for safe localized handling | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| pricing rules API loads rules through a query parameter so model names may contain slashes | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |
| pricing rules API replaces the complete rule set and preserves an explicit empty array | Retired HTTP client contract; replaced by retained-client/export-boundary tests in this file. |

### `src/pages/test/UsagePage.styles.test.ts`

| Original test name | Disposition and reason |
| --- | --- |
| UsagePage responsive layout and accessibility keeps fixed tariff fields responsive with four labeled decimal inputs | Retarget: assert retained matrix four-rate decimal editor and its responsive grid. |
| UsagePage responsive layout and accessibility wraps credential default metadata, options, errors and actions on narrow screens | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |
| UsagePage responsive layout and accessibility wraps channel names, member labels, readback, errors and actions on narrow screens | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |
| UsagePage responsive layout and accessibility contains wheel scrolling at overflowing card boundaries without trapping short lists | Preserve: retain request/log/API-key/session containment; remove only retired pricesGrid requirement. |
| UsagePage responsive layout and accessibility keeps Model Pricing Settings list viewport aligned with API Key Settings without shrinking it behind the form | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |
| UsagePage responsive layout and accessibility reflows the model pricing form from four to two to one column based on its container width | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |
| UsagePage responsive layout and accessibility keeps pricing help within the viewport and scrollable | Remove: exclusively retired default/channel/global-price/rule-help subtree layout. |

### `src/components/usage/test/PriceSettingsCard.interaction.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PriceSettingsCard persistence waits for create persistence before success and locks the active form | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |
| PriceSettingsCard persistence waits for edit persistence before success and locks the active form | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |
| PriceSettingsCard persistence waits for delete persistence before success and locks the active form | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |
| PriceSettingsCard persistence locks sync drafts while applying and exposes failed models for retry | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |
| PriceSettingsCard persistence reports unexpected preview failures through the notice callback | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |
| PriceSettingsCard persistence reports unexpected apply failures through the notice callback | Dormant global-price persistence/sync form removed; retained matrix/official editor and hook tests cover pending writes, failures and explicit commit. |

### `src/components/usage/test/PriceSettingsCard.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PriceSettingsCard renders 'claude' cache read and write prices | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard renders 'openai' cache read and write prices | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard shows Rules only for saved prices | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard renders saved prices by family priority, descending version and ascending suffix | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| PriceSettingsCard uses an exact-name tie-break for naturally equivalent saved model names | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| PriceSettingsCard shows the sync prices action when sync preview is available | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard marks failed sync drafts and keeps them selected for retry | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard notifies when pricing sync throws an unexpected error | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard shows an actionable notice when Models.dev times out | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard shows the concrete backend reason in the top notice when Models.dev prices cannot be applied | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard keeps explicit zero multipliers when converting sync drafts | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard keeps create and edit draft multipliers when converting to saved prices | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard parses OpenAI cache write prices without inferring missing values | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard defaults new sync matches to multiplier 1 and preserves existing model multipliers | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard builds sync save payloads from selected drafts only | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| PriceSettingsCard keeps Models.dev OpenAI cache write through draft and selected-price conversion | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| buildPricingModelOptions keeps unconfigured models first and applies the shared ordering within both groups | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| buildPricingModelOptions sorts 'case-insensitive family priority with…' independently of the input order | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| buildPricingModelOptions sorts 'natural versions, base models and nat…' independently of the input order | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| buildPricingModelOptions sorts 'Claude subseries and both dotted and …' independently of the input order | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| buildPricingModelOptions sorts 'dates remain suffixes after their bas…' independently of the input order | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| buildPricingModelOptions sorts 'unknown families and names without ve…' independently of the input order | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| buildPricingModelOptions sorts 'provider-prefixed models mixed with b…' independently of the input order | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| buildPricingModelOptions sorts 'nested and numbered prefixes before v…' independently of the input order | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| buildPricingModelOptions sorts 'full-name tie-breaks between prefixes…' independently of the input order | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |
| buildPricingModelOptions preserves distinct prefixed model identities and their configured status | Dormant global-price editor, multiplier and old sync drafts removed; retained matrix/official editors cover four-price validation and explicit commits. |
| buildPricingModelOptions uses an exact-name tie-break when natural model names compare equally | Retarget: retired price-editor adapter/rendering removed; shared comparator invariant transferred to utils/test/modelSort.test.ts. |

### `src/components/usage/hooks/test/usePricingData.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| usePricingData keeps the loader stable while using the latest auth callback | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData updates only the saved or deleted model and preserves state on failed mutations | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData notifies once per committed save, delete, rules replacement or nonempty sync, never on reads or failures | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData keeps mutation functions stable and uses the latest callback for an in-flight commit | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData does not turn callback throw into a mutation failure or await refresh | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData does not turn callback reject into a mutation failure or await refresh | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData does not turn callback pending into a mutation failure or await refresh | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData does not notify for a late legacy save after disable or unmount | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData does not notify for a late legacy delete after disable or unmount | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData does not notify for a late legacy sync after disable or unmount | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData does not notify for canceled or mismatched rule writes | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData keeps only the latest model-specific rule read response | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| usePricingData keeps only the latest model-specific rule write response | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| persistModelPriceEntries persists every model through one atomic batch update | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| persistModelPriceEntries reports one atomic batch failure against every submitted model | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| persistModelPriceEntries preserves special pricing values for free-model | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| persistModelPriceEntries preserves special pricing values for gpt-5.6-terra | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |
| pricingToModelPrice defaults invalid multipliers to 1 while preserving explicit zero | Dormant global-price/rules/sync hook removed; valid lifecycle and security invariants transferred to useMatrixPricingData.test.tsx. |

### `src/components/usage/pricing/test/PriceRulesModal.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PriceRulesModal treats the empty row as a placeholder and saves an explicit empty set | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal removes the final persisted rule and submits rules: [] only when saved | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal shows row validation and does not call the backend for a partial rule | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal clears a stale duplicate error after resolving it by edit | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal clears a stale duplicate error after resolving it by remove | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal shows the backend rule validation detail in an assertive alert without clearing the draft | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal does not allow an empty placeholder to overwrite rules after loading fails | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal locks closing, inputs and row actions while the complete set is saving | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal keeps late model A reads and saves from overwriting model B | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal exposes a keyboard-focusable help tooltip with only the approved examples | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal keeps the tooltip open while the pointer crosses from the help button into the portal | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal opens the scrollable tooltip from a touch pointer on a short viewport | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |
| PriceRulesModal closes a touch-opened tooltip on the second tap even when the button keeps focus | Retired per-model rules editor/help; no rule replacement in SPEC-003. Generic stale/canceled requests and redaction covered by new hooks. |

### `src/components/usage/pricing/test/PriceSyncPanel.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| pricing sync source selection remembers the chosen source and saves zero cache prices only after confirmation | Retired global Models.dev/LiteLLM sync dialog; official preview/commit hooks cover source cancellation and explicit commit. |
| pricing sync source selection aborts the previous source request and ignores its late preview | Retired global Models.dev/LiteLLM sync dialog; official preview/commit hooks cover source cancellation and explicit commit. |
| pricing sync source selection restores the last source and names it in timeout feedback | Retired global Models.dev/LiteLLM sync dialog; official preview/commit hooks cover source cancellation and explicit commit. |
| pricing sync source selection falls back for an unknown saved source and cancels requests when leaving pricing | Retired global Models.dev/LiteLLM sync dialog; official preview/commit hooks cover source cancellation and explicit commit. |

### `src/components/usage/pricing/test/PricingChannelModelsCard.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PricingChannelModelsCard does not render or fetch management data for a read-only user | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard hides controls on denied initial read 401 without upstream bodies | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard hides controls on denied initial read 403 without upstream bodies | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard selects saved stable channels and exact used/catalog/saved models including absent baselines | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard shows only the saved member count 0 and never guesses or manages members | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard shows only the saved member count 2 and never guesses or manages members | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard has an empty channel state without inventing a channel or enabling writes | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard saves using PUT and canonical GET readback, refreshes and explicitly deletes to inherit | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard saves active valid grammar 0 without computing costs | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard saves active valid grammar 1 without computing costs | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard saves active valid grammar .2X without computing costs | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard saves active valid grammar 120% without computing costs | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects invalid draft  before mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects invalid draft -1 before mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects invalid draft +1 before mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects invalid draft 1e2 before mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects invalid draft 20%x before mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects invalid draft NaN before mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects invalid draft Infinity before mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation PUT error 400 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation PUT error 422 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation PUT error 409 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation PUT error 500 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation PUT error 401 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation PUT error 403 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation DELETE error 409 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation DELETE error 500 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation DELETE error 401 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles mutation DELETE error 403 safely | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard retains committed PUT when GET readback fails | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard retains committed DELETE when GET readback fails | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard disables writes on model GET error 401 and refresh can recover | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard disables writes on model GET error 403 and refresh can recover | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard disables writes on model GET error 409 and refresh can recover | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard disables writes on model GET error 500 and refresh can recover | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard saves the complete fixed target only, then uses canonical GET rates and style | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard ignores invalid hidden fixed drafts when switching back to multiplier | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m= without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=-1 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=NaN without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=Infinity without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=1e2 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=2x without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=20% without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=+1 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m= without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=-1 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=NaN without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=Infinity without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=1e2 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=2x without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=20% without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=+1 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m= without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=-1 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=NaN without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=Infinity without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=1e2 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=2x without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=20% without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=+1 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m= without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=-1 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=NaN without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=Infinity without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=1e2 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=2x without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=20% without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=+1 without a mutation | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard requires explicit supported fallback without a configured-model baseline, and saves all four zeros as active | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard can omit fallback when the configured model has a baseline and does not invent a style | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles fixed save error 400 without exposing private bodies | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles fixed save error 422 without exposing private bodies | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles fixed save error 409 without exposing private bodies | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles fixed save error 500 without exposing private bodies | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles fixed save error 401 without exposing private bodies | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard handles fixed save error 403 without exposing private bodies | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard retains committed fixed canonical state after failed GET readback and refresh recovers | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard includes fixed-only historical identifiers in exact model choices | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard notifies before readback settles and retains the notification if readback is canceled | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard aborts a fixed mutation on unmount and ignores its late successful response | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard reads an active fixed tariff and clears it despite a null multiplier | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard preserves confirmed rates and drafts when refresh GET fails | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard clears canonical data and drafts when committed readback denies access 401 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard clears canonical data and drafts when committed readback denies access 403 | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard switches models without retaining previous fixed drafts or writing to the previous model | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard switches saved channel IDs and resets exceptions, model selection and drafts | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard clears a removed selected channel on refresh and disables mutations | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |
| PricingChannelModelsCard aborts late reads on permission revocation and ignores stale response | Retired channel model multiplier/fixed exceptions; channels are not a SPEC-003 layer. Matrix tests retain valid four-price/zero invariants. |

### `src/components/usage/pricing/test/PricingChannelsCard.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PricingChannelsCard does not render or fetch management data for read-only access | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard hides all sensitive controls on denied reads 401 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard hides all sensitive controls on denied reads 403 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard creates two independent same-provider channels through explicit safe member selection, renames, refreshes and reloads | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard removes members explicitly and requires a nonblank name without guessing memberships | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard saves 0.2 and displays server canonical active multiplier 0.2 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard saves 0.2x and displays server canonical active multiplier 0.2 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard saves 20% and displays server canonical active multiplier 0.2 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard saves 0 and displays server canonical active multiplier 0 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard saves 1X and displays server canonical active multiplier 1 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard saves 120% and displays server canonical active multiplier 1.2 | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard rejects invalid multiplier  without sending a write | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard rejects invalid multiplier -1 without sending a write | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard rejects invalid multiplier +1 without sending a write | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard rejects invalid multiplier 1e2 without sending a write | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard rejects invalid multiplier 20%x without sending a write | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard rejects invalid multiplier NaN without sending a write | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard rejects invalid multiplier Infinity without sending a write | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard clears to inheritance and uses real GET readback rather than a local calculation | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard retains committed channel configuration on readback failure | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard retains committed default configuration on readback failure | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard retains committed clear configuration on readback failure | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard handles mutation error 409 safely, without publishing a success | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard handles mutation error 500 safely, without publishing a success | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard handles mutation error 401 safely, without publishing a success | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard handles mutation error 403 safely, without publishing a success | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard notifies only for a commit received before mutation cancellation | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard notifies only for a commit received before readback cancellation | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |
| PricingChannelsCard sends dependency confirmation only after the explicit deletion dialog is confirmed | Retired named channels, membership and channel-default multiplier layer; no replacement management UI. |

### `src/components/usage/pricing/test/PricingCredentialDefaultsCard.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PricingCredentialDefaultsCard does not render or fetch management data for a read-only user | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard hides all controls on denied subject-list read 401 without showing upstream secrets | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard hides all controls on denied subject-list read 403 without showing upstream secrets | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard retries a failed subject-list read and does not invent a configuration | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles selected-default load error 401 without permitting writes | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles selected-default load error 403 without permitting writes | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles selected-default load error 409 without permitting writes | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles selected-default load error 500 without permitting writes | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles PUT error 409 without exposing secrets or a success notice | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles PUT error 500 without exposing secrets or a success notice | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles PUT error 401 without exposing secrets or a success notice | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles PUT error 403 without exposing secrets or a success notice | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles DELETE error 409 without exposing secrets or a success notice | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles DELETE error 500 without exposing secrets or a success notice | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles DELETE error 401 without exposing secrets or a success notice | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard handles DELETE error 403 without exposing secrets or a success notice | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard retains committed PUT canonical state when GET readback fails | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard retains committed DELETE canonical state when GET readback fails | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard notifies before readback settles and retains the notification if readback is canceled | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard uses the GET readback rather than calculating a local canonical value | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 0.2 and displays canonical 0.2 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 0.2x and displays canonical 0.2 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 0.2X and displays canonical 0.2 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 20% and displays canonical 0.2 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 1.2x and displays canonical 1.2 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 120% and displays canonical 1.2 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text .2 and displays canonical 0.2 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 1. and displays canonical 1 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 0x and displays canonical 0 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 0% and displays canonical 0 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard submits valid text 1X and displays canonical 1 as active | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard selects same-named credentials by safe subject ID, not raw identity or directory reference | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard keeps canonical 0.0000001 editable as plain decimal rather than rejected exponent text | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard keeps canonical 1000000000000000000000 editable as plain decimal rather than rejected exponent text | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard aborts a pending save on permission revocation and requires fresh selection after permission returns | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard aborts pending default GET on unmount and ignores its late response | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard aborts pending default PUT on unmount and ignores its late response | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard aborts pending default DELETE on unmount and ignores its late response | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard shows a safe network error without overwriting saved state | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard shows a field error for backend unsafe numeric rejection 400 and keeps the last saved value | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard shows a field error for backend unsafe numeric rejection 422 and keeps the last saved value | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard treats 0 as active and clears only through explicit DELETE to inheritance | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard treats 1 as active and clears only through explicit DELETE to inheritance | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "   " without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "-1" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "-0" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "+1" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "NaN" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "Infinity" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "Inf" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "1e3" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "0x10" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "20%x" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "0.2x%" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "0.2xx" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "1 x" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "1 .2" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "abc" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "1,2" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "." without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard rejects invalid text "9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999" without changing the saved override | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |
| PricingCredentialDefaultsCard selects a registered subject, saves text, reads the canonical value, and reloads it | Retired credential-wide multiplier/default layer; sparse matrix uses fixed/inherit, not multiplier grammar. |

### `src/components/usage/pricing/test/PricingCredentialModelsCard.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PricingCredentialModelsCard does not render or fetch management data for a read-only user | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard hides controls on denied initial read 401 without upstream bodies | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard hides controls on denied initial read 403 without upstream bodies | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard selects only registered safe credentials and exact used/catalog/saved models including absent baselines | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard saves using PUT and canonical GET readback, refreshes and explicitly deletes to inherit | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard saves active valid grammar 0 without computing costs | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard saves active valid grammar 1 without computing costs | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard saves active valid grammar .2X without computing costs | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard saves active valid grammar 120% without computing costs | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects invalid draft  before mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects invalid draft -1 before mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects invalid draft +1 before mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects invalid draft 1e2 before mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects invalid draft 20%x before mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects invalid draft NaN before mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects invalid draft Infinity before mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation PUT error 400 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation PUT error 409 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation PUT error 500 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation PUT error 401 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation PUT error 403 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation DELETE error 409 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation DELETE error 500 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation DELETE error 401 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles mutation DELETE error 403 safely | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard retains committed PUT when GET readback fails | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard retains committed DELETE when GET readback fails | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard disables writes on model GET error 401 and refresh can recover | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard disables writes on model GET error 403 and refresh can recover | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard disables writes on model GET error 409 and refresh can recover | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard disables writes on model GET error 500 and refresh can recover | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard saves the complete fixed target only, then uses canonical GET rates and style | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard ignores invalid hidden fixed drafts when switching back to multiplier | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m= without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=-1 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=NaN without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=Infinity without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=1e2 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=2x without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=20% without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft prompt_price_per_1m=+1 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m= without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=-1 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=NaN without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=Infinity without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=1e2 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=2x without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=20% without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft completion_price_per_1m=+1 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m= without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=-1 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=NaN without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=Infinity without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=1e2 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=2x without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=20% without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_read_price_per_1m=+1 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m= without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=-1 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=NaN without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=Infinity without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=1e2 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=2x without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=20% without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard rejects incomplete or invalid fixed draft cache_write_price_per_1m=+1 without a mutation | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard requires explicit supported fallback without a configured-model baseline, and saves all four zeros as active | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard can omit fallback when the configured model has a baseline and does not invent a style | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles fixed save error 400 without exposing private bodies | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles fixed save error 422 without exposing private bodies | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles fixed save error 409 without exposing private bodies | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles fixed save error 500 without exposing private bodies | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles fixed save error 401 without exposing private bodies | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard handles fixed save error 403 without exposing private bodies | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard retains committed fixed canonical state after failed GET readback and refresh recovers | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard includes fixed-only historical identifiers in exact model choices | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard notifies before readback settles and retains the notification if readback is canceled | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard aborts a fixed mutation on unmount and ignores its late successful response | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard reads an active fixed tariff and clears it despite a null multiplier | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |
| PricingCredentialModelsCard aborts late reads on permission revocation and ignores stale response | Retired credential model multiplier/fixed exception endpoint; retained matrix tests cover four complete prices, zeros and inheritance. |

### `src/components/usage/pricing/test/PricingCredentialsCard.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PricingCredentialsCard renders 'apikey alias instead of name or key h…' consistently in candidates and saved subjects | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard renders 'apikey key hint when there is no alias' consistently in candidates and saved subjects | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard renders 'OAuth account name without alias, key…' consistently in candidates and saved subjects | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard renders 'empty optional fields without empty s…' consistently in candidates and saved subjects | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the active status translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the disabled status translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the stale status translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the unknown status translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the unbound binding translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the bound binding translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the stale binding translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the unknown binding translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains the ambiguous binding translation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard does not expose or fetch the directory for read-only users | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard handles denied directory reads without exposing response secrets or controls | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard shows a safe load failure and can retry through refresh | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard handles save status 409 without displaying response secrets or inventing a subject | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard handles save status 500 without displaying response secrets or inventing a subject | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard handles save status 403 without displaying response secrets or inventing a subject | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard retains a committed subject and disables rebinding when readback fails | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard notifies only for a commit received before mutation cancellation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard notifies only for a commit received before readback cancellation | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard does not treat callback throw as a failed registration or wait for refresh | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard does not treat callback reject as a failed registration or wait for refresh | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard does not treat callback pending as a failed registration or wait for refresh | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard aborts the current refresh when unmounted | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |
| PricingCredentialsCard selects an explicit directory reference, submits, and reads the persistent subject | Retired manual registration/binding card; automatic credential refresh remains, with permission/cancellation/redaction tested by new hooks. |

### `src/components/usage/pricing/test/PricingIdentityMigrationCard.test.tsx`

| Original test name | Disposition and reason |
| --- | --- |
| PricingIdentityMigrationCard does not fetch or render for read-only access | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard hides management data and does not display upstream error text on denied reads 401 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard hides management data and does not display upstream error text on denied reads 403 | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard requires safe explicit subject and directory selection, blocks occupied/shared/unknown, then confirms migration and reads canonical state | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard keeps correction separate, confirms unbind and rebind to a different owner, using opaque references and current expected ownership | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard refreshes a stale/conflicting selection and requires a new confirmation without retry or raw errors | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard retains the committed receipt after readback failure and disables obsolete selections until refresh | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard notifies only for a commit received before mutation cancellation | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard notifies only for a commit received before readback cancellation | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |
| PricingIdentityMigrationCard discards controls and pending confirmation when permissions change | Retired identity migration/correction management card; snapshots and dual costs remain unchanged. |

### `src/components/usage/pricing/test/ruleDraft.test.ts`

| Original test name | Disposition and reason |
| --- | --- |
| buildPricingRuleSubmission normalizes keys and values while omitting a blank default multiplier | Retired model-rule draft/submission builder; no rule sets in SPEC-003. |
| buildPricingRuleSubmission ignores a completely empty placeholder row | Retired model-rule draft/submission builder; no rule sets in SPEC-003. |
| buildPricingRuleSubmission reports partially filled and invalid rows without producing a submission | Retired model-rule draft/submission builder; no rule sets in SPEC-003. |
| buildPricingRuleSubmission detects duplicates after key and value normalization without folding value case | Retired model-rule draft/submission builder; no rule sets in SPEC-003. |

Total individual retired/replaced/retargeted entries: **405**. All other mixed-suite cases are preserved.

## Final frontend validation

- `NODE_OPTIONS=--no-experimental-webstorage npm --prefix 'D:/WILL/AGENT/CPA/wt-keeper-t21/web' test -- src/lib/test/pricingApi.test.ts src/lib/test/pricingMatrixApi.test.ts --maxWorkers=2` — **exit 0**, 35 passed.
- `NODE_OPTIONS=--no-experimental-webstorage npm --prefix 'D:/WILL/AGENT/CPA/wt-keeper-t21/web' test -- src/components/usage/hooks/test/useMatrixPricingData.test.tsx src/utils/test/modelSort.test.ts --maxWorkers=2` — **exit 0**, 35 passed.
- `npm --prefix 'D:/WILL/AGENT/CPA/wt-keeper-t21/web' run lint` — **exit 0** after correcting the new test harness to capture results in effects.
- `npm --prefix 'D:/WILL/AGENT/CPA/wt-keeper-t21/web' run typecheck` — **exit 0**.
- Retired route invocations/imports: no matches across `web/`; retired names occur only in absence guards. Temporary test inventory/generation scripts removed. No commits, backend writes or runtime changes.
- `NODE_OPTIONS=--no-experimental-webstorage npm --prefix 'D:/WILL/AGENT/CPA/wt-keeper-t21/web' test -- --maxWorkers=2` — **exit 0**, **183 files / 1466 tests passed**, with no excluded suites.
