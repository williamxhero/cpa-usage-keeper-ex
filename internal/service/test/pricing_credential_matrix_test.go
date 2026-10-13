package test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type credentialMatrixFixture struct {
	db      *gorm.DB
	path    string
	matrix  service.CredentialModelMatrixProvider
	catalog *pricing.Catalog
}

func openCredentialMatrixDatabase(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := repository.OpenDatabase(config.Config{SQLitePath: path})
	if err != nil {
		t.Fatalf("open isolated matrix database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("close matrix database: %v", err)
		}
	})
	return db
}

func (f credentialMatrixFixture) reopen(t *testing.T) credentialMatrixFixture {
	t.Helper()
	sqlDB, err := f.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}
	f.db = openCredentialMatrixDatabase(t, f.path)
	provider, catalog := newCatalogPricingService(t, f.db)
	f.matrix, f.catalog = provider.(service.CredentialModelMatrixProvider), catalog
	return f
}

func newCredentialMatrixFixture(t *testing.T) credentialMatrixFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credential-matrix.db")
	db := openCredentialMatrixDatabase(t, path)
	subjects := []entities.CredentialPricingSubject{
		{ID: "matrix-a", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-matrix-a"},
		{ID: "matrix-b", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "synthetic-matrix-b"},
	}
	if err := db.Create(&subjects).Error; err != nil {
		t.Fatal(err)
	}
	provider, catalog := newCatalogPricingService(t, db)
	return credentialMatrixFixture{db: db, path: path, matrix: provider.(service.CredentialModelMatrixProvider), catalog: catalog}
}

func matrixTariff(subject, model, style string, prices [4]float64) service.CredentialMatrixCellInput {
	return service.CredentialMatrixCellInput{
		SubjectID: subject, CanonicalModelID: model, PricingStyle: style,
		PromptPricePer1M: &prices[0], CompletionPricePer1M: &prices[1],
		CacheReadPricePer1M: &prices[2], CacheWritePricePer1M: &prices[3],
	}
}

func assertMatrixCell(t *testing.T, cell service.CredentialMatrixCell, subject, model, style, snapshotID string, prices [4]float64) {
	t.Helper()
	if cell.SubjectID != subject || cell.CanonicalModelID != model || !cell.Covered || cell.Mode != "fixed" || cell.PricingStyle != style || cell.Source != pricing.CredentialMatrixSource || cell.SnapshotID != snapshotID {
		t.Fatalf("unexpected matrix cell: %+v", cell)
	}
	values := []*float64{cell.PromptPricePer1M, cell.CompletionPricePer1M, cell.CacheReadPricePer1M, cell.CacheWritePricePer1M}
	for index, value := range values {
		if value == nil || *value != prices[index] {
			t.Fatalf("cell %s/%s tariff segment %d = %v, want %v", subject, model, index, value, prices[index])
		}
	}
}

func assertMatrixStateUnchanged(t *testing.T, f credentialMatrixFixture, snapshot *pricing.Snapshot, rows []entities.CredentialModelPriceOverride) {
	t.Helper()
	if f.catalog.Snapshot() != snapshot {
		t.Fatal("rejected mutation published a different catalog snapshot")
	}
	persisted, err := repository.ListCredentialModelPriceOverrides(f.db)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, rows) {
		t.Fatalf("rejected mutation changed persisted rows: got %+v, want %+v", persisted, rows)
	}
}

