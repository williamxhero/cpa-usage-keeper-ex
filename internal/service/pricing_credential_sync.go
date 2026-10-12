package service

import (
	"context"
	"time"

	"cpa-usage-keeper/internal/repository"
	"gorm.io/gorm"
)

// SyncPricingCredentialSubjects reconciles automatically discovered subjects after
// a complete metadata run has succeeded. It is called by the background runner,
// keeping direct legacy metadata helpers backward-compatible for manual bindings.
func (s *SyncService) SyncPricingCredentialSubjects(ctx context.Context) error {
	if s == nil || s.db == nil {
		return repository.EnsureCredentialPricingSubjects(nil, time.Now())
	}
	return s.syncPricingCredentialSubjectsAt(ctx, s.now())
}

func (s *SyncService) syncPricingCredentialSubjectsAt(ctx context.Context, now time.Time) error {
	if s == nil || s.db == nil {
		return repository.EnsureCredentialPricingSubjects(nil, now)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return repository.EnsureCredentialPricingSubjects(tx, now)
	})
}
