package main

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
)

func duplicateLegacyPrice(model string, prompt float64) entities.ModelPriceSetting {
	return entities.ModelPriceSetting{Model: model, PricingStyle: "openai", PromptPricePer1M: prompt, CompletionPricePer1M: 3, CacheReadPricePer1M: 1, CacheWritePricePer1M: 4}
}

func duplicateOfficialPrice(model string, prompt float64) entities.OfficialModelPrice {
	return entities.OfficialModelPrice{CanonicalModelID: model, DisplayModelID: model, Source: entities.OfficialModelPriceSourceLegacySeed, PricingStyle: "openai", PromptPricePer1M: new(prompt), CompletionPricePer1M: new(3.0), CacheReadPricePer1M: new(1.0), CacheWritePricePer1M: new(4.0)}
}

func duplicateTargetFacts(t *testing.T, db *gorm.DB) ([]entities.OfficialModelPrice, []entities.CredentialModelPriceOverride) {
	t.Helper()
	var official []entities.OfficialModelPrice
	var matrix []entities.CredentialModelPriceOverride
	if err := db.Order("id asc").Find(&official).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Order("subject_id asc, canonical_model_id asc").Find(&matrix).Error; err != nil {
		t.Fatal(err)
	}
	return official, matrix
}

func TestCLIWhitespaceDuplicateSources(t *testing.T) {
	path, db := newCLIDatabase(t)
	legacy := []entities.ModelPriceSetting{duplicateLegacyPrice("Model-A", 2), duplicateLegacyPrice(" Model-A ", 2)}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.CredentialPricingSubject{ID: "subject-a", BindingRef: "binding-a", BindingDisabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"Model-B", " Model-B "} {
		row := entities.CredentialModelMultiplier{SubjectID: "subject-a", Model: model, Mode: "fixed", PricingStyle: "openai", PromptPricePer1M: new(2.0), CompletionPricePer1M: new(3.0), CacheReadPricePer1M: new(1.0), CacheWritePricePer1M: new(4.0)}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := invokeCLI(t, "-db", path, "-action", "run", "-batch", "whitespace-duplicates", "-batch-size", "1")
	report := decodeCLIReport(t, stdout)
	t.Logf("runCLI exit=%d status=%s failure_reason=%s", code, report.Batch.Status, report.Batch.FailureReason)
	if code != 0 || stderr != "" || report.Batch.Status != entities.PricingMigrationStatusSucceeded || report.Batch.ImportedCount != 2 || report.Batch.SkippedCount != 2 || report.Batch.RejectedCount != 0 {
		t.Fatal("whitespace duplicates aborted adoption")
	}
	for _, action := range []string{"rollback", "restore"} {
		code, _, stderr := invokeCLI(t, "-db", path, "-action", action, "-batch", "whitespace-duplicates")
		if code != 0 || stderr != "" {
			t.Fatalf("normalized previous snapshot cannot %s: exit=%d", action, code)
		}
	}
}

func TestCLIDuplicateFixedSources(t *testing.T) {
	for _, tc := range []struct {
		name     string
		price    *float64
		style    string
		reason   string
		conflict bool
	}{
		{name: "equal", price: new(2.0), style: "openai"},
		{name: "different", price: new(7.0), style: "openai", conflict: true},
		{name: "missing_segment", style: "openai", reason: "complete_fixed_tariff_required"},
		{name: "negative", price: new(-2.0), style: "openai", reason: "complete_fixed_tariff_required"},
		{name: "NaN", price: new(math.NaN()), style: "openai", reason: "complete_fixed_tariff_required"},
		{name: "Inf", price: new(math.Inf(1)), style: "openai", reason: "complete_fixed_tariff_required"},
		{name: "invalid_style", price: new(2.0), style: "invalid", reason: "invalid_style"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, db := newCLIDatabase(t)
			if err := db.Create(&entities.CredentialPricingSubject{ID: "subject-a", BindingRef: "binding-a", BindingDisabled: true}).Error; err != nil {
				t.Fatal(err)
			}
			rows := []entities.CredentialModelMultiplier{
				{SubjectID: "subject-a", Model: "Model-A", Mode: "fixed", PricingStyle: "openai", PromptPricePer1M: new(2.0), CompletionPricePer1M: new(3.0), CacheReadPricePer1M: new(1.0), CacheWritePricePer1M: new(4.0)},
				{SubjectID: "subject-a", Model: "model-a", Mode: "fixed", PricingStyle: tc.style, PromptPricePer1M: tc.price, CompletionPricePer1M: new(3.0), CacheReadPricePer1M: new(1.0), CacheWritePricePer1M: new(4.0)},
			}
			if err := db.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}
			code, stdout, _ := invokeCLI(t, "-db", path, "-action", "run", "-batch", "fixed-duplicates", "-batch-size", "1")
			report := decodeCLIReport(t, stdout)
			_, matrix := duplicateTargetFacts(t, db)
			if tc.reason != "" {
				if code != 1 || report.Batch.Status != entities.PricingMigrationStatusFailed || report.Batch.RejectedCount != 2 || len(matrix) != 0 || repository.IsStrictPricingActivated(db) {
					t.Fatal("invalid duplicate source did not fail atomically")
				}
				found := false
				for _, item := range report.Items {
					if item.Outcome == entities.PricingMigrationOutcomeRejected && item.Reason == tc.reason {
						found = true
					}
					if item.Reason == "duplicate_canonical_target" {
						t.Fatal("duplicate detection masked the actual invalid source reason")
					}
				}
				if !found {
					t.Fatal("report omitted the invalid source reason")
				}
				return
			}
			if code != 0 || report.Batch.Status != entities.PricingMigrationStatusSucceeded || report.Batch.ImportedCount != 1 || report.Batch.RejectedCount != 0 || len(matrix) != 1 || matrix[0].PromptPricePer1M != 2 {
				t.Fatal("duplicate fixed sources did not preserve the first adopted tariff")
			}
			if tc.conflict && report.Batch.ConflictCount != 1 || !tc.conflict && report.Batch.SkippedCount != 1 {
				t.Fatal("duplicate fixed source was not audited")
			}
		})
	}
}

