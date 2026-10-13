package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"

	"github.com/mattn/go-sqlite3"
)

func TestCredentialModelPriceOverrideCompositePrimaryKeyAndFullUpsert(t *testing.T) {
	db := openTestDatabase(t)
	createdAt := time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC)
	rows := []entities.CredentialModelPriceOverride{
		{SubjectID: "subject-a", CanonicalModelID: "shared-model", PricingStyle: "claude", PromptPricePer1M: 1, CompletionPricePer1M: 2, CacheReadPricePer1M: 3, CacheWritePricePer1M: 4, Source: "original", UpdatedAt: createdAt},
		{SubjectID: "subject-b", CanonicalModelID: "shared-model", PricingStyle: "openai", PromptPricePer1M: 5, CompletionPricePer1M: 6, CacheReadPricePer1M: 7, CacheWritePricePer1M: 8, Source: "other-subject", UpdatedAt: createdAt},
		{SubjectID: "subject-a", CanonicalModelID: "other-model", PricingStyle: "openai", PromptPricePer1M: 9, CompletionPricePer1M: 10, CacheReadPricePer1M: 11, CacheWritePricePer1M: 12, Source: "other-model", UpdatedAt: createdAt},
	}
	if err := repository.UpsertCredentialModelPriceOverrides(db, rows); err != nil {
		t.Fatalf("create independent subject/model pairs: %v", err)
	}

	// Bypass upsert to prove that SQLite itself enforces the composite key.
	duplicate := rows[0]
	var constraint sqlite3.Error
	if err := db.Create(&duplicate).Error; !errors.As(err, &constraint) || constraint.ExtendedCode != sqlite3.ErrConstraintPrimaryKey {
		t.Fatalf("duplicate pair must violate the DB primary key, got %v", err)
	}

	replacement := entities.CredentialModelPriceOverride{
		SubjectID: "subject-a", CanonicalModelID: "shared-model", PricingStyle: "openai",
		PromptPricePer1M: 0, CompletionPricePer1M: 20, CacheReadPricePer1M: 0, CacheWritePricePer1M: 0,
		Source: "replacement", UpdatedAt: createdAt.Add(time.Hour),
	}
	if err := repository.UpsertCredentialModelPriceOverrides(db, []entities.CredentialModelPriceOverride{replacement}); err != nil {
		t.Fatalf("replace full tariff including explicit zeroes: %v", err)
	}
	listed, err := repository.ListCredentialModelPriceOverrides(db)
	if err != nil {
		t.Fatal(err)
	}
	want := []entities.CredentialModelPriceOverride{rows[2], replacement, rows[1]}
	assertCredentialOverrideRows(t, listed, want)
}

func TestCredentialModelPriceOverrideDeleteOnlyTargetPair(t *testing.T) {
	db := openTestDatabase(t)
	rows := []entities.CredentialModelPriceOverride{
		{SubjectID: "subject-a", CanonicalModelID: "shared-model", PricingStyle: "openai", PromptPricePer1M: 1},
		{SubjectID: "subject-b", CanonicalModelID: "shared-model", PricingStyle: "openai", PromptPricePer1M: 2},
		{SubjectID: "subject-a", CanonicalModelID: "other-model", PricingStyle: "openai", PromptPricePer1M: 3},
	}
	if err := repository.UpsertCredentialModelPriceOverrides(db, rows); err != nil {
		t.Fatal(err)
	}
	keys := []struct{ SubjectID, CanonicalModelID string }{{"subject-a", "SHARED-MODEL"}}
	for range 2 {
		if err := repository.DeleteCredentialModelPriceOverrides(db, keys); err != nil {
			t.Fatalf("idempotent delete: %v", err)
		}
	}
	listed, err := repository.ListCredentialModelPriceOverrides(db)
	if err != nil {
		t.Fatal(err)
	}
	assertCredentialOverrideRows(t, listed, []entities.CredentialModelPriceOverride{rows[2], rows[1]})
}

func assertCredentialOverrideRows(t *testing.T, got, want []entities.CredentialModelPriceOverride) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("override row count = %d, want %d", len(got), len(want))
	}
	for index, row := range got {
		if !row.UpdatedAt.Equal(want[index].UpdatedAt) {
			t.Fatalf("row %d updated_at = %v, want %v", index, row.UpdatedAt, want[index].UpdatedAt)
		}
		row.UpdatedAt = want[index].UpdatedAt
		if row != want[index] {
			t.Fatalf("row %d = %+v, want %+v", index, row, want[index])
		}
	}
}

func TestCredentialModelPriceOverrideRepositoryCanonicalizesKeys(t *testing.T) {
	db := openTestDatabase(t)
	row := entities.CredentialModelPriceOverride{SubjectID: " subject-a ", CanonicalModelID: " Shared-Model ", PricingStyle: "openai", PromptPricePer1M: 1, CompletionPricePer1M: 2, CacheReadPricePer1M: 3, CacheWritePricePer1M: 4}
	if err := repository.UpsertCredentialModelPriceOverrides(db, []entities.CredentialModelPriceOverride{row}); err != nil {
		t.Fatal(err)
	}
	row.SubjectID = "subject-a"
	row.CanonicalModelID = "shared-model"
	listed, err := repository.ListCredentialModelPriceOverrides(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].SubjectID != row.SubjectID || listed[0].CanonicalModelID != row.CanonicalModelID {
		t.Fatalf("repository did not canonicalize key: %+v", listed)
	}
	if err := repository.DeleteCredentialModelPriceOverrides(db, []struct{ SubjectID, CanonicalModelID string }{{" subject-a ", " SHARED-MODEL "}}); err != nil {
		t.Fatal(err)
	}
	listed, err = repository.ListCredentialModelPriceOverrides(db)
	if err != nil || len(listed) != 0 {
		t.Fatalf("canonical delete left rows: %+v err=%v", listed, err)
	}
}

func TestCredentialModelPriceOverrideSourceSurvivesSnapshotLoad(t *testing.T) {
	db := openTestDatabase(t)
	if err := db.Create(&entities.CredentialPricingSubject{ID: "subject-source", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "source-index"}).Error; err != nil {
		t.Fatal(err)
	}
	row := entities.CredentialModelPriceOverride{SubjectID: "subject-source", CanonicalModelID: "source-model", PricingStyle: "openai", PromptPricePer1M: 1, CompletionPricePer1M: 2, CacheReadPricePer1M: 3, CacheWritePricePer1M: 4, Source: "migrated_from_legacy"}
	if err := repository.UpsertCredentialModelPriceOverrides(db, []entities.CredentialModelPriceOverride{row}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	config, ok := snapshot.CredentialMatrixConfig("subject-source", "source-model")
	if !ok || config.Source != row.Source {
		t.Fatalf("source provenance was not preserved: %+v", config)
	}
}
