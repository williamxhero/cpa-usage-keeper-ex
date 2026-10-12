package test

import (
	"context"
	. "cpa-usage-keeper/internal/app"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetadataSyncRunnerStartupAndManualRefreshAreCoalescedAndReported(t *testing.T) {
	var calls atomic.Int32
	syncer := &metadataSyncStub{onCall: func(call int) {
		calls.Store(int32(call))
	}}
	runner := NewMetadataSyncRunner(syncer, time.Hour)
	runner.RequestStartupSync()
	runner.RequestPricingCredentialRefresh()
	if status := runner.PricingCredentialSyncStatus(); status.State != "queued" || status.InFlight {
		t.Fatalf("queued status = %+v", status)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	syncer.onCall = func(call int) {
		calls.Store(int32(call))
		cancel()
	}
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("coalesced immediate calls = %d, want 1", got)
	}
	status := runner.PricingCredentialSyncStatus()
	if status.State != "succeeded" || status.InFlight || status.LastAttemptAt == nil || status.LastSuccessAt == nil {
		t.Fatalf("completed status = %+v", status)
	}
}
