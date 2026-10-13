# SPEC-003 #21 verification

Baseline: `main@635c4753b6727e09aae0f3930986c8b4ae61ea8c`; worktree branch `feat/spec003-t21`.

## Six mandatory gates — actual exit codes

All were executed from the isolated `wt-keeper-t21` worktree, without a pipeline that hides an exit code. Each PowerShell command captured `$LASTEXITCODE` immediately, printed it, and exited with it.

| Command | Actual exit | Result |
| --- | --- | --- |
| `go test ./cmd/... ./internal/...` | **0** | All Go packages passed; repeated after final test-spy readability cleanup, also 0. |
| `npm --prefix ./web ci` | **0** | Locked dependencies installed. |
| `NODE_OPTIONS=--no-experimental-webstorage npm --prefix ./web run test -- --maxWorkers=2` | **0** | 183 files, 1466 tests passed; no exclusions. |
| `npm --prefix ./web run lint` | **0** | ESLint passed. |
| `npm --prefix ./web run typecheck` | **0** | TypeScript passed. |
| `npm --prefix ./web run build` | **0** | Vite production build passed. |

The frontend test command uses POSIX environment notation above; the actual Windows invocation was:

```powershell
$env:NODE_OPTIONS='--no-experimental-webstorage'; npm --prefix ./web run test -- --maxWorkers=2; $code = $LASTEXITCODE; Write-Output "GATE frontend test EXIT=$code"; exit $code
```

## Additional evidence and truthful failures

- Initial all-30-route retirement regression against original handlers: **exit 1**. The same regression after replacement handlers: **exit 0**.
- Initial focused service/resolver/migration suite: **exit 1**, solely because `TestPricingCredentialDirectoryRejectsSharedUpstreamConfiguration` still expected manual bind HTTP 409 and received the intended 410. Rewrite preserves the ambiguity assertion at the service seam. Focused ambiguity test: **exit 0**; complete focused service/resolver/migration rerun: **exit 0**.
- Final `go test ./internal/api/test -count=1`, `go test ./internal/api/... -count=1`, `go vet ./internal/api/...`: **exit 0**.
- Transferred frontend hook regressions reproduced three canceled mutation-readback races: **exit 1** (23 pass/3 fail). Three post-readback identity/permission guards fixed these; mixed hook/styles/i18n rerun: **exit 0** (65 pass). The frontend disposition document records exact reproduction commands.
- Frontend retirement inventory initially ran without the required Node option and returned **exit 1**, including four old sync-panel localStorage failures. This was inventory evidence, not a successful gate. Final complete required-option run passed.
- Final frontend retained API targeted tests: **exit 0** (35 pass); hook/shared-ordering targeted tests: **exit 0** (35 pass).
- Final `git diff --check`: **exit 0**. Independent static production-boundary and focused regression review found no concrete issues.

## Non-fatal output and limitations

- `npm ci` warned that two dependency install scripts were not covered by npm allowScripts. No permission/configuration bypass was made; all verification gates still passed.
- The complete frontend run emitted `socket hang up`/`ECONNRESET` stderr in existing page runtime/tab tests; those tests passed and Vitest exited 0. These messages are not reported as a silent clean log.
- Vite emitted plugin-timing and >500 kB chunk warnings. This ticket does not redesign bundling or relax its warning threshold.
- Build output is ignored; tracked `web/dist/.gitkeep` was restored after Vite removed it. No generated assets, debug scripts, or temporary inventories are committed.
- Tests use isolated temporary SQLite databases, synthetic identities and stub/fake upstreams; no running CPA/Keeper service, runtime database, other project, or port 8318/8317 was intentionally contacted or modified.
- No deployment/adoption is performed. Existing runtime services require a separately authorized deployment to acquire 410 responses.
- Legacy pre-adoption/frozen rollback snapshot semantics remain by design; this ticket neither forces strict adoption nor removes migration/rollback support. Strict snapshots continue matrix > official/OpenRouter > unavailable.
- Existing request-intent/authentication/role middleware retains precedence over 410. The new handler never decodes/logs the rejected input and cannot call pricing services.
