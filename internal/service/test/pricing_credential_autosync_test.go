package test

import (
	"context"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/providerconfig"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

func TestSyncCredentialDirectoryAutoDiscoversExactRelationsAndPreservesOnFailure(t *testing.T) {
	db := openMetadataTestDatabase(t, "pricing-credential-autosync.db")
	fetcher := newMetadataTestFetcher()
	fetcher.standardResults["codex"] = &response.ProviderKeyConfigResult{StatusCode: 200, Payload: []providerconfig.ProviderKeyConfig{
		{AuthIndex: "key-a", APIKey: "secret-a", Name: "A"},
		{AuthIndex: "key-b", APIKey: "secret-b", Name: "B"},
		{AuthIndex: "key-c", APIKey: "secret-c", Name: "C"},
	}}
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	syncer := service.NewSyncServiceWithOptions(db, service.SyncServiceOptions{
		MetadataFetcher: fetcher,
		BaseURL:         "https://cpa.example.invalid",
		Now:             func() time.Time { return now },
	})
	ctx := context.Background()
	if err := syncer.SyncCredentialDirectory(ctx); err != nil {
		t.Fatal(err)
	}

	var subjects []entities.CredentialPricingSubject
	if err := db.Order("created_at asc, id asc").Find(&subjects).Error; err != nil {
		t.Fatal(err)
	}
	if len(subjects) != 3 {
		t.Fatalf("subjects = %d, want 3", len(subjects))
	}
	ids := map[string]string{}
	for _, subject := range subjects {
		if subject.ID[:5] != "cred_" {
			t.Fatalf("subject is not opaque: %+v", subject)
		}
		ids[subject.Identity] = subject.ID
	}

	if err := syncer.SyncCredentialDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	var repeated int64
	if err := db.Model(&entities.CredentialPricingSubject{}).Count(&repeated).Error; err != nil || repeated != 3 {
		t.Fatalf("repeat subject count = %d, err=%v", repeated, err)
	}

	fetcher.standardResults["codex"] = &response.ProviderKeyConfigResult{StatusCode: 200, Payload: []providerconfig.ProviderKeyConfig{
		{AuthIndex: "key-a", APIKey: "secret-a", Name: "Renamed"},
		{AuthIndex: "key-b", APIKey: "secret-b", Name: "B"},
		{AuthIndex: "key-c-rotated", APIKey: "secret-c2", Name: "C"},
	}}
	now = now.Add(time.Minute)
	if err := syncer.SyncCredentialDirectory(ctx); err != nil {
		t.Fatal(err)
	}
	var afterRotation []entities.CredentialPricingSubject
	if err := db.Order("created_at asc, id asc").Find(&afterRotation).Error; err != nil {
		t.Fatal(err)
	}
	if len(afterRotation) != 4 {
		t.Fatalf("subjects after rotation = %d, want 4", len(afterRotation))
	}
	for _, subject := range afterRotation {
		if expected, ok := ids[subject.Identity]; ok && expected != subject.ID {
			t.Fatalf("stable subject changed after label/rotation: %+v", subject)
		}
	}

	before := make([]entities.CredentialPricingSubject, len(afterRotation))
	copy(before, afterRotation)
	fetcher.standardErrors["codex"] = context.DeadlineExceeded
	if err := syncer.SyncCredentialDirectory(ctx); err == nil {
		t.Fatal("expected upstream failure")
	}
	var afterFailure []entities.CredentialPricingSubject
	if err := db.Order("created_at asc, id asc").Find(&afterFailure).Error; err != nil {
		t.Fatal(err)
	}
	if len(afterFailure) != len(before) {
		t.Fatalf("failed refresh changed subject count: %d -> %d", len(before), len(afterFailure))
	}
	for index := range before {
		if before[index].ID != afterFailure[index].ID || before[index].Identity != afterFailure[index].Identity {
			t.Fatalf("failed refresh changed subject directory: before=%+v after=%+v", before, afterFailure)
		}
	}

	// Keep the repository directory reader exercised so future changes cannot
	// accidentally make the auto-created subjects invisible to the pricing API.
	if err := repository.ReadCredentialPricingDirectory(ctx, db, func(_ []entities.UsageIdentity, subjects []entities.CredentialPricingSubject) error {
		if len(subjects) < len(before) {
			t.Fatalf("read directory subjects = %d, want at least %d", len(subjects), len(before))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
