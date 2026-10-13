// pricing-migrate performs explicit local SPEC-003 migration operations.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
)

var batchIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)

const usage = `Usage: pricing-migrate -db <existing SQLite file> -action <run|report|list|rollback|restore> [-batch <ID>] [-batch-size <1..1000>]

-db is required; SQLite URIs, query parameters, and memory databases are rejected.
-batch is required except for list. Use a non-sensitive identifier of 1..96 ASCII
letters, digits, underscores, or hyphens, beginning with a letter or digit.
-batch-size defaults to 250 and applies only to run (including retries).
Output is JSON. No configuration or environment files are loaded.
External mutations require restarting Keeper to load its new pricing catalog.
`

func main() {
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// Parse errors can contain flag values (including paths or credentials), so
// flag's automatic output and raw errors must never reach the terminal.
func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pricing-migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var databasePath, action, batchID string
	var batchSize int
	flags.StringVar(&databasePath, "db", "", "existing SQLite file")
	flags.StringVar(&action, "action", "", "migration action")
	flags.StringVar(&batchID, "batch", "", "non-sensitive batch identifier")
	flags.IntVar(&batchSize, "batch-size", 250, "staging page size")
	if err := flags.Parse(args); errors.Is(err, flag.ErrHelp) {
		_, _ = io.WriteString(stdout, usage)
		return 0
	} else if err != nil || flags.NArg() != 0 {
		return cliError(stderr, "invalid command arguments; use -h for usage", 2)
	}
	if action != "run" && action != "report" && action != "list" && action != "rollback" && action != "restore" {
		return cliError(stderr, "-action must be run, report, list, rollback, or restore", 2)
	}
	if action == "list" {
		if batchID != "" {
			return cliError(stderr, "-batch does not apply to list", 2)
		}
	} else if !batchIDPattern.MatchString(batchID) {
		return cliError(stderr, "-batch must be a non-sensitive identifier of 1..96 ASCII letters, digits, underscores, or hyphens, starting with a letter or digit", 2)
	}
	if batchSize < 1 || batchSize > 1000 {
		return cliError(stderr, "-batch-size must be between 1 and 1000", 2)
	}
	invalidSizeFlag := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "batch-size" && action != "run" {
			invalidSizeFlag = true
		}
	})
	if invalidSizeFlag {
		return cliError(stderr, "-batch-size applies only to run", 2)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	db, closeDB, err := openMigrationDatabase(ctx, databasePath, action == "report" || action == "list")
	if err != nil {
		return cliError(stderr, err.Error(), 1)
	}
	defer closeDB()
	if err := prepareMigrationSchema(db, action); err != nil {
		return cliError(stderr, err.Error(), 1)
	}

	if action == "list" {
		batches, err := repository.ListPricingMigrationBatches(ctx, db)
		if err != nil {
			return cliError(stderr, "cannot list pricing migration batches", 1)
		}
		result := make([]batchOutput, 0, len(batches))
		for _, batch := range batches {
			result = append(result, summarizeBatch(batch))
		}
		return writeJSON(stdout, stderr, result)
	}

	var report repository.PricingMigrationReport
	switch action {
	case "run":
		report, err = repository.RunLegacyPricingMigration(ctx, db, repository.PricingMigrationOptions{BatchID: batchID, BatchSize: batchSize})
	case "report":
		report, err = repository.GetPricingMigrationReport(ctx, db, batchID)
	case "rollback":
		report, err = repository.RollbackPricingMigration(ctx, db, batchID)
	case "restore":
		report, err = repository.RestorePricingMigration(ctx, db, batchID)
	}
	if report.Batch.ID != "" {
		if code := writeJSON(stdout, stderr, summarizeReport(report)); code != 0 {
			return code
		}
	}
	if errors.Is(err, repository.ErrPricingMigrationNotFound) {
		return cliError(stderr, "pricing migration batch not found", 1)
	}
	if err != nil {
		return cliError(stderr, "pricing migration operation failed; inspect the batch report", 1)
	}
	return 0
}

func cliError(stderr io.Writer, message string, code int) int {
	_, _ = fmt.Fprintln(stderr, message)
	return code
}

func writeJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return cliError(stderr, "cannot write migration output", 1)
	}
	return 0
}

