package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	CredentialMatrixDefaultCredentialPageSize = 50
	CredentialMatrixDefaultModelPageSize      = 50
	CredentialMatrixMaxCells                  = 5000
)

var ErrCredentialMatrixBatchTooLarge = errors.New("credential matrix batch exceeds limit")

type CredentialMatrixQuery struct {
	CredentialPage     int
	CredentialPageSize int
	ModelPage          int
	ModelPageSize      int
	CredentialFilter   string
	ModelFilter        string
	allowedPairs       map[credentialMatrixPair]bool
	viewerSecrets      []string
}

type CredentialMatrixCellInput struct {
	SubjectID            string
	CanonicalModelID     string
	Clear                bool
	PricingStyle         string
	PromptPricePer1M     *float64
	CompletionPricePer1M *float64
	CacheReadPricePer1M  *float64
	CacheWritePricePer1M *float64
}

type CredentialMatrixCell struct {
	SubjectID            string                 `json:"subject_id"`
	Credential           *dto.PricingCredential `json:"credential"`
	CanonicalModelID     string                 `json:"canonical_model_id"`
	Covered              bool                   `json:"covered"`
	Mode                 string                 `json:"mode"`
	PricingStyle         string                 `json:"pricing_style,omitempty"`
	PromptPricePer1M     *float64               `json:"prompt_price_per_1m,omitempty"`
	CompletionPricePer1M *float64               `json:"completion_price_per_1m,omitempty"`
	CacheReadPricePer1M  *float64               `json:"cache_read_price_per_1m,omitempty"`
	CacheWritePricePer1M *float64               `json:"cache_write_price_per_1m,omitempty"`
	Source               string                 `json:"source,omitempty"`
	SnapshotID           string                 `json:"snapshot_id"`
}

type CredentialMatrixPage struct {
	Credentials        []dto.PricingCredential         `json:"credentials"`
	Models             []string                        `json:"models"`
	Cells              []CredentialMatrixCell          `json:"cells"`
	EffectiveCells     []CredentialMatrixEffectiveCell `json:"effective_cells"`
	CredentialPage     int                             `json:"credential_page"`
	CredentialPageSize int                             `json:"credential_page_size"`
	ModelPage          int                             `json:"model_page"`
	ModelPageSize      int                             `json:"model_page_size"`
	TotalCredentials   int                             `json:"total_credentials"`
	TotalModels        int                             `json:"total_models"`
	SnapshotID         string                          `json:"snapshot_id"`
}

type CredentialModelMatrixProvider interface {
	ListCredentialModelMatrix(context.Context, CredentialMatrixQuery) (CredentialMatrixPage, error)
	UpdateCredentialModelMatrix(context.Context, []CredentialMatrixCellInput) ([]CredentialMatrixCell, error)
}