func TestCredentialMatrixCompleteTariffsIndependentSubjectsClearAndRestart(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	ctx := context.Background()
	cells, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff(" matrix-a ", " Provider/Shared-Model ", " CLAUDE ", [4]float64{1, 2, 3, 4}),
		matrixTariff("matrix-b", "provider/shared-model", "", [4]float64{5, 6, 7, 8}),
	})
	if err != nil || len(cells) != 2 {
		t.Fatalf("save complete tariffs: %+v %v", cells, err)
	}
	assertMatrixCell(t, cells[0], "matrix-a", "provider/shared-model", "claude", f.catalog.Snapshot().ID(), [4]float64{1, 2, 3, 4})
	assertMatrixCell(t, cells[1], "matrix-b", "provider/shared-model", "openai", f.catalog.Snapshot().ID(), [4]float64{5, 6, 7, 8})

	// Zero is an explicit full replacement, not a missing segment or merge.
	cells, err = f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff("matrix-a", "PROVIDER/SHARED-MODEL", "openai", [4]float64{}),
	})
	if err != nil || len(cells) != 1 {
		t.Fatalf("replace with zero tariff: %+v %v", cells, err)
	}
	assertMatrixCell(t, cells[0], "matrix-a", "provider/shared-model", "openai", f.catalog.Snapshot().ID(), [4]float64{})

	f = f.reopen(t)
	page, err := f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil || len(page.Cells) != 2 || page.TotalCredentials != 2 || page.TotalModels != 1 || !reflect.DeepEqual(page.Models, []string{"provider/shared-model"}) {
		t.Fatalf("restart did not recover independent sparse tariffs: %+v %v", page, err)
	}
	assertMatrixCell(t, page.Cells[0], "matrix-a", "provider/shared-model", "openai", f.catalog.Snapshot().ID(), [4]float64{})
	assertMatrixCell(t, page.Cells[1], "matrix-b", "provider/shared-model", "openai", f.catalog.Snapshot().ID(), [4]float64{5, 6, 7, 8})

	for range 2 {
		cells, err = f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{{SubjectID: " matrix-a ", CanonicalModelID: " Provider/Shared-Model ", Clear: true}})
		if err != nil || len(cells) != 1 || cells[0].Covered || cells[0].Mode != "inherit" || cells[0].SubjectID != "matrix-a" || cells[0].CanonicalModelID != "provider/shared-model" || cells[0].SnapshotID != f.catalog.Snapshot().ID() {
			t.Fatalf("idempotent clear: %+v %v", cells, err)
		}
		if cells[0].PromptPricePer1M != nil || cells[0].CompletionPricePer1M != nil || cells[0].CacheReadPricePer1M != nil || cells[0].CacheWritePricePer1M != nil {
			t.Fatalf("cleared cell retained tariff segments: %+v", cells[0])
		}
	}
	f = f.reopen(t)
	page, err = f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil || len(page.Cells) != 1 {
		t.Fatalf("clear did not survive restart: %+v %v", page, err)
	}
	assertMatrixCell(t, page.Cells[0], "matrix-b", "provider/shared-model", "openai", f.catalog.Snapshot().ID(), [4]float64{5, 6, 7, 8})
	persisted, err := repository.ListCredentialModelPriceOverrides(f.db)
	if err != nil || len(persisted) != 1 || persisted[0].SubjectID != "matrix-b" {
		t.Fatalf("clear removed the wrong DB pair: %+v %v", persisted, err)
	}
}

func TestCredentialMatrixInvalidSegmentRejectsWholeBatch(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	ctx := context.Background()
	if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff("matrix-a", "existing", "openai", [4]float64{1, 2, 3, 4}),
		matrixTariff("matrix-b", "existing", "claude", [4]float64{5, 6, 7, 8}),
	}); err != nil {
		t.Fatal(err)
	}
	previous := f.catalog.Snapshot()
	rows, err := repository.ListCredentialModelPriceOverrides(f.db)
	if err != nil {
		t.Fatal(err)
	}
	for segment, name := range []string{"prompt", "completion", "cache-read", "cache-write"} {
		for _, invalid := range []struct {
			name  string
			value *float64
		}{
			{"missing", nil}, {"negative", new(-1.0)}, {"nan", new(math.NaN())},
			{"positive-inf", new(math.Inf(1))}, {"negative-inf", new(math.Inf(-1))},
		} {
			t.Run(name+"/"+invalid.name, func(t *testing.T) {
				bad := matrixTariff("matrix-a", "new-model", "openai", [4]float64{10, 20, 30, 40})
				fields := []**float64{&bad.PromptPricePer1M, &bad.CompletionPricePer1M, &bad.CacheReadPricePer1M, &bad.CacheWritePricePer1M}
				*fields[segment] = invalid.value
				_, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
					matrixTariff("matrix-a", "existing", "claude", [4]float64{9, 10, 11, 12}),
					{SubjectID: "matrix-b", CanonicalModelID: "existing", Clear: true},
					bad,
				})
				if !errors.Is(err, service.ErrInvalidPricingInput) {
					t.Fatalf("invalid segment accepted: %v", err)
				}
				assertMatrixStateUnchanged(t, f, previous, rows)
			})
		}
	}
}

