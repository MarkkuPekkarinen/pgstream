// SPDX-License-Identifier: Apache-2.0

package search

import (
	"time"

	"github.com/xataio/pgstream/internal/backoff"
)

type IndexerConfig struct {
	// BatchSize is the max number of wal events accumulated before triggering a
	// send to the search store. Defaults to 100
	BatchSize int
	// BatchTime is the max time interval at which the batch sending to the search
	// store is triggered. Defaults to 1s
	BatchTime time.Duration
	// MaxQueueBytes is the max memory used by the batch indexer for inflight
	// batches. Defaults to 100MiB
	MaxQueueBytes int64
	// CleanupBackoff is the retry policy to follow for the async index
	// deletion. If no config is provided, no retry policy is applied.
	CleanupBackoff backoff.Config
	// DisableImmutableFields will stop using immutable fields for the search
	// documents if enabled. Use of immutable fields guarantees no data
	// reindexing is required when a table column is renamed, since an immutable
	// id is used instead of the field name for the mapping. Defaults to false.
	DisableImmutableFields bool
}

const (
	defaultMaxQueueBytes = int64(100 * 1024 * 1024) // 100MiB
	defaultBatchSize     = 100
	defaultBatchTime     = time.Second
)

func (c *IndexerConfig) batchSize() int {
	if c.BatchSize > 0 {
		return c.BatchSize
	}
	return defaultBatchSize
}

func (c *IndexerConfig) batchTime() time.Duration {
	if c.BatchTime > 0 {
		return c.BatchTime
	}
	return defaultBatchTime
}

func (c *IndexerConfig) maxQueueBytes() int64 {
	if c.MaxQueueBytes > 0 {
		return c.MaxQueueBytes
	}

	return defaultMaxQueueBytes
}