func (s *pricingService) ListCredentialModelMatrix(ctx context.Context, query CredentialMatrixQuery) (CredentialMatrixPage, error) {
	snapshot := s.catalog.Snapshot()
	if query.CredentialPage < 1 {
		query.CredentialPage = 1
	}
	if query.ModelPage < 1 {
		query.ModelPage = 1
	}
	if query.CredentialPageSize < 1 {
		query.CredentialPageSize = CredentialMatrixDefaultCredentialPageSize
	}
	if query.ModelPageSize < 1 {
		query.ModelPageSize = CredentialMatrixDefaultModelPageSize
	}
	if query.CredentialPageSize > CredentialMatrixDefaultCredentialPageSize {
		query.CredentialPageSize = CredentialMatrixDefaultCredentialPageSize
	}
	if query.ModelPageSize > CredentialMatrixDefaultModelPageSize {
		query.ModelPageSize = CredentialMatrixDefaultModelPageSize
	}

	credentials := []dto.PricingCredential{}
	var secrets []string
	if err := repository.ReadCredentialPricingDirectory(ctx, s.db, func(identities []entities.UsageIdentity, subjects []entities.CredentialPricingSubject) error {
		secrets = append(credentialMatrixSecrets(identities, subjects), query.viewerSecrets...)
		allowedSubjects := make(map[string]bool)
		for pair := range query.allowedPairs {
			allowedSubjects[pair.subjectID] = true
		}
		seen := make(map[string]bool, len(subjects))
		for _, subject := range subjects {
			if seen[subject.ID] || (query.allowedPairs != nil && !allowedSubjects[subject.ID]) {
				continue
			}
			seen[subject.ID] = true
			item := dto.PricingCredential{SubjectID: subject.ID, Name: "Credential", ProviderType: "unknown", AuthType: "unknown", Status: "stale", BindingStatus: "unbound"}
			for _, identity := range identities {
				if identity.AuthType == subject.AuthType && identity.Identity == subject.Identity {
					candidate := pricingCredential(identity, subjects)
					if candidate.SubjectID == subject.ID && candidate.BindingStatus == "bound" && !subject.BindingDisabled && identity.BindingIdentityStatus == "unique" && identity.AuthTypeName == subject.AuthTypeName {
						item = candidate
					}
					break
				}
			}
			if !safeCredentialMatrixText(item.Name, secrets) {
				item.Name = "Credential"
			}
			if !safeCredentialMatrixText(item.Alias, secrets) {
				item.Alias = ""
			}
			if !safeCredentialMatrixText(item.Endpoint, secrets) {
				item.Endpoint = ""
			}
			if query.allowedPairs != nil {
				item.KeyHint, item.Endpoint = "", ""
				item.DirectoryID = 0
			}
			credentials = append(credentials, item)
		}
		return nil
	}); err != nil {
		return CredentialMatrixPage{}, err
	}
	filter := strings.ToLower(strings.TrimSpace(query.CredentialFilter))
	filteredCredentials := credentials[:0]
	for _, credential := range credentials {
		if filter == "" || strings.Contains(strings.ToLower(credential.Name), filter) || strings.Contains(strings.ToLower(credential.Alias), filter) || strings.Contains(strings.ToLower(credential.ProviderType), filter) || strings.Contains(strings.ToLower(credential.Status), filter) {
			filteredCredentials = append(filteredCredentials, credential)
		}
	}
	credentials = filteredCredentials
	sort.Slice(credentials, func(i, j int) bool { return credentials[i].SubjectID < credentials[j].SubjectID })
	modelsSet := map[string]struct{}{}
	localModels, err := repository.ListUsedModels(s.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}))
	if err != nil {
		return CredentialMatrixPage{}, err
	}
	if s.modelsFetcher != nil {
		// Upstream failure leaves historical and persisted model choices available.
		// Never log transport errors, which may contain credential-bearing URLs.
		if result, err := s.modelsFetcher.FetchModels(ctx); err == nil {
			localModels = append(localModels, extractCPAModelIDs(result)...)
		}
	}
	for _, config := range snapshot.ModelConfigs() {
		localModels = append(localModels, config.Pricing.Model)
	}
	for _, model := range localModels {
		if canonical := pricing.CanonicalCredentialModelID(model); canonical != "" {
			modelsSet[canonical] = struct{}{}
		}
	}
	for _, price := range snapshot.OfficialPrices() {
		modelsSet[pricing.CanonicalCredentialModelID(price.CanonicalModelID)] = struct{}{}
	}
	matrix := snapshot.CredentialMatrixConfigs()
	for _, config := range matrix {
		modelsSet[config.CanonicalModelID] = struct{}{}
	}
	if query.allowedPairs != nil {
		modelsSet = make(map[string]struct{})
		for pair := range query.allowedPairs {
			modelsSet[pair.model] = struct{}{}
		}
	}
	models := make([]string, 0, len(modelsSet))
	for model := range modelsSet {
		if !safeCredentialMatrixModel(model, secrets) {
			continue
		}
		if query.ModelFilter == "" || strings.Contains(strings.ToLower(model), strings.ToLower(strings.TrimSpace(query.ModelFilter))) {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	credentialsPage := paginateCredentials(credentials, query.CredentialPage, query.CredentialPageSize)
	modelsPage := paginateStrings(models, query.ModelPage, query.ModelPageSize)
	allowedSubjects, allowedModels := map[string]bool{}, map[string]bool{}
	for _, item := range credentialsPage {
		allowedSubjects[item.SubjectID] = true
	}
	for _, model := range modelsPage {
		allowedModels[model] = true
	}
	cells := make([]CredentialMatrixCell, 0)
	for _, config := range snapshot.CredentialMatrixConfigs() {
		if !allowedSubjects[config.SubjectID] || !allowedModels[config.CanonicalModelID] || (query.allowedPairs != nil && !query.allowedPairs[credentialMatrixPair{config.SubjectID, config.CanonicalModelID}]) {
			continue
		}
		cell := matrixCellFromConfig(config, snapshot.ID())
		for index := range credentialsPage {
			if credentialsPage[index].SubjectID == config.SubjectID {
				cell.Credential = &credentialsPage[index]
				break
			}
		}
		cells = append(cells, cell)
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].SubjectID == cells[j].SubjectID {
			return cells[i].CanonicalModelID < cells[j].CanonicalModelID
		}
		return cells[i].SubjectID < cells[j].SubjectID
	})
	return CredentialMatrixPage{Credentials: credentialsPage, Models: modelsPage, Cells: cells, EffectiveCells: matrixEffectiveCells(snapshot, credentialsPage, modelsPage, query.allowedPairs, secrets), CredentialPage: query.CredentialPage, CredentialPageSize: query.CredentialPageSize, ModelPage: query.ModelPage, ModelPageSize: query.ModelPageSize, TotalCredentials: len(credentials), TotalModels: len(models), SnapshotID: snapshot.ID()}, nil
}

