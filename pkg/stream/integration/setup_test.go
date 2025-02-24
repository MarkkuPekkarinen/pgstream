// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/xataio/pgstream/internal/testcontainers"
	"github.com/xataio/pgstream/pkg/stream"
)

func TestMain(m *testing.M) {
	// if integration tests are not enabled, nothing to setup
	if os.Getenv("PGSTREAM_INTEGRATION_TESTS") != "" {
		ctx := context.Background()
		pgcleanup, err := testcontainers.SetupPostgresContainer(ctx, &pgurl, testcontainers.Postgres14, "config/postgresql.conf")
		if err != nil {
			log.Fatal(err)
		}
		defer pgcleanup()

		if err := stream.Init(ctx, pgurl, ""); err != nil {
			log.Fatal(err)
		}

		kafkacleanup, err := testcontainers.SetupKafkaContainer(ctx, &kafkaBrokers)
		if err != nil {
			log.Fatal(err)
		}
		defer kafkacleanup()

		oscleanup, err := testcontainers.SetupOpenSearchContainer(ctx, &opensearchURL)
		if err != nil {
			log.Fatal(err)
		}
		defer oscleanup()

		escleanup, err := testcontainers.SetupElasticsearchContainer(ctx, &elasticsearchURL)
		if err != nil {
			log.Fatal(err)
		}
		defer escleanup()

		targetPGCleanup, err := testcontainers.SetupPostgresContainer(ctx, &targetPGURL, testcontainers.Postgres17)
		if err != nil {
			log.Fatal(err)
		}
		defer targetPGCleanup()
	}

	os.Exit(m.Run())
}
