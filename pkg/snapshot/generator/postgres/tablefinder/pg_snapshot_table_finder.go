// SPDX-License-Identifier: Apache-2.0

package tablefinder

import (
	"context"
	"fmt"
	"slices"

	pglib "github.com/xataio/pgstream/internal/postgres"
	"github.com/xataio/pgstream/pkg/snapshot"
	"github.com/xataio/pgstream/pkg/snapshot/generator"
)

// SnapshotTableFinder is a decorator around a snapshot generator that will
// explode the wildcard references in the snapshot requests and replace them by
// all the schema tables in postgres.
type SnapshotTableFinder struct {
	wrapped generator.SnapshotGenerator
	conn    pglib.Querier
}

const wildcard = "*"

// NewSnapshotTableFinder will return the generator on input wrapped with a
// table finder that will explode the wildcard references in the snapshot
// request and translate them into all the postgres tables for the given schema.
func NewSnapshotTableFinder(ctx context.Context, pgurl string, generator generator.SnapshotGenerator) (*SnapshotTableFinder, error) {
	conn, err := pglib.NewConnPool(ctx, pgurl)
	if err != nil {
		return nil, err
	}
	return &SnapshotTableFinder{
		wrapped: generator,
		conn:    conn,
	}, nil
}

func (s *SnapshotTableFinder) CreateSnapshot(ctx context.Context, ss *snapshot.Snapshot) error {
	if slices.Contains(ss.TableNames, wildcard) {
		var err error
		ss.TableNames, err = s.discoverAllSchemaTables(ctx, ss.SchemaName)
		if err != nil {
			return err
		}
	}
	return s.wrapped.CreateSnapshot(ctx, ss)
}

func (s *SnapshotTableFinder) Close() error {
	return s.conn.Close(context.Background())
}

func (s *SnapshotTableFinder) discoverAllSchemaTables(ctx context.Context, schema string) ([]string, error) {
	const query = "SELECT tablename FROM pg_tables WHERE schemaname=$1"
	rows, err := s.conn.Query(ctx, query, schema)
	if err != nil {
		return nil, fmt.Errorf("discovering all tables for schema %s: %w", schema, err)
	}
	defer rows.Close()

	tableNames := []string{}
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("scanning table name: %w", err)
		}
		tableNames = append(tableNames, tableName)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tableNames, nil
}
