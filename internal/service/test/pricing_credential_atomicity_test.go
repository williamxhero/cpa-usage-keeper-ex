package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/providerconfig"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/service"
	"gorm.io/gorm"
)

func TestSyncCredentialDirectoryRollsBackAllScopesOnPersistenceFailure(t *testing.T) {
	db := openMetadataTestDatabase(t, "pricing-credential-atomicity.db")
	fetcher := newMetadataTestFetcher()
	fetcher.standardResults["codex"] = &response.ProviderKeyConfigResult{
		StatusCode: 200,
		Payload:    []providerconfig.ProviderKeyConfig{{AuthIndex: "key-before", APIKey: "secret-before", Name: "Before"}},
	}
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	syncer := service.NewSyncServiceWithOptions(db, service.SyncServiceOptions{
		MetadataFetcher: fetcher,
		BaseURL:         "https://cpa.example.invalid",
		Now:             func() time.Time { return now },
	})
	if err := syncer.SyncCredentialDirectory(context.Background()); err != nil {
		t.Fatal(err)
	}
	var beforeSubjects []entities.CredentialPricingSubject
	if err := db.Order("id asc").Find(&beforeSubjects).Error; err != nil {
		t.Fatal(err)
	}
	if len(beforeSubjects) != 1 {
		t.Fatalf("initial subjects = %d, want 1", len(beforeSubjects))
	}

	fetcher.standardResults["codex"] = &response.ProviderKeyConfigResult{
		StatusCode: 200,
		Payload:    []providerconfig.ProviderKeyConfig{{AuthIndex: "key-after", APIKey: "secret-after", Name: "After"}},
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:block_complete_provider_identity_create", func(tx *gorm.DB) {
		if tx.Statement.Table == "usage_identities" {
			tx.AddError(errors.New("complete provider persistence blocked"))
		}
	}); err != nil {
		t.Fatalf("register persistence callback: %v", err)
	}
	if err := syncer.SyncCredentialDirectory(context.Background()); err == nil {
		t.Fatal("expected complete refresh persistence failure")
	}

	identities := loadMetadataIdentityMap(t, db)
	if _, ok := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "key-after")]; ok {
		t.Fatalf("failed refresh published new identity: %+v", identities)
	}
	if before := identities[metadataIdentityKey(entities.UsageIdentityAuthTypeAIProvider, "key-before")]; before.Identity != "key-before" || before.IsDeleted {
		t.Fatalf("previous identity changed after rollback: %+v", before)
	}
	var afterSubjects []entities.CredentialPricingSubject
	if err := db.Order("id asc").Find(&afterSubjects).Error; err != nil {
		t.Fatal(err)
	}
	if len(afterSubjects) != len(beforeSubjects) || afterSubjects[0].ID != beforeSubjects[0].ID {
		t.Fatalf("failed refresh changed subjects: before=%+v after=%+v", beforeSubjects, afterSubjects)
	}
}
