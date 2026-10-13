package service

import (
	"context"
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
)

// PricingMigrationProvider is an optional extension of PricingProvider, like
// the official-price and credential-matrix providers.
type PricingMigrationProvider interface {
	RunLegacyPricingMigration(context.Context, repository.PricingMigrationOptions) (repository.PricingMigrationReport, error)
	GetPricingMigrationReport(context.Context, string) (repository.PricingMigrationReport, error)
	ListPricingMigrationBatches(context.Context) ([]entities.PricingMigrationBatch, error)
	RollbackPricingMigration(context.Context, string) (repository.PricingMigrationReport, error)
	RestorePricingMigration(context.Context, string) (repository.PricingMigrationReport, error)
}

var _ PricingMigrationProvider = (*pricingService)(nil)

// The repository compiles inside its final transaction and invokes Replace
// after COMMIT. Holding mutationMu through both steps keeps ordinary mutations
// from publishing out of order. Repository no-ops never invoke the publisher.
func (s *pricingService) RunLegacyPricingMigration(ctx context.Context, options repository.PricingMigrationOptions) (repository.PricingMigrationReport, error) {
	if s == nil || s.db == nil {
		return repository.PricingMigrationReport{}, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return repository.RunLegacyPricingMigration(ctx, s.db, options, s.catalog.Replace)
}

func (s *pricingService) GetPricingMigrationReport(ctx context.Context, batchID string) (repository.PricingMigrationReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return repository.GetPricingMigrationReport(ctx, s.migrationDB(), batchID)
}

func (s *pricingService) ListPricingMigrationBatches(ctx context.Context) ([]entities.PricingMigrationBatch, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return repository.ListPricingMigrationBatches(ctx, s.migrationDB())
}

func (s *pricingService) RollbackPricingMigration(ctx context.Context, batchID string) (repository.PricingMigrationReport, error) {
	return s.mutateMigration(ctx, batchID, repository.RollbackPricingMigration)
}

func (s *pricingService) RestorePricingMigration(ctx context.Context, batchID string) (repository.PricingMigrationReport, error) {
	return s.mutateMigration(ctx, batchID, repository.RestorePricingMigration)
}

type migrationOperation func(context.Context, *gorm.DB, string, ...func(*pricing.Snapshot)) (repository.PricingMigrationReport, error)

func (s *pricingService) mutateMigration(ctx context.Context, batchID string, operation migrationOperation) (repository.PricingMigrationReport, error) {
	if s == nil || s.db == nil {
		return repository.PricingMigrationReport{}, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return operation(ctx, s.db, batchID, s.catalog.Replace)
}

func (s *pricingService) migrationDB() *gorm.DB {
	if s == nil {
		return nil
	}
	return s.db
}
