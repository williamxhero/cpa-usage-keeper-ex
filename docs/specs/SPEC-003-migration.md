# SPEC-003 pricing migration

This document describes the explicit legacy pricing adoption tool delivered for SPEC-003. Keeper startup creates or upgrades the pricing schema, but it does not adopt legacy prices automatically. An administrator must run the migration command against the Keeper SQLite file.

## Command

Run the command from the repository checkout or from a built deployment containing the command package:

```text
go run ./cmd/pricing-migrate -db <existing SQLite file> -action run -batch <batch-id> -batch-size 250
```

The database path is explicit. The command does not load configuration files, environment variables, CPA credentials, network clients, or the server application. The file must already exist and be a regular local SQLite file. SQLite URI paths, query parameters, memory databases, directories, and missing files are rejected. GORM logging is silent.

`batch-id` is an administrator-chosen audit identifier containing 1 to 96 ASCII letters, digits, `_`, or `-`, beginning with a letter or digit. Do not put secrets, credentials, URLs, or filesystem paths in it. `batch-size` defaults to 250 and is bounded to 1 through 1000. It controls the staged input page size and is useful when a large database must be resumed in bounded work units.

The command emits a safe JSON projection containing the batch status, aggregate counts, opaque source references, outcomes, and allowlisted reasons. It never emits source keys, endpoints, credentials, frozen snapshots, private staged input, or database paths. Errors are intentionally generic. The process returns `0` when the selected operation succeeds, `1` for a database or migration failure, and `2` for invalid arguments. A failing `run` can emit its persisted JSON failure report on stdout while returning `1` and a sanitized explanation on stderr. Inspect the batch status rather than treating the presence of JSON as success. Reading an existing failed batch with `report` returns `0` because the report query itself succeeded.

## Actions

Run adoption explicitly:

```text
go run ./cmd/pricing-migrate -db ./data/app.db -action run -batch spec003-20261013 -batch-size 250
```

A successful run freezes pricing-only source facts, stages them in bounded pages, validates the complete candidate, writes the strict official and credential matrix facts, and publishes strict pricing only after the final transaction commits. Legacy tables remain retained for audit and rollback. Reusing a successful or rolled-back batch ID is idempotent and does not create a new catalog publication. Use `restore` to reactivate a rolled-back batch.

| Source | Adoption behavior |
| --- | --- |
| Legacy model prices | Seed the official catalog without overwriting an existing target tariff. |
| Complete credential model fixed tariffs | Copy the four explicit rates and pricing style into the credential model matrix. |
| Credential model multipliers | Materialize four rates against a complete matching official price; missing official prices or segments are reported as skipped. |
| Credential defaults, channels, channel prices, association overlays, model rules | Retain and report inactive; strict pricing excludes these legacy layers. |

Existing target tariffs that differ from a proposed import are preserved and reported as conflicts. Conflicts are auditable and do not themselves abort adoption. Invalid source facts or a candidate that cannot compile fail the batch; the resolver state and target facts remain unchanged.

### Canonical duplicates (issue #31)

Migration identity follows the existing trim-and-lowercase canonical model rule; it does not strip arbitrary provider prefixes. Source rows are processed in the existing deterministic order (`model, id` for legacy model prices; `subject_id, model` for credential prices), across all staged pages. When no target exists, the first valid row establishes the target. Later equal tariffs are reported as skipped (`target_exists`), and differing tariffs as conflicts (`target_conflict`), without overwriting that target. Duplicate identity never replaces a genuinely invalid source's rejection reason.

Official rows, including startup `source='legacy-seed'` rows, are existing targets and multiplier bases, not additional migration sources. Their presence must not synthesize a legacy model-price rejection when source tables are empty. Migration preserves all pre-existing official rows and their values rather than deleting or merging administrator-visible facts. A proposed source is skipped only if every existing row with that normalized canonical ID has the same tariff; any differing row makes it a conflict. A multiplier depending on differing existing official tariffs is also reported as a conflict and is not materialized against an arbitrarily selected base. If a differing existing canonical group has no legacy model-price source, one conflict-only catalog audit item records that group; it never imports an official seed as a source. Equal seed duplicates with empty source tables produce no items.

