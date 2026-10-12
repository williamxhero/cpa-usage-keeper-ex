package service

import (
	"context"
	"fmt"

	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service/providermetadata"
	"cpa-usage-keeper/internal/timeutil"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// SyncMetadata 同步三类 CPA metadata，并只在三类数据库写入都成功后通知后台 identity 聚合。
func (s *SyncService) SyncMetadata(ctx context.Context) error {
	return s.syncMetadata(ctx, false)
}

// SyncCredentialDirectory performs the complete credential-directory refresh.
// Unlike the legacy metadata path, any source failure aborts before publication,
// preserving the last successful directory and its pricing subjects.
func (s *SyncService) SyncCredentialDirectory(ctx context.Context) error {
	return s.syncMetadata(ctx, true)
}

func (s *SyncService) syncMetadata(ctx context.Context, completeCredentialDirectory bool) error {
	// metadata 入口仍复用原 validate，并保留老构造器的 client 回填行为。
	if err := s.validate(syncMetadataRequired); err != nil {
		// 配置不完整时不发起任何远端读取。
		return err
	}
	// 开始日志保持原有 debug 级别和 message。
	logrus.Debug("metadata sync started")
	// 同一轮 metadata 写入与兼容同步补算共用该时间；后台 Runner 使用自己的事务时间。
	fetchedAt := timeutil.NormalizeStorageTime(s.now())
	// Auth Files 先读取，但失败不跳过后续两类 metadata。
	authFilesResult, authFilesErr := s.metadataFetcher.FetchAuthFiles(ctx)
	// 管理 API Keys 保持第二个读取位置，失败同样不阻止 provider。
	apiKeysResult, apiKeysErr := s.metadataFetcher.FetchManagementAPIKeys(ctx)
	// 八个 provider endpoint 只在纯包内并发，返回按 registry 确定性归并的 snapshot。
	providerSnapshot, providerFetchErr := providermetadata.Fetch(ctx, s.metadataFetcher)
	if completeCredentialDirectory {
		if authFilesErr == nil && authFilesResult == nil {
			authFilesErr = fmt.Errorf("empty response")
		}
		if apiKeysErr == nil && apiKeysResult == nil {
			apiKeysErr = fmt.Errorf("empty response")
		}
	}
	if completeCredentialDirectory && (authFilesErr != nil || apiKeysErr != nil || providerFetchErr != nil) {
		// A credential refresh is authoritative only when every source scope
		// completed. Do not enter any persistence transaction on a partial fetch.
		return joinErrors(
			wrapMetadataFetchError("fetch auth files", authFilesErr),
			wrapMetadataFetchError("fetch management api keys", apiKeysErr),
			wrapMetadataFetchError("fetch provider metadata", providerFetchErr),
		)
	}
	// Fetching does not hold the pricing writer lock. Relevant committed metadata
	// and pricing publication are serialized with all admin pricing mutations.
	if s.pricingCatalog != nil {
		s.pricingCatalog.MutationMutex().Lock()
	}
	var authSyncErr, apiKeySyncErr, providerSyncErr, providerWarningErr error
	if completeCredentialDirectory {
		// Complete credential publication uses one outer writer transaction. The
		// existing scoped helpers remain unchanged for legacy metadata callers.
		transactionErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			authSyncErr = syncAuthFilesInTransaction(ctx, tx, authFilesResult, authFilesErr, fetchedAt)
			if authSyncErr != nil {
				return authSyncErr
			}
			apiKeySyncErr = syncManagementAPIKeysInTransaction(tx, apiKeysResult, apiKeysErr, fetchedAt)
			if apiKeySyncErr != nil {
				return apiKeySyncErr
			}
			providerSyncErr, providerWarningErr = persistProviderMetadataInTransaction(ctx, tx, providerSnapshot, providerFetchErr, fetchedAt)
			if providerSyncErr != nil {
				return providerSyncErr
			}
			return repository.EnsureCredentialPricingSubjects(tx, fetchedAt)
		})
		if transactionErr != nil && authSyncErr == nil && apiKeySyncErr == nil && providerSyncErr == nil {
			providerSyncErr = transactionErr
		}
	} else {
		// Legacy metadata callers retain the existing independent scope semantics.
		authSyncErr = syncAuthFiles(ctx, s.db, authFilesResult, authFilesErr, fetchedAt)
		apiKeySyncErr = syncManagementAPIKeys(s.db, apiKeysResult, apiKeysErr, fetchedAt)
		providerSyncErr, providerWarningErr = persistProviderMetadata(ctx, s.db, providerSnapshot, providerFetchErr, fetchedAt)
	}
	// 三类持久化错误按 Auth Files、管理 key、provider 的既有顺序合并。
	upsertErr := joinErrors(authSyncErr, apiKeySyncErr, providerSyncErr)
	if s.pricingCatalog != nil {
		var pricingErr error
		if !completeCredentialDirectory || upsertErr == nil {
			// Publish only the committed directory. Complete refresh failures retain
			// the previous in-memory pricing snapshot and its credential ownership.
			snapshot, loadErr := repository.LoadPricingSnapshot(ctx, s.db)
			pricingErr = loadErr
			if pricingErr == nil {
				s.pricingCatalog.Replace(snapshot)
			} else if !completeCredentialDirectory {
				s.pricingCatalog.Replace(s.pricingCatalog.Snapshot().WithoutCredentialAttribution())
			}
		}
		s.pricingCatalog.MutationMutex().Unlock()
		if pricingErr != nil {
			upsertErr = joinErrors(upsertErr, fmt.Errorf("refresh pricing identity evidence failed"))
		}
	}
	// aggregateErr 只承接没有 notifier 的兼容同步补算错误。
	var aggregateErr error
	// 任一数据库写入失败都阻止基于半成品 identity 发送通知或执行兼容补算。
	if upsertErr == nil {
		// provider fetch warning 不属于持久化失败，因此仍允许已提交 identity 唤醒后台 runner。
		if s.usageAggregation != nil {
			// notifier 只写内存并发 wake，不在 metadata 热路径执行 SQLite 聚合事务。
			s.usageAggregation.NotifyUsageIdentitiesChanged()
		} else {
			// 没有 notifier 的既有构造路径继续在返回前补算历史 identity 事件。
			aggregateErr = repository.AggregateUsageIdentityStats(ctx, s.db, fetchedAt)
			// 错误包装保持 main 对兼容调用方的稳定文本。
			if aggregateErr != nil {
				aggregateErr = fmt.Errorf("aggregate usage identity stats: %w", aggregateErr)
			}
		}
	}
	// 最终顺序保持 persistence、兼容 aggregate、provider warning；生产后台聚合仍有独立生命周期。
	err := joinErrors(upsertErr, aggregateErr, providerWarningErr)
	// 完成日志默认沿用 completed 状态。
	fields := logrus.Fields{
		// status 字段保持现有 dashboard/runtime 观察语义。
		"status": "completed",
	}
	// 任一 warning 或错误都只改变完成状态，不回滚已提交的独立事务。
	if err != nil {
		// completed_with_warnings 保持原有非 fail-fast 表达。
		fields["status"] = "completed_with_warnings"
		// error 只写稳定合并后的错误；provider client 已保证不泄漏 response body。
		fields["error"] = err.Error()
	}
	// 完成日志保持原有 debug 级别和 message。
	logrus.WithFields(fields).Debug("metadata sync finished")
	// 返回全部可观察错误，同时保留本轮已提交数据。
	return err
}

func wrapMetadataFetchError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
