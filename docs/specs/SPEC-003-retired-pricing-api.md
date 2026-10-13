# SPEC-003 retired pricing management API (#21)

## Contract and boundary

SPEC-003 (#14), implementation decisions §4–5 (removed pricing layers and UI/API), §7 (API/test retirement), and T26/T30 require removed management features to stop accepting requests without changing retained data. Issue #21 explicitly includes old global model-price writes, rules, identity correction, defaults, channels, and model exceptions.

After the existing request-intent and administrator-authentication middleware, every route below returns HTTP **410 Gone** with the fixed JSON body:

```json
{"code":"pricing_feature_removed","error":"pricing_feature_removed"}
```

The handler does not read the request body, query, path parameters, provider, database, or snapshot, and does not log request data. Invalid, oversized, and secret-shaped inputs have the same response as formerly valid input. Existing authentication/role and request-intent failures still take precedence. No wildcard or global `/pricing` middleware is used: the new routes remain independently registered.

All paths below include the `/api/v1` prefix (an optional configured application base path still applies).

## Retired routes and basis

| Method | Path | Basis |
| --- | --- | --- |
| PUT | `/api/v1/pricing` | #21: old global model-price write; #14 §1/§4: old multiplier metadata is inactive |
| PUT | `/api/v1/pricing/batch` | Same, batch write |
| PUT | `/api/v1/pricing/:model` | Same, path-model write |
| DELETE | `/api/v1/pricing` | Same, old global price deletion |
| GET | `/api/v1/pricing/sync/preview` | #14 §3/§7: superseded by official refresh preview/commit, not the old Models.dev/LiteLLM sync workflow |
| GET | `/api/v1/pricing/rules` | #14 §1/§4/§5: removed model rules; #21 rules scope |
| PUT | `/api/v1/pricing/rules` | Same, rule replacement |
| POST | `/api/v1/pricing/credentials` | #14 automatic credential import, §6: no manual identity binding; directory GET and sync refresh remain |
| GET | `/api/v1/pricing/credentials/:subjectID/default` | #14 §4/§5/T26: removed credential-wide default multiplier |
| PUT | `/api/v1/pricing/credentials/:subjectID/default` | Same, default write |
| DELETE | `/api/v1/pricing/credentials/:subjectID/default` | Same, default clearing |
| GET | `/api/v1/pricing/channels` | #14 §4/§5/T26: removed named channels/membership |
| POST | `/api/v1/pricing/channels` | Same, channel/member creation |
| GET | `/api/v1/pricing/channels/:channelID` | Same, channel/member read |
| PUT | `/api/v1/pricing/channels/:channelID` | Same, channel/member update |
| DELETE | `/api/v1/pricing/channels/:channelID` | Same, channel/member deletion |
| GET | `/api/v1/pricing/channels/:channelID/default` | #14 §4/§5/T26: removed channel default multiplier |
| PUT | `/api/v1/pricing/channels/:channelID/default` | Same, default write |
| DELETE | `/api/v1/pricing/channels/:channelID/default` | Same, default clearing |
| GET | `/api/v1/pricing/credentials/:subjectID/models` | #14 §4/§5/T26: removed credential model exceptions, replaced by sparse matrix |
| GET | `/api/v1/pricing/credentials/:subjectID/model` | Same, individual exception read |
| PUT | `/api/v1/pricing/credentials/:subjectID/model` | Same, multiplier/fixed exception write |
| DELETE | `/api/v1/pricing/credentials/:subjectID/model` | Same, exception clearing |
| GET | `/api/v1/pricing/channels/:channelID/models` | #14 §4/§5/T26: removed channel model exceptions |
| GET | `/api/v1/pricing/channels/:channelID/model` | Same, individual exception read |
| PUT | `/api/v1/pricing/channels/:channelID/model` | Same, multiplier/fixed exception write |
| DELETE | `/api/v1/pricing/channels/:channelID/model` | Same, exception clearing |
| GET | `/api/v1/pricing/identity-bindings` | #14 §5/§6/T26: removed identity migration/correction management view |
| POST | `/api/v1/pricing/identity-bindings/migrate` | Same, identity migration |
| PUT | `/api/v1/pricing/identity-bindings/:bindingRef/correction` | Same, identity unbind/rebind correction |

### Scope interpretation

Although #21's minimum acceptance emphasizes writes, removed feature-specific GET routes are also retired under #14 §5's old-management-interface decision. Old sync preview and manual credential registration are retired rather than left as apparently supported clients of an obsolete workflow. This does **not** retire `GET /pricing` (read-only compatibility), `GET /models/used`, credential-directory/subject GETs, credential sync status/refresh, either matrix alias and batch route, official prices and refresh preview/commit, key-viewer matrix, usage queries, or export.

No HTTP migration-batch report route existed at the baseline. Existing internal report/list service and `cmd/pricing-migrate` read-only report/list operations remain unchanged. Identity-bindings GET is a retired management view, not a SPEC-003 migration-batch report.

Legacy tables, service operations and rollback resolver machinery are retained for explicit migration/rollback, pre-adoption compatibility, and isolated fixtures. The baseline snapshot loader intentionally supports an unadopted database or a selected frozen rollback snapshot; this ticket does not change that behavior or force adoption. Strict mode uses only matrix > official/OpenRouter > unavailable. This change is an HTTP management boundary, not deletion of migration evidence or a redesign of pricing.

## Deployment limitations

Source changes only: no service deployment, runtime database adoption, CPA configuration change, or contact with running services. Existing instances do not acquire this contract until separately deployed. Internal migration/rollback can still read retained legacy facts by design; strict pricing remains matrix > official/OpenRouter > unavailable. Authentication-disabled deployments continue to have their existing administrator-equivalent access semantics.
