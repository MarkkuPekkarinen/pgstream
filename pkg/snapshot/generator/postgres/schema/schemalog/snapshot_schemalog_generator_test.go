// SPDX-License-Identifier: Apache-2.0

package schemalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/stretchr/testify/require"
	"github.com/xataio/pgstream/pkg/schemalog"
	schemalogmocks "github.com/xataio/pgstream/pkg/schemalog/mocks"
	"github.com/xataio/pgstream/pkg/snapshot"
	generatormocks "github.com/xataio/pgstream/pkg/snapshot/generator/mocks"
)

func TestSnapshotGenerator_CreateSnapshot(t *testing.T) {
	t.Parallel()

	testSchemaName := "test-schema"
	testTable := "test-table"
	testSnapshot := &snapshot.Snapshot{
		SchemaName: testSchemaName,
		TableNames: []string{testTable},
	}

	now := schemalog.NewSchemaCreatedAtTimestamp(time.Now())
	testXID := xid.New()
	testSchemaLog := &schemalog.LogEntry{
		ID:         testXID,
		Version:    1,
		SchemaName: testSchemaName,
		CreatedAt:  now,
		Schema: schemalog.Schema{
			Tables: []schemalog.Table{
				{
					Name: testTable,
					Columns: []schemalog.Column{
						{Name: "id", DataType: "int4"},
						{Name: "name", DataType: "text"},
					},
				},
			},
		},
		Acked: false,
	}

	testSchemaBytes, err := json.Marshal(testSchemaLog.Schema)
	require.NoError(t, err)
	testRow := &snapshot.Row{
		Schema: schemalog.SchemaName,
		Table:  schemalog.TableName,
		Columns: []snapshot.Column{
			{Name: "id", Type: "pgstream.xid", Value: testXID},
			{Name: "version", Type: "bigint", Value: int64(1)},
			{Name: "schema_name", Type: "text", Value: testSchemaName},
			{Name: "created_at", Type: "timestamp without time zone", Value: now},
			{Name: "schema", Type: "jsonb", Value: string(testSchemaBytes)},
			{Name: "acked", Type: "boolean", Value: false},
		},
	}

	errTest := errors.New("oh noes")

	tests := []struct {
		name        string
		schemaStore *schemalogmocks.Store
		processRow  snapshot.RowProcessor
		marshaler   func(any) ([]byte, error)
		generator   *generatormocks.Generator

		wantGeneratorCalls uint
		wantErr            error
	}{
		{
			name: "ok",
			schemaStore: &schemalogmocks.Store{
				InsertFn: func(ctx context.Context, schemaName string) (*schemalog.LogEntry, error) {
					return testSchemaLog, nil
				},
			},
			processRow: func(ctx context.Context, r *snapshot.Row) error {
				require.Equal(t, testRow, r)
				return nil
			},

			wantErr: nil,
		},
		{
			name: "ok - with generator",
			schemaStore: &schemalogmocks.Store{
				InsertFn: func(ctx context.Context, schemaName string) (*schemalog.LogEntry, error) {
					return testSchemaLog, nil
				},
			},
			processRow: func(ctx context.Context, r *snapshot.Row) error {
				require.Equal(t, testRow, r)
				return nil
			},
			generator: &generatormocks.Generator{
				CreateSnapshotFn: func(ctx context.Context, ss *snapshot.Snapshot) error {
					require.Equal(t, testSnapshot, ss)
					return nil
				},
				CloseFn: func() error { return nil },
			},

			wantGeneratorCalls: 1,
			wantErr:            nil,
		},
		{
			name: "error - inserting schema log",
			schemaStore: &schemalogmocks.Store{
				InsertFn: func(ctx context.Context, schemaName string) (*schemalog.LogEntry, error) {
					return nil, errTest
				},
			},
			processRow: func(ctx context.Context, r *snapshot.Row) error {
				return errors.New("processRow: should not be called")
			},

			wantErr: snapshot.NewErrors(errTest),
		},
		{
			name: "error - processing schema row",
			schemaStore: &schemalogmocks.Store{
				InsertFn: func(ctx context.Context, schemaName string) (*schemalog.LogEntry, error) {
					return testSchemaLog, nil
				},
			},

			processRow: func(ctx context.Context, r *snapshot.Row) error {
				return errTest
			},

			wantErr: snapshot.NewErrors(errTest),
		},
		{
			name: "error - converting log entry to row",
			schemaStore: &schemalogmocks.Store{
				InsertFn: func(ctx context.Context, schemaName string) (*schemalog.LogEntry, error) {
					return testSchemaLog, nil
				},
			},
			processRow: func(ctx context.Context, r *snapshot.Row) error {
				return errors.New("processRow: should not be called")
			},
			marshaler: func(a any) ([]byte, error) { return nil, errTest },

			wantErr: snapshot.NewErrors(fmt.Errorf("marshaling log entry schema into json: %w", errTest)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := []Option{}
			if tc.generator != nil {
				opts = append(opts, WithSnapshotGenerator(tc.generator))
			}

			g := NewSnapshotGenerator(tc.schemaStore, tc.processRow, opts...)
			defer g.Close()

			if tc.marshaler != nil {
				g.marshaler = tc.marshaler
			}

			err := g.CreateSnapshot(context.Background(), testSnapshot)
			require.Equal(t, err, tc.wantErr)

			if tc.generator != nil {
				require.Equal(t, tc.wantGeneratorCalls, tc.generator.CreateSnapshotCalls())
			}
		})
	}
}
