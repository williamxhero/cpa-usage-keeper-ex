package test

import (
	"context"
	"encoding/json"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
)

// Decode the public response contract, independently of its Go representation.
func effectiveMatrixCells(t *testing.T, page service.CredentialMatrixPage) []struct {
	SubjectID    string   `json:"subject_id"`
	Model        string   `json:"canonical_model_id"`
	Covered      bool     `json:"covered"`
	Mode         string   `json:"mode"`
	Status       string   `json:"status"`
	Source       string   `json:"source"`
	Reason       string   `json:"unavailable_reason"`
	MatchedModel string   `json:"matched_model"`
	SnapshotID   string   `json:"snapshot_id"`
	Prompt       *float64 `json:"prompt_price_per_1m"`
	Completion   *float64 `json:"completion_price_per_1m"`
	Read         *float64 `json:"cache_read_price_per_1m"`
	Write        *float64 `json:"cache_write_price_per_1m"`
} {
	t.Helper()
	data, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		EffectiveCells []struct {
			SubjectID    string   `json:"subject_id"`
			Model        string   `json:"canonical_model_id"`
			Covered      bool     `json:"covered"`
			Mode         string   `json:"mode"`
			Status       string   `json:"status"`
			Source       string   `json:"source"`
			Reason       string   `json:"unavailable_reason"`
			MatchedModel string   `json:"matched_model"`
			SnapshotID   string   `json:"snapshot_id"`
			Prompt       *float64 `json:"prompt_price_per_1m"`
			Completion   *float64 `json:"completion_price_per_1m"`
			Read         *float64 `json:"cache_read_price_per_1m"`
			Write        *float64 `json:"cache_write_price_per_1m"`
		} `json:"effective_cells"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	return response.EffectiveCells
}

func TestMatrixEffectivePricesPreserveSparseCoverageAndSource(t *testing.T) {
	f := newCredentialMatrixFixture(t)
	ctx := context.Background()
	number := func(v float64) *float64 { return &v }
	prices := []entities.OfficialModelPrice{
		{CanonicalModelID: "claude-sonnet-5-5", DisplayModelID: "Sonnet", Source: "official", PricingStyle: "claude", PromptPricePer1M: number(2), CompletionPricePer1M: number(10), CacheReadPricePer1M: number(.10), CacheWritePricePer1M: number(2.5)},
		{CanonicalModelID: "partial", DisplayModelID: "partial", Source: "official", PricingStyle: "openai", PromptPricePer1M: number(3)},
		{CanonicalModelID: "empty", DisplayModelID: "empty", Source: "official", PricingStyle: "openai"},
		{CanonicalModelID: "zero", DisplayModelID: "zero", Source: "openrouter", PricingStyle: "openai", PromptPricePer1M: number(0), CompletionPricePer1M: number(0), CacheReadPricePer1M: number(0), CacheWritePricePer1M: number(0)},
	}
	if err := f.db.Create(&prices).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.LoadPricingSnapshot(ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.catalog.Replace(snapshot)
	if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{matrixTariff("matrix-a", "claude-sonnet-5-5", "claude", [4]float64{2, 10, .20, 2.5})}); err != nil {
		t.Fatal(err)
	}
	page, err := f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{})
	if err != nil {
		t.Fatal(err)
	}
	cells := effectiveMatrixCells(t, page)
	if len(page.Cells) != 1 || len(cells) != 8 {
		t.Fatalf("sparse covers=%d, effective=%d; want 1/8", len(page.Cells), len(cells))
	}
	for _, cell := range cells {
		if cell.SnapshotID != page.SnapshotID {
			t.Fatal("mixed snapshots")
		}
		switch cell.Model {
		case "claude-sonnet-5-5":
			want, source := .10, "official"
			if cell.SubjectID == "matrix-a" {
				want, source = .20, "credential_matrix"
			}
			if cell.Read == nil || *cell.Read != want || cell.Source != source || cell.Status != "complete" || cell.Covered != (cell.SubjectID == "matrix-a") {
				t.Fatalf("provider/official mixed: %+v", cell)
			}
		case "partial":
			if cell.Status != "partial" || cell.Prompt == nil || *cell.Prompt != 3 || cell.Read != nil || cell.Write != nil || cell.Completion != nil {
				t.Fatalf("missing segment invented: %+v", cell)
			}
		case "empty":
			if cell.Status != "unavailable" || cell.Prompt != nil || cell.Read != nil {
				t.Fatalf("empty tariff is not unavailable: %+v", cell)
			}
		case "zero":
			if cell.Status != "complete" || cell.Source != "openrouter" || cell.Read == nil || *cell.Read != 0 {
				t.Fatalf("explicit zero lost: %+v", cell)
			}
		}
	}
	if _, err := f.matrix.UpdateCredentialModelMatrix(ctx, []service.CredentialMatrixCellInput{{SubjectID: "matrix-a", CanonicalModelID: "claude-sonnet-5-5", Clear: true}}); err != nil {
		t.Fatal(err)
	}
	page, err = f.matrix.ListCredentialModelMatrix(ctx, service.CredentialMatrixQuery{CredentialPageSize: 1, ModelFilter: "sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	cells = effectiveMatrixCells(t, page)
	if len(page.Cells) != 0 || len(cells) != 1 || cells[0].Covered || cells[0].Mode != "inherit" || cells[0].Read == nil || *cells[0].Read != .10 {
		t.Fatalf("clear should inherit official within page: %+v", page)
	}
}