// Explicit output types keep private frozen snapshots and staged pricing
// inputs out of CLI output even if the repository report gains new fields.
type batchOutput struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	ImportedCount int    `json:"imported_count"`
	SkippedCount  int    `json:"skipped_count"`
	RejectedCount int    `json:"rejected_count"`
	ConflictCount int    `json:"conflict_count"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type countOutput struct {
	Category  string `json:"category"`
	Migrated  int    `json:"migrated"`
	Skipped   int    `json:"skipped"`
	Rejected  int    `json:"rejected"`
	Conflicts int    `json:"conflicts"`
}

type itemOutput struct {
	Category  string `json:"category"`
	SourceRef string `json:"source_ref"`
	Outcome   string `json:"outcome"`
	Reason    string `json:"reason,omitempty"`
}

type reportOutput struct {
	Batch  batchOutput   `json:"batch"`
	Counts []countOutput `json:"counts"`
	Items  []itemOutput  `json:"items"`
}

func summarizeBatch(batch entities.PricingMigrationBatch) batchOutput {
	status := "unknown"
	switch batch.Status {
	case entities.PricingMigrationStatusRunning, entities.PricingMigrationStatusSucceeded, entities.PricingMigrationStatusFailed, entities.PricingMigrationStatusRolledBack:
		status = batch.Status
	}
	id := "invalid"
	if batchIDPattern.MatchString(batch.ID) {
		id = batch.ID
	}
	return batchOutput{ID: id, Status: status, ImportedCount: batch.ImportedCount, SkippedCount: batch.SkippedCount, RejectedCount: batch.RejectedCount, ConflictCount: batch.ConflictCount, FailureReason: safeReason(batch.FailureReason)}
}

func summarizeReport(report repository.PricingMigrationReport) reportOutput {
	result := reportOutput{Batch: summarizeBatch(report.Batch), Counts: make([]countOutput, 0, len(report.Counts)), Items: make([]itemOutput, 0, len(report.Items))}
	for _, count := range report.Counts {
		result.Counts = append(result.Counts, countOutput{Category: safeCategory(count.Category), Migrated: count.Migrated, Skipped: count.Skipped, Rejected: count.Rejected, Conflicts: count.Conflicts})
	}
	for _, item := range report.Items {
		category := safeCategory(item.Category)
		ref := "redacted"
		if category != "unknown" && strings.HasPrefix(item.SourceRef, category+":") && opaqueRefPattern.MatchString(item.SourceRef) {
			ref = item.SourceRef
		}
		outcome := "unknown"
		switch item.Outcome {
		case entities.PricingMigrationOutcomeMigrated, entities.PricingMigrationOutcomeSkipped, entities.PricingMigrationOutcomeRejected, entities.PricingMigrationOutcomeConflict:
			outcome = item.Outcome
		}
		result.Items = append(result.Items, itemOutput{Category: category, SourceRef: ref, Outcome: outcome, Reason: safeReason(item.Reason)})
	}
	return result
}

var opaqueRefPattern = regexp.MustCompile(`^[a-z_]+:[a-f0-9]{24}$`)

func safeCategory(value string) string {
	switch value {
	case entities.PricingMigrationCategoryModelPrices, entities.PricingMigrationCategoryCredentialFixed, entities.PricingMigrationCategoryCredentialMultiplier, entities.PricingMigrationCategoryCredentialDefaults, entities.PricingMigrationCategoryChannels, entities.PricingMigrationCategoryChannelMembers, entities.PricingMigrationCategoryChannelDefaults, entities.PricingMigrationCategoryChannelModels, entities.PricingMigrationCategoryAssociations, entities.PricingMigrationCategoryModelRules:
		return value
	default:
		return "unknown"
	}
}

func safeReason(value string) string {
	switch value {
	case "", "missing_model", "missing_subject", "invalid_style", "invalid_price", "invalid_mode", "invalid_multiplier", "complete_fixed_tariff_required", "missing_official_model", "official_price_missing_segment", "materialized_price_overflow", "target_exists", "target_conflict", "legacy_layer_disabled", "candidate_invalid", "duplicate_canonical_target", "previous_snapshot_invalid", "input_encoding_failed", "input_read_failed", "frozen_input_invalid", "target_read_failed", "target_write_failed", "stage_write_failed", "stage_read_failed", "report_read_failed", "report_write_failed", "state_write_failed", "activation_failed", "snapshot_capture_failed", "mode_read_failed", "batch_update_failed", "migration_failed", "batch_rejected", "previous_snapshot_selected":
		return value
	default:
		return "redacted"
	}
}