For migration's frozen rollback snapshot only, trim-equivalent legacy model and credential rows are normalized to the same deterministic first-row policy. Every discarded row is still independently validated, including its model rules; ordinary snapshot compilation remains unchanged. This permits migration and rollback even when raw whitespace aliases made the old legacy snapshot uncompilable. Original source rows remain retained. A batch frozen by an older tool may retain its synthetic rejection; use a new batch ID rather than rewriting its frozen audit input.

This repair deliberately does not change startup seeding, official API behavior, or runtime matching priorities. Retained duplicate official rows can therefore still produce the resolver's existing `ambiguous_match`; migration success is not a promise to resolve that ambiguity. Malformed existing credential matrix duplicates that cannot compile remain a candidate failure. Successful batch IDs remain idempotent, and a fresh migration over adopted facts preserves target rows and pricing mode (while recording a new audit batch and migration ownership).

Read one report:

```text
go run ./cmd/pricing-migrate -db ./data/app.db -action report -batch spec003-20261013
```

List migration batches:

```text
go run ./cmd/pricing-migrate -db ./data/app.db -action list
```

Switch a successful migration to its frozen previous pricing state:

```text
go run ./cmd/pricing-migrate -db ./data/app.db -action rollback -batch spec003-20261013
```

Restore the current strict target facts:

```text
go run ./cmd/pricing-migrate -db ./data/app.db -action restore -batch spec003-20261013
```

Rollback and restore are durable, audited state changes. Repeating a terminal operation is idempotent and does not publish a fresh catalog snapshot. A rollback is allowed only for a successful migration whose retained previous state is valid. Restore uses the current strict target facts and therefore preserves administrator edits made after adoption. An ordinary administrator strict-mode change invalidates stale migration rollback state; run a new explicit migration when a new legacy adoption is required.

## Schema and deployment behavior

The command may create only the four migration report tables when they are absent:

- `pricing_migration_batches`
- `pricing_migration_batch_counts`
- `pricing_migration_items`
- `pricing_migration_legacy_states`

It checks the required existing pricing schema before a mutating action. It does not run the full application startup path and does not run broad `AutoMigrate` over business tables. Complete the normal Keeper schema upgrade before using this command. `report` and `list` open the existing file read-only and require the migration report schema to exist.

An online Keeper process publishes its new catalog in the same process after a committed service operation. An external `pricing-migrate` process cannot update the already loaded in-memory catalog of a running Keeper process. Restart Keeper after a successful external `run`, `rollback`, or `restore` so it reloads the database state. Deployment does not implicitly adopt legacy pricing; the explicit command remains required.

## Recovery model

The source legacy rows are retained. The migration report stores aggregate counters and safe opaque audit references; frozen candidate input and the previous snapshot are private database state and are excluded from reports and CLI output. A failed or interrupted staging attempt can be retried with the same batch ID by repeating the `run` command. Once input is frozen, a retry uses that original input and the batch's original page size; it does not re-read later legacy edits. After correcting invalid legacy source rows, choose a new batch ID to freeze the repaired source. Repository failures leave the catalog publisher untouched; a candidate is published only after its database transaction commits. Invalid candidates fail atomically and return a safe batch report reason.

Rollback and restore require the batch still to own the active migration switch. A later migration or an ordinary official-price/matrix mutation supersedes that ownership, and stale rollback/restore is rejected. Rollback preserves every target row and selects the frozen prior configuration; restore selects the current strict facts rather than replaying old inputs.

Credential indexes and legacy rule values in the frozen prior configuration are stored as hashes and rehydrated privately from retained original rows. If those original values are physically removed from every retained directory/association/rule row, rollback fails safely. It does not attach an old tariff to a replacement credential. Pricing values are frozen, while display labels during rollback are generic `Credential`/`Channel`.

All source reads and staging writes use bounded pages, but the initial input freeze and final pricing publication each use one transaction to maintain a consistent view and prevent partial adoption. The prior snapshot and model/subject indexes still require memory proportional to their size; this is not an unlimited streaming migration.

Defaults, channels, members, channel defaults/model prices, associations, and rules remain in their original tables. The strict snapshot loader does not query those pricing layers. Frontend changes and removal of their legacy management endpoints belong to issues #20 and #21 and are not implemented here.

Before operating on a production database, use the deployment's ordinary backup procedure and stop or coordinate concurrent writers according to the SQLite deployment policy. The tool uses a bounded single SQLite writer and does not contact CPA or any network service.
