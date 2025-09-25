// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lib/pq"
)

type QualifiedName struct {
	schema string
	name   string
}

var (
	errUnexpectedQualifiedName = errors.New("unexpected qualified name format")
	errInvalidURL              = errors.New("invalid URL")
)

func NewQualifiedName(s string) (*QualifiedName, error) {
	qualifiedName := strings.Split(s, ".")
	switch len(qualifiedName) {
	case 1:
		return &QualifiedName{
			name: s,
		}, nil
	case 2:
		return &QualifiedName{
			schema: qualifiedName[0],
			name:   qualifiedName[1],
		}, nil
	default:
		return nil, errUnexpectedQualifiedName
	}
}

func (qn *QualifiedName) String() string {
	if qn.schema == "" {
		return qn.name
	}
	return QuoteQualifiedIdentifier(qn.schema, qn.name)
}

func (qn *QualifiedName) Schema() string {
	return qn.schema
}

func (qn *QualifiedName) Name() string {
	return qn.name
}

func QuoteIdentifier(s string) string {
	if IsQuotedIdentifier(s) {
		return s
	}
	return pq.QuoteIdentifier(s)
}

func QuoteQualifiedIdentifier(schema, table string) string {
	return QuoteIdentifier(schema) + "." + QuoteIdentifier(table)
}

func IsQuotedIdentifier(s string) bool {
	return len(s) > 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`)
}

type (
	PGDumpFn    func(context.Context, PGDumpOptions) ([]byte, error)
	PGDumpAllFn func(context.Context, PGDumpAllOptions) ([]byte, error)
	PGRestoreFn func(context.Context, PGRestoreOptions, []byte) (string, error)
)

func newIdentifier(tableName string) (pgx.Identifier, error) {
	var identifier pgx.Identifier
	qualifiedTableName := strings.Split(tableName, ".")
	switch len(qualifiedTableName) {
	case 1:
		identifier = pgx.Identifier{tableName}
	case 2:
		identifier = pgx.Identifier{qualifiedTableName[0], qualifiedTableName[1]}
	default:
		return nil, fmt.Errorf("invalid table name: %s", tableName)
	}

	// Remove any quotes from the table name. Identifier has a `Sanitize` method
	// that will be called and will add quotes, so if there are existing ones,
	// it will produce an invalid identifier name.
	for i, part := range identifier {
		identifier[i] = removeQuotes(part)
	}

	return identifier, nil
}

func removeQuotes(s string) string {
	return strings.Trim(s, `"`)
}

func extractDatabase(url string) (string, error) {
	pgCfg, err := ParseConfig(url)
	if err != nil {
		return "", err
	}
	return pgCfg.Database, nil
}

func registerTypesToConnMap(ctx context.Context, conn *pgx.Conn) error {
	var hstoreOID uint32
	err := conn.QueryRow(ctx, "SELECT oid FROM pg_type WHERE typname = 'hstore'").Scan(&hstoreOID)
	if err == nil && hstoreOID != 0 {
		conn.TypeMap().RegisterType(&pgtype.Type{
			Codec: pgtype.HstoreCodec{},
			Name:  "hstore",
			OID:   hstoreOID,
		})
	}

	return nil
}

const DiscoverAllSchemasQuery = "SELECT nspname FROM pg_catalog.pg_namespace WHERE nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast', 'pgstream')"

func DiscoverAllSchemas(ctx context.Context, conn Querier) ([]string, error) {
	rows, err := conn.Query(ctx, DiscoverAllSchemasQuery)
	if err != nil {
		return nil, fmt.Errorf("discovering all schemas for wildcard: %w", err)
	}
	defer rows.Close()

	schemas := []string{}
	for rows.Next() {
		var schemaName string
		if err := rows.Scan(&schemaName); err != nil {
			return nil, fmt.Errorf("scanning schema name: %w", err)
		}
		schemas = append(schemas, schemaName)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return schemas, nil
}

const DiscoverAllSchemaTablesQuery = "SELECT tablename FROM pg_tables WHERE schemaname=$1"

func DiscoverAllSchemaTables(ctx context.Context, conn Querier, schema string) ([]string, error) {
	rows, err := conn.Query(ctx, DiscoverAllSchemaTablesQuery, schema)
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

func ParseConfig(pgurl string) (*pgx.ConnConfig, error) {
	pgCfg, err := pgx.ParseConfig(pgurl)
	if err != nil {
		urlErr := &url.Error{}
		if errors.As(err, &urlErr) {
			escapedURL, err := escapeConnectionURL(pgurl)
			if err != nil {
				return nil, fmt.Errorf("failed to escape connection URL: %w", err)
			}
			return pgx.ParseConfig(escapedURL)
		}
		return nil, fmt.Errorf("failed parsing postgres connection string: %w", mapError(err))
	}
	return pgCfg, nil
}

var postgresURLRegex = regexp.MustCompile(`^(postgres(?:ql)?://)([^@]+?)@(.+)$`)

func escapeConnectionURL(rawURL string) (string, error) {
	// Only process PostgreSQL URLs
	if !strings.HasPrefix(rawURL, "postgresql://") && !strings.HasPrefix(rawURL, "postgres://") {
		return rawURL, nil
	}

	matches := postgresURLRegex.FindStringSubmatch(rawURL)
	if matches == nil {
		return "", errInvalidURL
	}

	scheme := matches[1]      // "postgresql://" or "postgres://"
	userInfo := matches[2]    // "username:password"
	hostAndPath := matches[3] // "host:port/database?params"

	// Find the first colon in userInfo to split username and password. This
	// replicates the behaviour of psql
	firstColonIndex := strings.Index(userInfo, ":")
	if firstColonIndex == -1 {
		// No password, return as-is
		return rawURL, nil
	}

	username := userInfo[:firstColonIndex]
	password := userInfo[firstColonIndex+1:]
	if username == "" {
		return "", errInvalidURL
	}
	// URL encode the password
	encodedPassword := url.QueryEscape(password)

	return fmt.Sprintf("%s%s:%s@%s", scheme, username, encodedPassword, hostAndPath), nil
}