func (s *pricingService) UpdateCredentialModelMatrix(ctx context.Context, inputs []CredentialMatrixCellInput) ([]CredentialMatrixCell, error) {
	if len(inputs) > CredentialMatrixMaxCells {
		return nil, fmt.Errorf("%w: maximum is %d", ErrCredentialMatrixBatchTooLarge, CredentialMatrixMaxCells)
	}
	if len(inputs) == 0 {
		return []CredentialMatrixCell{}, nil
	}
	rows := make([]entities.CredentialModelPriceOverride, 0, len(inputs))
	keys := make(map[string]struct{}, len(inputs))
	for index, input := range inputs {
		subject := strings.TrimSpace(input.SubjectID)
		model := pricing.CanonicalCredentialModelID(input.CanonicalModelID)
		if subject == "" || model == "" {
			return nil, fmt.Errorf("%w: cell %d requires subject_id and canonical_model_id", ErrInvalidPricingInput, index)
		}
		key := subject + "\x00" + model
		if _, exists := keys[key]; exists {
			return nil, fmt.Errorf("%w: duplicate cell", ErrInvalidPricingInput)
		}
		keys[key] = struct{}{}
		if input.Clear {
			if input.PricingStyle != "" || input.PromptPricePer1M != nil || input.CompletionPricePer1M != nil || input.CacheReadPricePer1M != nil || input.CacheWritePricePer1M != nil {
				return nil, fmt.Errorf("%w: clear cannot contain a fixed tariff", ErrInvalidPricingInput)
			}
			continue
		}
		style := strings.ToLower(strings.TrimSpace(input.PricingStyle))
		if style == "" {
			style = entities.ModelPricingStyleOpenAI
		}
		if style != entities.ModelPricingStyleOpenAI && style != entities.ModelPricingStyleClaude {
			return nil, fmt.Errorf("%w: pricing_style must be openai or claude", ErrInvalidPricingInput)
		}
		values := []*float64{input.PromptPricePer1M, input.CompletionPricePer1M, input.CacheReadPricePer1M, input.CacheWritePricePer1M}
		for _, value := range values {
			if value == nil || *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
				return nil, fmt.Errorf("%w: complete finite non-negative fixed tariff required", ErrInvalidPricingInput)
			}
		}
		rows = append(rows, entities.CredentialModelPriceOverride{SubjectID: subject, CanonicalModelID: model, PricingStyle: style, PromptPricePer1M: *values[0], CompletionPricePer1M: *values[1], CacheReadPricePer1M: *values[2], CacheWritePricePer1M: *values[3], Source: pricing.CredentialMatrixSource})
	}
	candidate, err := s.mutatePricing(ctx, func(tx *gorm.DB) error {
		tx = tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		identities, subjects, err := repository.LoadCredentialPricingDirectory(tx)
		if err != nil {
			return err
		}
		secrets := credentialMatrixSecrets(identities, subjects)
		for _, input := range inputs {
			if !safeCredentialMatrixModel(pricing.CanonicalCredentialModelID(input.CanonicalModelID), secrets) {
				return ErrInvalidPricingInput
			}
			var count int64
			if err := tx.Model(&entities.CredentialPricingSubject{}).Where("id = ?", strings.TrimSpace(input.SubjectID)).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrPricingCredentialNotFound
			}
		}
		if err := repository.UpsertCredentialModelPriceOverrides(tx, rows); err != nil {
			return err
		}
		if err := repository.ActivateStrictPricing(tx); err != nil {
			return err
		}
		clears := make([]struct{ SubjectID, CanonicalModelID string }, 0)
		for _, input := range inputs {
			if input.Clear {
				clears = append(clears, struct{ SubjectID, CanonicalModelID string }{strings.TrimSpace(input.SubjectID), pricing.CanonicalCredentialModelID(input.CanonicalModelID)})
			}
		}
		if err := repository.DeleteCredentialModelPriceOverrides(tx, clears); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, repository.ErrInvalidPricingSnapshot) {
			return nil, fmt.Errorf("%w: invalid pricing snapshot", ErrInvalidPricingInput)
		}
		return nil, err
	}
	result := make([]CredentialMatrixCell, 0, len(inputs))
	for _, input := range inputs {
		if config, ok := candidate.CredentialMatrixConfig(strings.TrimSpace(input.SubjectID), input.CanonicalModelID); ok {
			result = append(result, matrixCellFromConfig(config, candidate.ID()))
		} else {
			result = append(result, CredentialMatrixCell{SubjectID: strings.TrimSpace(input.SubjectID), CanonicalModelID: pricing.CanonicalCredentialModelID(input.CanonicalModelID), Covered: false, Mode: "inherit", SnapshotID: candidate.ID()})
		}
	}
	return result, nil
}

