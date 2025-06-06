// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/stretchr/testify/require"
	"github.com/xataio/pgstream/pkg/schemalog"
	schemalogmocks "github.com/xataio/pgstream/pkg/schemalog/mocks"
)

func TestDDLAdapter_walDataToQueries(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	id := xid.New()

	table1 := "table1"
	table2 := "table2"

	testLogEntry := func(version int64) *schemalog.LogEntry {
		return &schemalog.LogEntry{
			ID:         id,
			Version:    version,
			SchemaName: testSchema,
			CreatedAt:  schemalog.NewSchemaCreatedAtTimestamp(now),
		}
	}

	tests := []struct {
		name         string
		querier      schemalogQuerier
		schemaDiffer schemaDiffer
		logEntry     *schemalog.LogEntry

		wantQueries []*query
		wantErr     error
	}{
		{
			name: "ok - initial schema",
			querier: &schemalogmocks.Store{
				FetchFn: func(ctx context.Context, schemaName string, version int) (*schemalog.LogEntry, error) {
					return nil, errors.New("unexpected call to FetchFn")
				},
			},
			schemaDiffer: func(old, new *schemalog.LogEntry) *schemalog.Diff {
				return &schemalog.Diff{
					TablesChanged: []schemalog.TableDiff{
						{
							TableName: table2,
							TableNameChange: &schemalog.ValueChange[string]{
								Old: table1,
								New: table2,
							},
						},
					},
				}
			},
			logEntry: testLogEntry(0),

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table2,
					sql:    fmt.Sprintf("ALTER TABLE %s RENAME TO %s", quotedTableName(testSchema, table1), table2),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - fetching existing schema",
			querier: &schemalogmocks.Store{
				FetchFn: func(ctx context.Context, schemaName string, version int) (*schemalog.LogEntry, error) {
					require.Equal(t, testSchema, schemaName)
					require.Equal(t, int(0), version)
					return testLogEntry(0), nil
				},
			},
			schemaDiffer: func(old, new *schemalog.LogEntry) *schemalog.Diff {
				return &schemalog.Diff{
					TablesChanged: []schemalog.TableDiff{
						{
							TableName: table2,
							TableNameChange: &schemalog.ValueChange[string]{
								Old: table1,
								New: table2,
							},
						},
					},
				}
			},
			logEntry: testLogEntry(1),

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table2,
					sql:    fmt.Sprintf("ALTER TABLE %s RENAME TO %s", quotedTableName(testSchema, table1), table2),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "error - fetching schema log",
			querier: &schemalogmocks.Store{
				FetchFn: func(ctx context.Context, schemaName string, version int) (*schemalog.LogEntry, error) {
					return nil, errTest
				},
			},
			logEntry: testLogEntry(1),

			wantQueries: nil,
			wantErr:     errTest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := ddlAdapter{
				schemalogQuerier: tc.querier,
				schemaDiffer:     tc.schemaDiffer,
			}

			queries, err := a.schemaLogToQueries(context.Background(), tc.logEntry)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantQueries, queries)
		})
	}
}