func TestCredentialMatrixCanonicalDuplicateRejectsWholeBatch(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	ctx := context.Background()
	if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff("matrix-a", "provider/shared-model", "openai", [4]float64{1, 2, 3, 4}),
	}); err != nil {
		t.Fatal(err)
	}
	previous := f.catalog.Snapshot()
	rows, err := repository.ListCredentialModelPriceOverrides(f.db)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		clearFirst  bool
		clearSecond bool
	}{
		{name: "two-upserts"}, {name: "upsert-then-clear", clearSecond: true}, {name: "clear-then-upsert", clearFirst: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			first := matrixTariff("matrix-a", "provider/shared-model", "openai", [4]float64{9, 10, 11, 12})
			second := matrixTariff(" matrix-a ", " Provider/SHARED-MODEL ", "claude", [4]float64{13, 14, 15, 16})
			if test.clearFirst {
				first = service.CredentialMatrixCellInput{SubjectID: first.SubjectID, CanonicalModelID: first.CanonicalModelID, Clear: true}
			}
			if test.clearSecond {
				second = service.CredentialMatrixCellInput{SubjectID: second.SubjectID, CanonicalModelID: second.CanonicalModelID, Clear: true}
			}
			if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{first, second}); !errors.Is(err, service.ErrInvalidPricingInput) {
				t.Fatalf("canonical duplicate accepted: %v", err)
			}
			assertMatrixStateUnchanged(t, f, previous, rows)
		})
	}
}

func TestCredentialMatrix5000TariffsSucceed5001RejectsAtomically(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	// Avoid dumping thousands of SQL parameters if the actual boundary fails.
	f.db.Logger = logger.Default.LogMode(logger.Silent)
	ctx := context.Background()
	inputs := make([]service.CredentialMatrixCellInput, 5000)
	for index := range inputs {
		inputs[index] = matrixTariff("matrix-a", fmt.Sprintf("model-%04d", index), "openai", [4]float64{1, 2, 3, 4})
	}
	cells, err := f.matrix.UpdateCredentialModelMatrix(ctx, inputs)
	if err != nil || len(cells) != 5000 {
		t.Fatalf("5000 actual complete tariff upserts must succeed: cells=%d err=%v", len(cells), err)
	}
	previous := f.catalog.Snapshot()
	for index, cell := range cells {
		assertMatrixCell(t, cell, "matrix-a", fmt.Sprintf("model-%04d", index), "openai", previous.ID(), [4]float64{1, 2, 3, 4})
	}
	rows, err := repository.ListCredentialModelPriceOverrides(f.db)
	if err != nil || len(rows) != 5000 || len(previous.CredentialMatrixConfigs()) != 5000 {
		t.Fatalf("5000 tariffs were not persisted and published: rows=%d err=%v", len(rows), err)
	}
	// At least one existing cell would change if the over-limit batch ran.
	inputs[0] = matrixTariff("matrix-a", "model-0000", "claude", [4]float64{9, 10, 11, 12})
	inputs = append(inputs, matrixTariff("matrix-a", "model-5000", "openai", [4]float64{1, 2, 3, 4}))
	if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, inputs); !errors.Is(err, service.ErrCredentialMatrixBatchTooLarge) {
		t.Fatalf("5001 tariffs must be rejected: %v", err)
	}
	assertMatrixStateUnchanged(t, f, previous, rows)
	f = f.reopen(t)
	page, err := f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{ModelPage: 100})
	if err != nil || page.TotalModels != 5000 || len(page.Cells) != 50 || page.Cells[0].CanonicalModelID != "model-4950" || page.Cells[49].CanonicalModelID != "model-4999" {
		t.Fatalf("restart must recover the final page of all 5000 tariffs: %+v %v", page, err)
	}
}