func matrixCellFromConfig(config pricing.CredentialMatrixConfig, snapshotID string) CredentialMatrixCell {
	return CredentialMatrixCell{SubjectID: config.SubjectID, CanonicalModelID: config.CanonicalModelID, Covered: true, Mode: "fixed", PricingStyle: config.Fixed.PricingStyle, PromptPricePer1M: config.Fixed.PromptPricePer1M, CompletionPricePer1M: config.Fixed.CompletionPricePer1M, CacheReadPricePer1M: config.Fixed.CacheReadPricePer1M, CacheWritePricePer1M: config.Fixed.CacheWritePricePer1M, Source: config.Source, SnapshotID: snapshotID}
}
func paginateCredentials(items []dto.PricingCredential, page, size int) []dto.PricingCredential {
	if page < 1 || size < 1 || len(items) == 0 || page-1 > (len(items)-1)/size {
		return []dto.PricingCredential{}
	}
	start := (page - 1) * size
	if start >= len(items) {
		return []dto.PricingCredential{}
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
func paginateStrings(items []string, page, size int) []string {
	if page < 1 || size < 1 || len(items) == 0 || page-1 > (len(items)-1)/size {
		return []string{}
	}
	start := (page - 1) * size
	if start >= len(items) {
		return []string{}
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
