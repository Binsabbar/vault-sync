package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"vault-sync/internal/repository"
	"vault-sync/internal/service/job"
	"vault-sync/internal/service/pathmatching"
	"vault-sync/internal/vault"
	"vault-sync/pkg/log"
)

type SyncResult struct {
	TotalSecrets    int
	SuccessfulSyncs int
	FailedSyncs     int
	SkippedSecrets  int
	NoOpSecrets     int
	Duration        time.Duration
	JobResults      []*job.SyncJobResult
}

type SyncOrchestrator struct {
	logger      zerolog.Logger
	vaultClient vault.Syncer
	pathMatcher pathmatching.PathMatcher
	dbClient    repository.SyncedSecretRepository
	concurrency int
}

func NewSyncOrchestrator(
	vaultClient vault.Syncer,
	dbClient repository.SyncedSecretRepository,
	pathMatcher pathmatching.PathMatcher,
	concurrency int,
) *SyncOrchestrator {
	return &SyncOrchestrator{
		logger:      log.Logger.With().Str("component", "orchestrator").Logger(),
		vaultClient: vaultClient,
		pathMatcher: pathMatcher,
		dbClient:    dbClient,
		concurrency: concurrency,
	}
}

func (o *SyncOrchestrator) StartSync(ctx context.Context) (*SyncResult, error) {
	startTime := time.Now()
	o.logger.Info().Msg("Starting secret synchronization")

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	secretPaths := o.discoverSecrets(ctx)

	if len(secretPaths) == 0 {
		return o.emptyResult(startTime), nil
	}

	result := o.executeSyncJobs(ctx, secretPaths)
	result.Duration = time.Since(startTime)

	o.logSummary(result)

	// Check if sync was interrupted
	if ctx.Err() != nil {
		return result, fmt.Errorf("sync interrupted: %w", ctx.Err())
	}

	return result, nil
}

func (o *SyncOrchestrator) discoverSecrets(ctx context.Context) []pathmatching.SecretPath {
	o.logger.Info().Msg("Discovering secrets to sync")
	//FIX: current impleentation of DiscoverSecretsForSync swallows errors
	secretPaths, _ := o.pathMatcher.DiscoverSecretsForSync(ctx)
	o.logger.Info().Int("total_secrets", len(secretPaths)).Msg("Discovered secrets")
	return secretPaths
}

func (o *SyncOrchestrator) emptyResult(startTime time.Time) *SyncResult {
	o.logger.Warn().Msg("No secrets found to sync")
	return &SyncResult{
		TotalSecrets: 0,
		Duration:     time.Since(startTime),
	}
}

func (o *SyncOrchestrator) executeSyncJobs(
	ctx context.Context,
	secretPaths []pathmatching.SecretPath,
) *SyncResult {
	concurrency := o.concurrency
	o.logger.Info().
		Int("concurrency", concurrency).
		Int("total_secrets", len(secretPaths)).
		Msg("Starting concurrent sync jobs")

	result := &SyncResult{
		TotalSecrets: len(secretPaths),
		JobResults:   make([]*job.SyncJobResult, 0, len(secretPaths)),
	}

	jobResults := o.runJobsInParallel(ctx, secretPaths, concurrency)
	o.collectResults(result, jobResults)

	return result
}

func (o *SyncOrchestrator) runJobsInParallel(
	ctx context.Context,
	secretPaths []pathmatching.SecretPath,
	concurrency int,
) chan *job.SyncJobResult {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, concurrency)
	jobResults := make(chan *job.SyncJobResult, len(secretPaths))

	for _, secret := range secretPaths {
		wg.Add(1)
		go o.executeJob(ctx, secret, &wg, semaphore, jobResults)
	}

	go func() {
		wg.Wait()
		close(jobResults)
	}()

	return jobResults
}