func TestCLIWhitespaceDuplicateDoesNotHideInvalidRule(t *testing.T) {
	path, db := newCLIDatabase(t)
	rows := []entities.ModelPriceSetting{duplicateLegacyPrice(" Model-A ", 2), duplicateLegacyPrice("Model-A", 2)}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.ModelPriceRule{ModelPriceSettingID: rows[1].ID, Key: "invalid-field", Value: "ordinary", Multiplier: 2}).Error; err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := invokeCLI(t, "-db", path, "-action", "run", "-batch", "invalid-duplicate-rule")
	report := decodeCLIReport(t, stdout)
	official, matrix := duplicateTargetFacts(t, db)
	if code != 1 || report.Batch.Status != entities.PricingMigrationStatusFailed || report.Batch.FailureReason != "previous_snapshot_invalid" || len(official) != 0 || len(matrix) != 0 || repository.IsStrictPricingActivated(db) {
		t.Fatal("normalizing duplicate rollback input hid an invalid discarded rule")
	}
}

func TestCLIExistingMatrixDuplicatesFailAtomically(t *testing.T) {
	path, db := newCLIDatabase(t)
	if err := db.Create(&entities.CredentialPricingSubject{ID: "subject-a", BindingRef: "binding-a", BindingDisabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	rows := []entities.CredentialModelPriceOverride{
		{SubjectID: "subject-a", CanonicalModelID: "Model-A", PricingStyle: "openai", PromptPricePer1M: 2, Source: "manual"},
		{SubjectID: "subject-a", CanonicalModelID: "model-a", PricingStyle: "openai", PromptPricePer1M: 7, Source: "manual"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	beforeOfficial, beforeMatrix := duplicateTargetFacts(t, db)
	mode, pointer, err := repository.CurrentPricingMigrationState(db)
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := invokeCLI(t, "-db", path, "-action", "run", "-batch", "invalid-existing-matrix")
	report := decodeCLIReport(t, stdout)
	afterOfficial, afterMatrix := duplicateTargetFacts(t, db)
	modeAfter, pointerAfter, err := repository.CurrentPricingMigrationState(db)
	if code != 1 || report.Batch.Status != entities.PricingMigrationStatusFailed || err != nil || mode != modeAfter || pointer != pointerAfter || !reflect.DeepEqual(beforeOfficial, afterOfficial) || !reflect.DeepEqual(beforeMatrix, afterMatrix) {
		t.Fatal("uncompilable existing matrix was accepted or changed pricing state")
	}
}

func TestCLIDuplicateMultiplierSources(t *testing.T) {
	for _, tc := range []struct {
		name           string
		seeded         bool
		differentBase  bool
		fixedFirst     bool
		secondMultiple float64
		conflicts      int
		skipped        int
	}{
		{name: "equal_seed_targets_and_equal_multipliers", seeded: true, secondMultiple: 2, skipped: 1},
		{name: "equal_seed_targets_and_different_multipliers", seeded: true, secondMultiple: 3, conflicts: 1},
		{name: "different_sources_use_first_adopted_base", differentBase: true, secondMultiple: 2, conflicts: 1, skipped: 1},
		{name: "fixed_and_multiplier_share_matrix_target", seeded: true, fixedFirst: true, secondMultiple: 3, conflicts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, db := newCLIDatabase(t)
			if tc.seeded {
				prices := []entities.OfficialModelPrice{duplicateOfficialPrice("Model-A", 2), duplicateOfficialPrice("model-a", 2)}
				if err := db.Create(&prices).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				second := 2.0
				if tc.differentBase {
					second = 7
				}
				prices := []entities.ModelPriceSetting{duplicateLegacyPrice("Model-A", 2), duplicateLegacyPrice("model-a", second)}
				if err := db.Create(&prices).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&entities.CredentialPricingSubject{ID: "subject-a", BindingRef: "binding-a", BindingDisabled: true}).Error; err != nil {
				t.Fatal(err)
			}
			first := entities.CredentialModelMultiplier{SubjectID: "subject-a", Model: "Model-A", Mode: "multiplier", Multiplier: 2}
			if tc.fixedFirst {
				first.Mode, first.PricingStyle = "fixed", "openai"
				first.PromptPricePer1M, first.CompletionPricePer1M = new(4.0), new(6.0)
				first.CacheReadPricePer1M, first.CacheWritePricePer1M = new(2.0), new(8.0)
			}
			rows := []entities.CredentialModelMultiplier{first, {SubjectID: "subject-a", Model: "model-a", Mode: "multiplier", Multiplier: tc.secondMultiple}}
			if err := db.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := invokeCLI(t, "-db", path, "-action", "run", "-batch", "multiplier-duplicates", "-batch-size", "1")
			report := decodeCLIReport(t, stdout)
			imports := 1
			if !tc.seeded {
				imports++
			}
			if code != 0 || stderr != "" || report.Batch.Status != entities.PricingMigrationStatusSucceeded || report.Batch.ImportedCount != imports || report.Batch.ConflictCount != tc.conflicts || report.Batch.SkippedCount != tc.skipped || report.Batch.RejectedCount != 0 {
				t.Fatal("duplicate multipliers or mixed modes were not audited correctly")
			}
			_, matrix := duplicateTargetFacts(t, db)
			if len(matrix) != 1 || matrix[0].PromptPricePer1M != 4 || matrix[0].CompletionPricePer1M != 6 || matrix[0].CacheReadPricePer1M != 2 || matrix[0].CacheWritePricePer1M != 8 {
				t.Fatal("duplicate source overwrote the first materialized tariff")
			}
		})
	}
}

func TestCLIAmbiguousOfficialMultiplierIsConflict(t *testing.T) {
	path, db := newCLIDatabase(t)
	// The differing target is on a later input page, not among the first two.
	prices := []entities.OfficialModelPrice{duplicateOfficialPrice("MODEL-A", 2), duplicateOfficialPrice("Model-A", 2), duplicateOfficialPrice("model-a", 7)}
	if err := db.Create(&prices).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.CredentialPricingSubject{ID: "subject-a", BindingRef: "binding-a", BindingDisabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.CredentialModelMultiplier{SubjectID: "subject-a", Model: "model-a", Mode: "multiplier", Multiplier: 2}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entities.ModelPriceSetting{Model: "model-a", PricingStyle: "openai", PromptPricePer1M: 2, CompletionPricePer1M: 3, CacheReadPricePer1M: 1, CacheWritePricePer1M: 4}).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := duplicateTargetFacts(t, db)
	code, stdout, stderr := invokeCLI(t, "-db", path, "-action", "run", "-batch", "ambiguous-multiplier", "-batch-size", "1")
	report := decodeCLIReport(t, stdout)
	if code != 0 || stderr != "" || report.Batch.Status != entities.PricingMigrationStatusSucceeded || report.Batch.ConflictCount != 2 || report.Batch.RejectedCount != 0 || len(report.Items) != 2 {
		t.Fatal("ambiguous official targets were not reported as conflicts")
	}
	for _, item := range report.Items {
		if item.Outcome != entities.PricingMigrationOutcomeConflict || item.Reason != "target_conflict" {
			t.Fatal("differing target on a later page was silently ignored")
		}
	}
	after, matrix := duplicateTargetFacts(t, db)
	if !reflect.DeepEqual(before, after) || len(matrix) != 0 {
		t.Fatal("ambiguous official prices were modified or silently materialized")
	}
}

func TestCLIDuplicateCanonicalTargets(t *testing.T) {
	for _, tc := range []struct {
		name      string
		legacy    bool
		seeded    bool
		single    bool
		different bool
		conflicts int
		skipped   int
		imported  int
	}{
		{name: "AC1_empty_sources_with_legacy_seed_targets", seeded: true},
		{name: "AC3_empty_sources_differing_existing_targets", seeded: true, different: true, conflicts: 1},
		{name: "AC2_equal_aliases_with_legacy_seed_targets", legacy: true, seeded: true, skipped: 2, imported: 1},
		{name: "AC2_equal_aliases_without_targets", legacy: true, skipped: 1, imported: 2},
		{name: "AC3_different_aliases_preserve_existing_targets", legacy: true, seeded: true, different: true, conflicts: 2, imported: 1},
		{name: "AC3_different_aliases_preserve_single_existing_target", legacy: true, seeded: true, single: true, different: true, conflicts: 2, imported: 1},
		{name: "AC3_different_aliases_preserve_first_import", legacy: true, different: true, conflicts: 1, imported: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, db := newCLIDatabase(t)
			secondPrompt := 2.0
			if tc.different {
				secondPrompt = 7
			}
			// Startup seed rows are targets, not additional legacy source items.
			if tc.seeded {
				prices := []entities.OfficialModelPrice{duplicateOfficialPrice("Vendor/Model-A", 2), duplicateOfficialPrice("vendor/model-a", secondPrompt)}
				if tc.single {
					prices = []entities.OfficialModelPrice{duplicateOfficialPrice("vendor/model-a", 9)}
				}
				if err := db.Create(&prices).Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.legacy {
				legacy := []entities.ModelPriceSetting{duplicateLegacyPrice("Vendor/Model-A", 2), duplicateLegacyPrice("vendor/model-a", secondPrompt)}
				if err := db.Create(&legacy).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&entities.CredentialPricingSubject{ID: "subject-a", BindingRef: "binding-a", BindingDisabled: true}).Error; err != nil {
					t.Fatal(err)
				}
				fixed := entities.CredentialModelMultiplier{SubjectID: "subject-a", Model: "independent-model", Mode: "fixed", PricingStyle: "openai", PromptPricePer1M: new(5.0), CompletionPricePer1M: new(6.0), CacheReadPricePer1M: new(1.0), CacheWritePricePer1M: new(2.0)}
				if err := db.Create(&fixed).Error; err != nil {
					t.Fatal(err)
				}
			}
			before, _ := duplicateTargetFacts(t, db)
			code, stdout, stderr := invokeCLI(t, "-db", path, "-action", "run", "-batch", "duplicate-test", "-batch-size", "1")
			report := decodeCLIReport(t, stdout)
			t.Logf("runCLI exit=%d status=%s imported=%d skipped=%d conflicts=%d rejected=%d failure_reason=%s", code, report.Batch.Status, report.Batch.ImportedCount, report.Batch.SkippedCount, report.Batch.ConflictCount, report.Batch.RejectedCount, report.Batch.FailureReason)
			for _, item := range report.Items {
				t.Logf("category=%s outcome=%s reason=%s", item.Category, item.Outcome, item.Reason)
			}
			if code != 0 || stderr != "" || report.Batch.Status != entities.PricingMigrationStatusSucceeded || report.Batch.RejectedCount != 0 {
				t.Fatal("canonical duplicates aborted adoption")
			}
			if report.Batch.ImportedCount != tc.imported || report.Batch.SkippedCount != tc.skipped || report.Batch.ConflictCount != tc.conflicts {
				t.Fatalf("unexpected counts: %+v", report.Batch)
			}
			expectedItems := 0
			if tc.legacy {
				expectedItems = 3
			} else if tc.different {
				expectedItems = 1 // catalog-only audit of differing retained targets
			}
			if len(report.Items) != expectedItems {
				t.Fatal("official seed targets generated phantom source items")
			}
			if strings.Contains(stdout+stderr, path) || strings.Contains(stdout+stderr, "Vendor/Model-A") || strings.Contains(stdout+stderr, "subject-a") {
				t.Fatal("report leaked private input")
			}
			after, matrix := duplicateTargetFacts(t, db)
			if tc.seeded {
				if !reflect.DeepEqual(before, after) {
					t.Fatal("migration modified or dropped existing official target values")
				}
			} else if len(after) != 1 || after[0].PromptPricePer1M == nil || *after[0].PromptPricePer1M != 2 {
				t.Fatal("duplicate source overwrote the first adopted target")
			}
			if tc.legacy && (len(matrix) != 1 || matrix[0].PromptPricePer1M != 5) {
				t.Fatal("independent fixed tariff was rejected with duplicate official sources")
			}
			mode, pointer, err := repository.CurrentPricingMigrationState(db)
			if err != nil || !repository.IsStrictPricingActivated(db) {
				t.Fatal("successful migration did not activate strict pricing")
			}
			// A repeated batch is a read-only success; a fresh batch may audit sources
			// again but must not overwrite targets or change pricing mode.
			for _, batch := range []string{"duplicate-test", "duplicate-rerun"} {
				code, _, stderr = invokeCLI(t, "-db", path, "-action", "run", "-batch", batch, "-batch-size", "1")
				if code != 0 || stderr != "" {
					t.Fatalf("rerun exit=%d", code)
				}
				officialAgain, matrixAgain := duplicateTargetFacts(t, db)
				modeAgain, pointerAgain, err := repository.CurrentPricingMigrationState(db)
				if err != nil || modeAgain != mode || (batch == "duplicate-test" && pointerAgain != pointer) || !reflect.DeepEqual(after, officialAgain) || !reflect.DeepEqual(matrix, matrixAgain) {
					t.Fatal("rerun changed pricing mode or target facts")
				}
			}
		})
	}
}
