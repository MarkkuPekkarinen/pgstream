// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"errors"
	"time"

	"github.com/xataio/pgstream/pkg/kafka"
	kafkacheckpoint "github.com/xataio/pgstream/pkg/wal/checkpointer/kafka"
	snapshotbuilder "github.com/xataio/pgstream/pkg/wal/listener/snapshot/builder"
	"github.com/xataio/pgstream/pkg/wal/processor/injector"
	kafkaprocessor "github.com/xataio/pgstream/pkg/wal/processor/kafka"
	"github.com/xataio/pgstream/pkg/wal/processor/postgres"
	"github.com/xataio/pgstream/pkg/wal/processor/search"
	"github.com/xataio/pgstream/pkg/wal/processor/search/store"
	"github.com/xataio/pgstream/pkg/wal/processor/transformer"
	"github.com/xataio/pgstream/pkg/wal/processor/webhook/notifier"
	"github.com/xataio/pgstream/pkg/wal/processor/webhook/subscription/server"
	pgreplication "github.com/xataio/pgstream/pkg/wal/replication/postgres"
)

type Config struct {
	Listener  ListenerConfig
	Processor ProcessorConfig
}

type ListenerConfig struct {
	Postgres *PostgresListenerConfig
	Kafka    *KafkaListenerConfig
	Snapshot *snapshotbuilder.SnapshotListenerConfig
}

type PostgresListenerConfig struct {
	Replication pgreplication.Config
	Snapshot    *snapshotbuilder.SnapshotListenerConfig
}

type KafkaListenerConfig struct {
	Reader       kafka.ReaderConfig
	Checkpointer kafkacheckpoint.Config
}

type ProcessorConfig struct {
	Kafka       *KafkaProcessorConfig
	Search      *SearchProcessorConfig
	Webhook     *WebhookProcessorConfig
	Postgres    *PostgresProcessorConfig
	Injector    *injector.Config
	Transformer *transformer.Config
}

type KafkaProcessorConfig struct {
	Writer *kafkaprocessor.Config
}

type SearchProcessorConfig struct {
	Indexer search.IndexerConfig
	Store   store.Config
	Retrier search.StoreRetryConfig
}

type WebhookProcessorConfig struct {
	Notifier           notifier.Config
	SubscriptionServer server.Config
	SubscriptionStore  WebhookSubscriptionStoreConfig
}

type PostgresProcessorConfig struct {
	BatchWriter postgres.Config
}

type WebhookSubscriptionStoreConfig struct {
	URL                  string
	CacheEnabled         bool
	CacheRefreshInterval time.Duration
}

func (c *Config) IsValid() error {
	if c.Listener.Kafka == nil && c.Listener.Postgres == nil && c.Listener.Snapshot == nil {
		return errors.New("need at least one listener configured")
	}

	if c.Processor.Kafka == nil && c.Processor.Search == nil && c.Processor.Webhook == nil && c.Processor.Postgres == nil {
		return errors.New("need at least one processor configured")
	}

	return nil
}
