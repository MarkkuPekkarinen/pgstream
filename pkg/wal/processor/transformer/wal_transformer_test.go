// SPDX-License-Identifier: Apache-2.0

package transformer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xataio/pgstream/pkg/log"
	"github.com/xataio/pgstream/pkg/transformers"
	transformermocks "github.com/xataio/pgstream/pkg/transformers/mocks"
	"github.com/xataio/pgstream/pkg/wal"
	"github.com/xataio/pgstream/pkg/wal/processor"
	"github.com/xataio/pgstream/pkg/wal/processor/mocks"
)

func TestTransformer_New(t *testing.T) {
	t.Parallel()

	mockProcessor := &mocks.Processor{}
	testTransformer, err := transformers.NewStringTransformer(transformers.Random, nil)
	require.NoError(t, err)

	tests := []struct {
		name   string
		config *Config

		wantTransformer *Transformer
		wantErr         error
	}{
		{
			name: "ok",
			config: &Config{
				TransformerRulesFile: "test/test_transformer_rules.yaml",
			},

			wantTransformer: &Transformer{
				logger:    log.NewNoopLogger(),
				processor: mockProcessor,
				transformerMap: map[string]columnTransformers{
					"public/test1": {
						"column_1": testTransformer,
					},
					"test/test2": {
						"column_2": testTransformer,
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			transformer, err := New(tc.config, mockProcessor)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantTransformer, transformer)
		})
	}
}

func TestTransformer_ProcessWALEvent(t *testing.T) {
	t.Parallel()

	testSchema := "test_schema"
	testTable := "test_table"
	errTest := errors.New("oh noes")
	testKey := testSchema + "/" + testTable

	newTestEvent := func(cols []wal.Column) *wal.Event {
		return &wal.Event{
			CommitPosition: "1",
			Data: &wal.Data{
				Action:  "I",
				Schema:  testSchema,
				Table:   testTable,
				Columns: cols,
			},
		}
	}

	tests := []struct {
		name           string
		event          *wal.Event
		processor      processor.Processor
		transformerMap map[string]columnTransformers

		wantErr error
	}{
		{
			name:  "ok - no data",
			event: &wal.Event{},
			processor: &mocks.Processor{
				ProcessWALEventFn: func(ctx context.Context, walEvent *wal.Event) error {
					require.Equal(t, &wal.Event{}, walEvent)
					return nil
				},
			},
			transformerMap: map[string]columnTransformers{},

			wantErr: nil,
		},
		{
			name:  "ok - no transformers for schema table",
			event: newTestEvent(nil),
			processor: &mocks.Processor{
				ProcessWALEventFn: func(ctx context.Context, walEvent *wal.Event) error {
					require.Equal(t, newTestEvent(nil), walEvent)
					return nil
				},
			},
			transformerMap: map[string]columnTransformers{
				"anotherschema/table": {},
			},

			wantErr: nil,
		},
		{
			name: "ok - with transformers for schema table",
			event: newTestEvent([]wal.Column{
				{Name: "column_1", Type: "text", Value: "one"},
				{Name: "column_2", Type: "int", Value: 1},
			}),
			processor: &mocks.Processor{
				ProcessWALEventFn: func(ctx context.Context, walEvent *wal.Event) error {
					wantEvent := newTestEvent([]wal.Column{
						{Name: "column_1", Type: "text", Value: "two"},
						{Name: "column_2", Type: "int", Value: 1},
					})
					require.Equal(t, wantEvent, walEvent)
					return nil
				},
			},
			transformerMap: map[string]columnTransformers{
				testKey: {
					"column_1": &transformermocks.Transformer{
						TransformFn: func(a any) (any, error) {
							aStr, ok := a.(string)
							require.True(t, ok)
							require.Equal(t, "one", aStr)
							return "two", nil
						},
					},
				},
			},

			wantErr: nil,
		},
		{
			name: "error - transforming",
			event: newTestEvent([]wal.Column{
				{Name: "column_1", Type: "text", Value: "one"},
				{Name: "column_2", Type: "int", Value: 1},
			}),
			processor: &mocks.Processor{
				ProcessWALEventFn: func(ctx context.Context, walEvent *wal.Event) error {
					wantEvent := newTestEvent([]wal.Column{
						{Name: "column_1", Type: "text", Value: "two"},
						{Name: "column_2", Type: "int", Value: 1},
					})
					require.Equal(t, wantEvent, walEvent)
					return nil
				},
			},
			transformerMap: map[string]columnTransformers{
				testKey: {
					"column_1": &transformermocks.Transformer{
						TransformFn: func(a any) (any, error) {
							return nil, errTest
						},
					},
				},
			},

			wantErr: errTest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			transformer := &Transformer{
				logger:         log.NewNoopLogger(),
				transformerMap: tc.transformerMap,
				processor:      tc.processor,
			}

			err := transformer.ProcessWALEvent(context.Background(), tc.event)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func Test_transformerMapFromRules(t *testing.T) {
	t.Parallel()

	testSchema := "test_schema"
	testTable := "test_table"
	testTransformer, err := transformers.NewStringTransformer(transformers.Random, nil)
	require.NoError(t, err)
	testKey := testSchema + "/" + testTable

	tests := []struct {
		name  string
		rules *Rules

		wantTransformerMap map[string]columnTransformers
		wantErr            error
	}{
		{
			name: "ok",
			rules: &Rules{
				Transformers: []TableRules{
					{
						Schema: testSchema,
						Table:  testTable,
						ColumnRules: map[string]TransformerRules{
							"column_1": {
								Name:      "string",
								Generator: "random",
							},
							"column_2": {
								Name:      "string",
								Generator: "random",
							},
						},
					},
				},
			},

			wantTransformerMap: map[string]columnTransformers{
				testKey: {
					"column_1": testTransformer,
					"column_2": testTransformer,
				},
			},
			wantErr: nil,
		},
		{
			name:  "ok - no rules",
			rules: &Rules{},

			wantTransformerMap: map[string]columnTransformers{},
			wantErr:            nil,
		},
		{
			name: "error - invalid transformer rules",
			rules: &Rules{
				Transformers: []TableRules{
					{
						Schema: testSchema,
						Table:  testTable,
						ColumnRules: map[string]TransformerRules{
							"column_1": {
								Name:      "invalid",
								Generator: "random",
							},
						},
					},
				},
			},

			wantTransformerMap: nil,
			wantErr:            transformers.ErrUnsupportedTransformer,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			transformerMap, err := transformerMapFromRules(tc.rules)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantTransformerMap, transformerMap)
		})
	}
}
