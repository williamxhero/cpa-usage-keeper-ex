# SPEC-003 #21 service test disposition

## Rewritten test

| Test | Obsolete assertion | Replacement and preserved assertions |
| --- | --- | --- |
| `internal/service/test/pricing_credentials_test.go::TestPricingCredentialDirectoryRejectsSharedUpstreamConfiguration` | Expected POST `/api/v1/pricing/credentials` to dispatch manual binding and return 409. The manual-registration HTTP route now returns 410 for every administrator request. | Call the retained `BindPricingCredential` service seam and require `ErrCredentialNotSelectable`. Keep the exact original directory assertions: duplicate shared upstream configuration is ambiguous and has no subject ID. HTTP 410/auth/no-write behavior is covered by the API route table. |

The focused suite initially returned exit **1**, solely because this test expected 409 and observed the correct fixed 410 body. After the seam rewrite, `go test ./internal/service/test -run '^TestPricingCredentialDirectoryRejectsSharedUpstreamConfiguration$' -count=1` returned **0**.

## Retained tests and rationale

No service test was deleted or skipped. All other service test assertions remain unchanged, including:

- Legacy writer persistence, four-rate fidelity, explicit-zero multiplier validation, and unknown-style validation: the retained source facts and service operations still serve compatibility fixtures and migration/rollback.
- Legacy default/channel/model/identity precedence and restoration tests: these exercise the deliberately retained pre-adoption/frozen rollback snapshot, not strict SPEC-003 pricing. HTTP feature retirement must not erase rollback coverage.
- `pricing_catalog_mutation_test.go`: immutable snapshot reads, candidate/commit failure, complete batch publication, and concurrent writer consistency remain valid infrastructure behavior.
- `usage_dual_costs_test.go`: configured/reference independence, all four token buckets, incomplete/known-zero behavior, overview/analysis/events/export/realtime compatibility and pinned snapshots are not obsolete management contracts.
- Credential discovery, exact identity/ambiguity, metadata atomicity, sanitized labels/key hints, and stale-safe behavior: these still protect current matrix ownership and safe directory reads.
- `pricing_credential_matrix_test.go`, `spec003_integration_test.go`, official pricing and refresh tests: current sparse matrix, strict matrix > official/OpenRouter > unavailable, legacy isolation, four-price validation, atomicity, restart, and independent reference behavior remain unchanged.
- Migration/report/rollback/restore tests, including deferred commit failure after candidate compilation: retained legacy rows are intentional migration evidence; these tests must not be removed.
- Old metadata preview matching tests remain while their internal implementation is retained; they are not evidence that the retired HTTP preview route or permissive matching policy is current. New official deterministic matching tests remain unchanged.

Repository migration tests and both strict and compatibility resolver tests likewise remain unchanged. No production service/resolver/repository behavior is modified by this ticket.