// executeJob runs a single sync job.
func (o *SyncOrchestrator) executeJob(
	ctx context.Context,
	secret pathmatching.SecretPath,
	wg *sync.WaitGroup,
	semaphore chan struct{},
	jobResults chan *job.SyncJobResult,
) {
	defer wg.Done()

	cancelJob := func(secret pathmatching.SecretPath, err error) {
		jobResults <- &job.SyncJobResult{Mount: secret.Mount, KeyPath: secret.KeyPath, Error: err}
	}

	select {
	case <-ctx.Done():
		cancelJob(secret, ctx.Err())
		return
	default:
		// Context is still valid, proceed
	}

	// Acquire semaphore with context awareness
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
	case <-ctx.Done():
		cancelJob(secret, ctx.Err())
		return
	}

	// Create and execute sync job
	syncJob := job.NewSyncJob(secret.Mount, secret.KeyPath, o.vaultClient, o.dbClient)

	// Execute with context (job.Execute should also respect context)
	jobSyncResult, err := syncJob.Execute(ctx)
	if err != nil {
		o.logger.Error().
			Err(err).
			Str("mount", secret.Mount).
			Str("path", secret.KeyPath).
			Msg("Sync job execution failed")

		// Create a failed result
		jobSyncResult = job.NewSyncJobResult(syncJob, []*job.ClusterSyncStatus{}, err)
	}
	jobResults <- jobSyncResult
}

// collectResults aggregates job results and updates counters.
func (o *SyncOrchestrator) collectResults(result *SyncResult, jobResults chan *job.SyncJobResult) {
	for jobResult := range jobResults {
		result.JobResults = append(result.JobResults, jobResult)
		o.categorizeJobResult(jobResult, result)
	}
}

func (o *SyncOrchestrator) categorizeJobResult(jobResult *job.SyncJobResult, result *SyncResult) {
	// Check if job execution failed
	if jobResult.Error != nil {
		// Check if the error is due to context cancellation
		if errors.Is(jobResult.Error, context.Canceled) ||
			errors.Is(jobResult.Error, context.DeadlineExceeded) {
			result.SkippedSecrets++
			o.logger.Debug().
				Str("mount", jobResult.Mount).
				Str("path", jobResult.KeyPath).
				Msg("Job skipped due to context cancellation")
			return
		}
	}

	hasFailure := false
	allNoOp := true

	for _, clusterStatus := range jobResult.Status {
		if o.isFailureStatus(clusterStatus.Status) {
			o.logJobError(clusterStatus.ClusterName, jobResult)
			hasFailure = true
			allNoOp = false
		} else if o.isSuccessStatus(clusterStatus.Status) {
			allNoOp = false
		}
	}

	o.updateResultCounters(result, hasFailure, allNoOp, jobResult)
}

// isFailureStatus checks if a status indicates failure.
func (o *SyncOrchestrator) isFailureStatus(status job.SyncJobStatus) bool {
	return status == job.SyncJobStatusFailed || status == job.SyncJobStatusErrorDeleting
}

// isSuccessStatus checks if a status indicates a successful change.
func (o *SyncOrchestrator) isSuccessStatus(status job.SyncJobStatus) bool {
	return status == job.SyncJobStatusUpdated || status == job.SyncJobStatusDeleted
}

// updateResultCounters updates the appropriate counter based on job outcome.
func (o *SyncOrchestrator) updateResultCounters(
	result *SyncResult,
	hasFailure bool,
	allNoOp bool,
	jobResult *job.SyncJobResult,
) {
	switch {
	case hasFailure:
		result.FailedSyncs++
		o.logger.Error().
			Str("mount", jobResult.Mount).
			Str("path", jobResult.KeyPath).
			Msg("One or more clusters failed to sync")

	case allNoOp:
		result.NoOpSecrets++
		o.logger.Debug().
			Str("mount", jobResult.Mount).
			Str("path", jobResult.KeyPath).
			Msg("Secret unchanged - no sync needed")

	default:
		result.SuccessfulSyncs++
		o.logger.Debug().
			Str("mount", jobResult.Mount).
			Str("path", jobResult.KeyPath).
			Msg("Successfully synced to all clusters")
	}
}

func (o *SyncOrchestrator) logJobError(clusterName string, jobResult *job.SyncJobResult) {
	o.logger.Error().
		Err(jobResult.Error).
		Str("mount", jobResult.Mount).
		Str("path", jobResult.KeyPath).
		Str("cluster", clusterName).
		Msg("Job failed")
}

func (o *SyncOrchestrator) logSummary(result *SyncResult) {
	o.logger.Info().
		Int("total", result.TotalSecrets).
		Int("successful", result.SuccessfulSyncs).
		Int("failed", result.FailedSyncs).
		Int("skipped", result.SkippedSecrets).
		Int("no_op", result.NoOpSecrets).
		Dur("duration", result.Duration).
		Msg("Synchronization completed")
}
