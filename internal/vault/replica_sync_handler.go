package vault

import (
	"context"
	"fmt"
	"slices"
	"time"
	"vault-sync/internal/models"

	"github.com/rs/zerolog"
)

type operationType string

const (
	operationTypeSync   operationType = "sync"
	operationTypeDelete operationType = "delete"
)

type replicaSyncHandler[T replicaSyncOperationResult] struct {
	operationType operationType
	ctx           context.Context
	logger        *zerolog.Logger
	sourceVersion int64
	clusters      []string
	mount         string
	keyPath       string
	resultsChan   chan T
	operationFunc syncOperationFunc[T]
}

func (o *replicaSyncHandler[T]) executeSync() ([]T, error) {
	if len(o.clusters) == 0 {
		o.logger.Warn().Msg("No replica clusters configured, skipping synchronization")
		return []T{}, nil
	}

	collector := newSyncResultAggregator[T](len(o.clusters))
	o.resultsChan = collector.resultsChan

	for _, clusterName := range o.clusters {
		go o.syncSingleCluster(clusterName)
	}

	results, err := collector.aggregate(o.ctx)
	if err != nil {
		o.logger.Error().Err(err).Msg("Failed to collect sync results")
		return nil, err
	}

	sortSyncedSecretsByDestination(results)
	logOperationSummary(o.logger, results)
	return results, nil
}

func (o *replicaSyncHandler[T]) syncSingleCluster(destinationCluster string) {
	var zero T
	var result any

	switch any(zero).(type) {
	case *models.SyncedSecret:
		if o.operationType != operationTypeSync {
			o.logger.Error().Msgf("Operation type mismatch: expected %s, got %s", operationTypeSync, o.operationType)
			return
		}
		result = &models.SyncedSecret{
			SecretBackend:      o.mount,
			SecretPath:         o.keyPath,
			SourceVersion:      o.sourceVersion,
			DestinationCluster: destinationCluster,
			LastSyncAttempt:    time.Now(),
			Status:             models.StatusPending,
		}
	case *models.SyncSecretDeletionResult:
		if o.operationType != operationTypeDelete {
			o.logger.Error().Msgf("Operation type mismatch: expected %s, got %s", operationTypeDelete, o.operationType)
			return
		}
		result = &models.SyncSecretDeletionResult{
			SecretBackend:      o.mount,
			SecretPath:         o.keyPath,
			DestinationCluster: destinationCluster,
			DeletionAttempt:    time.Now(),
			Status:             models.StatusPending,
		}
	}

	o.execute(destinationCluster, result.(T))
}

func (o *replicaSyncHandler[T]) execute(destinationCluster string, syncResult T) {
	defer o.processResults(o.ctx, syncResult)
	err := o.operationFunc(o.ctx, o.mount, o.keyPath, destinationCluster, syncResult)
	errorMsg, hasError := o.checkSyncError(destinationCluster, err)
	if hasError {
		syncResult.SetErrorMessage(&errorMsg)
		syncResult.SetStatus(models.StatusFailed)
		o.logger.Error().Err(err).Msg(fmt.Sprintf("Failed to %s secret to replica cluster: %s", o.operationType, destinationCluster))
		return
	}

	now := time.Now()
	syncResult.SetLastSuccessAttempt(&now)
}

func (o *replicaSyncHandler[T]) processResults(ctx context.Context, syncResult T) {
	select {
	case o.resultsChan <- syncResult:
	case <-ctx.Done():
	}
}

func (o *replicaSyncHandler[T]) checkSyncError(destinationCluster string, err error) (string, bool) {
	if err != nil && o.operationType == operationTypeDelete && isNotFoundError(err) {
		return "Secret not found - treated as successful deletion", false
	}

	if err != nil {
		o.logger.Error().Err(err).Msg(fmt.Sprintf("Failed to %s secret to replica cluster", destinationCluster))
		errorMsg := fmt.Sprintf("failed to %s secret to cluster %s: %v", o.operationType, destinationCluster, err)
		return errorMsg, true
	}

	return "", false
}

func sortSyncedSecretsByDestination[T replicaSyncOperationResult](results []T) {
	slices.SortStableFunc(results, func(a, b T) int {
		if a.GetDestinationCluster() < b.GetDestinationCluster() {
			return -1
		} else if a.GetDestinationCluster() > b.GetDestinationCluster() {
			return 1
		}
		return 0
	})
}