func TestCredentialMatrixDeferredCommitFailureKeepsDatabaseAndCatalog(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	ctx := context.Background()
	if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff("matrix-a", "existing", "openai", [4]float64{1, 2, 3, 4}),
		matrixTariff("matrix-b", "clear-me", "claude", [4]float64{5, 6, 7, 8}),
	}); err != nil {
		t.Fatal(err)
	}
	previous := f.catalog.Snapshot()
	rows, err := repository.ListCredentialModelPriceOverrides(f.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TABLE matrix_commit_probe (
		subject_id TEXT,
		FOREIGN KEY(subject_id) REFERENCES credential_pricing_subjects(id) DEFERRABLE INITIALLY DEFERRED
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`CREATE TRIGGER fail_matrix_commit AFTER UPDATE ON credential_model_price_overrides
		BEGIN INSERT INTO matrix_commit_probe(subject_id) VALUES ('absent-matrix-subject'); END`).Error; err != nil {
		t.Fatal(err)
	}
	// Seeing the changed rows in the candidate load proves DML succeeded and
	// the deferred constraint did not fail until the surrounding COMMIT.
	candidateLoaded := false
	const callback = "test:matrix-deferred-commit-candidate"
	if err := f.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if values, ok := tx.Statement.Dest.(*[]entities.CredentialModelPriceOverride); ok && tx.Error == nil {
			candidateLoaded = len(*values) == 2 && (*values)[0].SubjectID == "matrix-a" && (*values)[0].PromptPricePer1M == 9 && (*values)[1].CanonicalModelID == "new-model"
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Callback().Query().Remove(callback) })
	_, err = f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff("matrix-a", "existing", "claude", [4]float64{9, 10, 11, 12}),
		{SubjectID: "matrix-b", CanonicalModelID: "clear-me", Clear: true},
		matrixTariff("matrix-b", "new-model", "openai", [4]float64{13, 14, 15, 16}),
	})
	var constraint sqlite3.Error
	if !errors.As(err, &constraint) || constraint.ExtendedCode != sqlite3.ErrConstraintForeignKey || !candidateLoaded {
		t.Fatalf("expected real deferred FK COMMIT failure after candidate load: candidate=%v err=%v", candidateLoaded, err)
	}
	assertMatrixStateUnchanged(t, f, previous, rows)
	var probeCount int64
	if err := f.db.Table("matrix_commit_probe").Count(&probeCount).Error; err != nil || probeCount != 0 {
		t.Fatalf("failed commit did not roll back the trigger: count=%d err=%v", probeCount, err)
	}
	if err := f.db.Exec("DROP TRIGGER fail_matrix_commit").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{matrixTariff("matrix-a", "existing", "claude", [4]float64{9, 10, 11, 12})}); err != nil {
		t.Fatalf("service cannot recover after failed commit: %v", err)
	}
}