func TestDDLAdapter_schemaDiffToQueries(t *testing.T) {
	t.Parallel()

	table1 := "test-table-1"
	table2 := "test-table-2"
	defaultAge := "0"

	tests := []struct {
		name string
		diff *schemalog.Diff

		wantQueries []*query
		wantErr     error
	}{
		{
			name: "ok - empty schema diff",
			diff: &schemalog.Diff{},

			wantQueries: []*query{},
			wantErr:     nil,
		},
		{
			name: "ok - table removed",
			diff: &schemalog.Diff{
				TablesRemoved: []schemalog.Table{
					{Name: table1},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("DROP TABLE IF EXISTS %s", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table added",
			diff: &schemalog.Diff{
				TablesAdded: []schemalog.Table{
					{
						Name:              table1,
						PrimaryKeyColumns: []string{"id"},
						Columns: []schemalog.Column{
							{Name: "id", DataType: "uuid", Nullable: false, Unique: true},
							{Name: "name", DataType: "text", Nullable: true, Unique: true},
							{Name: "age", DataType: "int", Nullable: false, DefaultValue: &defaultAge},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n\"id\" uuid NOT NULL,\n\"name\" text,\n\"age\" int NOT NULL DEFAULT 0,\nUNIQUE (\"name\"),\nPRIMARY KEY (\"id\")\n)", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table added no primary key or unique constraint",
			diff: &schemalog.Diff{
				TablesAdded: []schemalog.Table{
					{
						Name: table1,
						Columns: []schemalog.Column{
							{Name: "id", DataType: "uuid", Nullable: false, Unique: false},
							{Name: "name", DataType: "text", Nullable: true, Unique: false},
							{Name: "age", DataType: "int", Nullable: false, DefaultValue: &defaultAge},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n\"id\" uuid NOT NULL,\n\"name\" text,\n\"age\" int NOT NULL DEFAULT 0)", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table renamed",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table2,
						TableNameChange: &schemalog.ValueChange[string]{
							Old: table1,
							New: table2,
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table2,
					sql:    fmt.Sprintf("ALTER TABLE %s RENAME TO %s", quotedTableName(testSchema, table1), table2),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table changed, column dropped",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table1,
						ColumnsRemoved: []schemalog.Column{
							{Name: "age"},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("ALTER TABLE %s DROP COLUMN \"age\"", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table changed, column added",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table1,
						ColumnsAdded: []schemalog.Column{
							{Name: "age", DataType: "int", Nullable: false, DefaultValue: &defaultAge},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("ALTER TABLE %s ADD COLUMN \"age\" int NOT NULL DEFAULT 0", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table changed, column renamed",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table1,
						ColumnsChanged: []schemalog.ColumnDiff{
							{
								ColumnName: "name",
								NameChange: &schemalog.ValueChange[string]{Old: "name", New: "new_name"},
							},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("ALTER TABLE %s RENAME COLUMN \"name\" TO \"new_name\"", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table changed, column type changed",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table1,
						ColumnsChanged: []schemalog.ColumnDiff{
							{
								ColumnName: "name",
								TypeChange: &schemalog.ValueChange[string]{Old: "uuid", New: "int"},
							},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("ALTER TABLE %s ALTER COLUMN \"name\" TYPE int", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table changed, column changed to nullable",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table1,
						ColumnsChanged: []schemalog.ColumnDiff{
							{
								ColumnName: "name",
								NullChange: &schemalog.ValueChange[bool]{Old: false, New: true},
							},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("ALTER TABLE %s ALTER COLUMN \"name\" DROP NOT NULL", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table changed, column changed to not nullable",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table1,
						ColumnsChanged: []schemalog.ColumnDiff{
							{
								ColumnName: "name",
								NullChange: &schemalog.ValueChange[bool]{Old: true, New: false},
							},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("ALTER TABLE %s ALTER COLUMN \"name\" SET NOT NULL", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table changed, column default removed",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table1,
						ColumnsChanged: []schemalog.ColumnDiff{
							{
								ColumnName:    "age",
								DefaultChange: &schemalog.ValueChange[*string]{Old: &defaultAge, New: nil},
							},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("ALTER TABLE %s ALTER COLUMN \"age\" DROP DEFAULT", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
		{
			name: "ok - table changed, column default changed",
			diff: &schemalog.Diff{
				TablesChanged: []schemalog.TableDiff{
					{
						TableName: table1,
						ColumnsChanged: []schemalog.ColumnDiff{
							{
								ColumnName:    "age",
								DefaultChange: &schemalog.ValueChange[*string]{Old: nil, New: &defaultAge},
							},
						},
					},
				},
			},

			wantQueries: []*query{
				{
					schema: testSchema,
					table:  table1,
					sql:    fmt.Sprintf("ALTER TABLE %s ALTER COLUMN \"age\" SET DEFAULT 0", quotedTableName(testSchema, table1)),
					isDDL:  true,
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ddlAdapter := newDDLAdapter(nil)

			queries, err := ddlAdapter.schemaDiffToQueries(testSchema, tc.diff)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantQueries, queries)
		})
	}
}
