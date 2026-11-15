package warehouse

import "context"

// ErasedUsersTableName is the DuckDB-only tombstone table.
const ErasedUsersTableName = "erased_users"

// PseudonymPrefix prefixes the 32 hex characters of an erasure pseudonym.
const PseudonymPrefix = "erased:"

// ErasedUsersSchema is erased_users(user_hmac PRIMARY KEY, pseudonym, erased_at).
var ErasedUsersSchema = []Column{
	{Name: "user_hmac", DuckDBType: "VARCHAR", BigQueryType: "STRING"},
	{Name: "pseudonym", DuckDBType: "VARCHAR", BigQueryType: "STRING"},
	{Name: "erased_at", DuckDBType: "TIMESTAMP", BigQueryType: "TIMESTAMP"},
}

// EraseCounts maps a table name (tracking_events, order_facts, engagement_facts,
// listing_sellers) to the number of rows the purge re-keyed or deleted there.
type EraseCounts map[string]int64

// UserEraser purges one user in a single transaction; idempotent.
type UserEraser interface {
	EraseUser(ctx context.Context, userID string) (EraseCounts, error)
}