func TestCredentialMatrixConcurrentReadersSeeCompleteOldOrNewSnapshot(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	ctx := context.Background()
	if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff("matrix-a", "one", "openai", [4]float64{1, 2, 3, 4}),
		matrixTariff("matrix-b", "two", "claude", [4]float64{5, 6, 7, 8}),
	}); err != nil {
		t.Fatal(err)
	}
	old := f.catalog.Snapshot()
	oldPage, err := f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	stop := make(chan struct{})
	oldRead := make(chan struct{}, 6)
	failures := make(chan string, 6)
	matches := func(fixed pricing.FixedTariff, style string, values [4]float64) bool {
		actual := []*float64{fixed.PromptPricePer1M, fixed.CompletionPricePer1M, fixed.CacheReadPricePer1M, fixed.CacheWritePricePer1M}
		if fixed.PricingStyle != style {
			return false
		}
		for index, value := range actual {
			if value == nil || *value != values[index] {
				return false
			}
		}
		return true
	}
	checkSnapshot := func(snapshot *pricing.Snapshot) bool {
		configs := snapshot.CredentialMatrixConfigs()
		if len(configs) != 2 {
			return false
		}
		one, oneOK := snapshot.CredentialMatrixConfig("matrix-a", "one")
		two, twoOK := snapshot.CredentialMatrixConfig("matrix-b", "two")
		three, threeOK := snapshot.CredentialMatrixConfig("matrix-b", "three")
		isOld := oneOK && twoOK && !threeOK && matches(one.Fixed, "openai", [4]float64{1, 2, 3, 4}) && matches(two.Fixed, "claude", [4]float64{5, 6, 7, 8})
		isNew := oneOK && !twoOK && threeOK && matches(one.Fixed, "claude", [4]float64{9, 10, 11, 12}) && matches(three.Fixed, "openai", [4]float64{13, 14, 15, 16})
		return isOld || isNew
	}
	checkPage := func(page service.CredentialMatrixPage) bool {
		if len(page.Cells) != 2 || page.TotalModels != 2 || page.TotalCredentials != 2 || page.SnapshotID == "" {
			return false
		}
		if len(page.EffectiveCells) != 4 {
			return false
		}
		for _, effective := range page.EffectiveCells {
			if effective.SnapshotID != page.SnapshotID {
				return false
			}
			if effective.Covered {
				matched := false
				for _, sparse := range page.Cells {
					if sparse.SubjectID == effective.SubjectID && sparse.CanonicalModelID == effective.CanonicalModelID {
						matched = reflect.DeepEqual(effective.CredentialMatrixCell, sparse)
					}
				}
				if !matched || effective.Status != "complete" {
					return false
				}
			} else if effective.Status != "unavailable" || effective.PromptPricePer1M != nil {
				return false
			}
		}
		cellMatches := func(cell service.CredentialMatrixCell, subject, model, style string, values [4]float64) bool {
			fixed := pricing.FixedTariff{PricingStyle: cell.PricingStyle, PromptPricePer1M: cell.PromptPricePer1M, CompletionPricePer1M: cell.CompletionPricePer1M, CacheReadPricePer1M: cell.CacheReadPricePer1M, CacheWritePricePer1M: cell.CacheWritePricePer1M}
			return cell.SubjectID == subject && cell.CanonicalModelID == model && cell.Covered && cell.Mode == "fixed" && cell.Source == pricing.CredentialMatrixSource && cell.SnapshotID == page.SnapshotID && matches(fixed, style, values)
		}
		isOld := reflect.DeepEqual(page.Models, []string{"one", "two"}) && cellMatches(page.Cells[0], "matrix-a", "one", "openai", [4]float64{1, 2, 3, 4}) && cellMatches(page.Cells[1], "matrix-b", "two", "claude", [4]float64{5, 6, 7, 8})
		isNew := reflect.DeepEqual(page.Models, []string{"one", "three"}) && cellMatches(page.Cells[0], "matrix-a", "one", "claude", [4]float64{9, 10, 11, 12}) && cellMatches(page.Cells[1], "matrix-b", "three", "openai", [4]float64{13, 14, 15, 16})
		return isOld || isNew
	}
	for range 4 {
		wg.Go(func() {
			<-start
			if snapshot := f.catalog.Snapshot(); snapshot != old || !checkSnapshot(snapshot) {
				failures <- "reader did not start with the complete old snapshot"
				oldRead <- struct{}{}
				return
			}
			oldRead <- struct{}{}
			for {
				if !checkSnapshot(f.catalog.Snapshot()) {
					failures <- "reader observed a partial tariff or mixed batch"
					return
				}
				select {
				case <-stop:
					if snapshot := f.catalog.Snapshot(); snapshot == old || !checkSnapshot(snapshot) {
						failures <- "reader did not finish with the complete new snapshot"
					}
					return
				default:
				}
			}
		})
	}
	for range 2 {
		wg.Go(func() {
			<-start
			page, err := f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
			if err != nil || page.SnapshotID != old.ID() || !checkPage(page) {
				failures <- fmt.Sprintf("list reader did not start with a complete old page: %v", err)
				oldRead <- struct{}{}
				return
			}
			oldRead <- struct{}{}
			for {
				page, err = f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
				if err != nil || !checkPage(page) {
					failures <- fmt.Sprintf("list reader observed a mixed page: %v", err)
					return
				}
				select {
				case <-stop:
					page, err = f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
					if err != nil || page.SnapshotID == old.ID() || !checkPage(page) {
						failures <- fmt.Sprintf("list reader did not finish with a complete new page: %v", err)
					}
					return
				default:
				}
			}
		})
	}
	close(start)
	for range 6 {
		<-oldRead
	}
	cells, updateErr := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{
		matrixTariff("matrix-a", "one", "claude", [4]float64{9, 10, 11, 12}),
		{SubjectID: "matrix-b", CanonicalModelID: "two", Clear: true},
		matrixTariff("matrix-b", "three", "openai", [4]float64{13, 14, 15, 16}),
	})
	close(stop)
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	if updateErr != nil || len(cells) != 3 {
		t.Fatalf("concurrent batch: %+v %v", cells, updateErr)
	}
	oldOne, oldOneOK := old.CredentialMatrixConfig("matrix-a", "one")
	oldTwo, oldTwoOK := old.CredentialMatrixConfig("matrix-b", "two")
	if !oldOneOK || !oldTwoOK || !matches(oldOne.Fixed, "openai", [4]float64{1, 2, 3, 4}) || !matches(oldTwo.Fixed, "claude", [4]float64{5, 6, 7, 8}) {
		t.Fatal("publication changed the pinned old snapshot")
	}
	if f.catalog.Snapshot() == old || !checkSnapshot(f.catalog.Snapshot()) {
		t.Fatal("publication did not produce a complete new snapshot")
	}
	assertMatrixCell(t, cells[0], "matrix-a", "one", "claude", f.catalog.Snapshot().ID(), [4]float64{9, 10, 11, 12})
	if cells[1].Covered || cells[1].SnapshotID != cells[0].SnapshotID {
		t.Fatalf("clear was not published with the batch: %+v", cells[1])
	}
	assertMatrixCell(t, cells[2], "matrix-b", "three", "openai", cells[0].SnapshotID, [4]float64{13, 14, 15, 16})
	// Returned pages and pinned snapshots remain complete after publication.
	if len(oldPage.Cells) != 2 || oldPage.Models[0] != "one" || oldPage.Models[1] != "two" {
		t.Fatalf("previous page mutated: %+v", oldPage)
	}
	assertMatrixCell(t, oldPage.Cells[0], "matrix-a", "one", "openai", old.ID(), [4]float64{1, 2, 3, 4})
	page, err := f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil || len(page.Cells) != 2 || !reflect.DeepEqual(page.Models, []string{"one", "three"}) {
		t.Fatalf("new page is incomplete: %+v %v", page, err)
	}
	assertMatrixCell(t, page.Cells[0], "matrix-a", "one", "claude", f.catalog.Snapshot().ID(), [4]float64{9, 10, 11, 12})
	assertMatrixCell(t, page.Cells[1], "matrix-b", "three", "openai", f.catalog.Snapshot().ID(), [4]float64{13, 14, 15, 16})
	restarted, _ := newCatalogPricingService(t, f.db)
	persisted, err := restarted.(service.CredentialModelMatrixProvider).ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil || len(persisted.Cells) != 2 {
		t.Fatalf("DB disagrees with published batch: %+v %v", persisted, err)
	}
	assertMatrixCell(t, persisted.Cells[0], "matrix-a", "one", "claude", persisted.SnapshotID, [4]float64{9, 10, 11, 12})
	assertMatrixCell(t, persisted.Cells[1], "matrix-b", "three", "openai", persisted.SnapshotID, [4]float64{13, 14, 15, 16})
}
