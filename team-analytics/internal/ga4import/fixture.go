package ga4import

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

// FixtureSource reads events_YYYYMMDD.parquet (GA4 export schema) from a directory.
type FixtureSource struct{ dir string }

// NewFixtureSource returns a Source over Parquet files in dir.
func NewFixtureSource(dir string) *FixtureSource { return &FixtureSource{dir: dir} }

func param(name, field string) string {
	return fmt.Sprintf("list_filter(event_params, x -> x.key = '%s')[1].value.%s", name, field)
}

// Extract aggregates the day's Parquet export with an in-memory DuckDB.
func (f *FixtureSource) Extract(ctx context.Context, day time.Time) (*Extract, error) {
	path := filepath.Join(f.dir, "events_"+day.Format("20060102")+".parquet")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("export table events_%s not found: %w", day.Format("20060102"), err)
	}
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	src := "read_parquet('" + strings.ReplaceAll(path, "'", "''") + "')"

	ex := &Extract{}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+src).Scan(&ex.Events); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `WITH e AS (
	  SELECT user_pseudo_id, make_timestamp(event_timestamp) AS ts,
	    CAST(`+param("ga_session_id", "int_value")+` AS VARCHAR) AS sid,
	    COALESCE(NULLIF(`+param("source", "string_value")+`, ''), NULLIF(traffic_source.source, ''), '(direct)') AS source,
	    COALESCE(NULLIF(`+param("medium", "string_value")+`, ''), NULLIF(traffic_source.medium, ''), '(none)') AS medium,
	    COALESCE(NULLIF(`+param("campaign", "string_value")+`, ''), NULLIF(traffic_source.name, ''), '(direct)') AS campaign,
	    `+param("page_location", "string_value")+` AS page
	  FROM `+src+`)
	SELECT user_pseudo_id, sid, arg_min(source, ts), arg_min(medium, ts), arg_min(campaign, ts),
	  COALESCE(regexp_replace(arg_min(page, ts) FILTER (WHERE page IS NOT NULL), '[?#].*$', ''), ''),
	  min(ts), max(ts)
	FROM e WHERE sid IS NOT NULL AND user_pseudo_id IS NOT NULL
	GROUP BY user_pseudo_id, sid ORDER BY user_pseudo_id, sid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.UserPseudoID, &s.SessionID, &s.Source, &s.Medium, &s.Campaign, &s.LandingPage, &s.Start, &s.End); err != nil {
			return nil, err
		}
		ex.Sessions = append(ex.Sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	drows, err := db.QueryContext(ctx, "SELECT event_name, count(*) FROM "+src+" GROUP BY event_name")
	if err != nil {
		return nil, err
	}
	defer drows.Close()
	for drows.Next() {
		var d DailyCount
		if err := drows.Scan(&d.EventName, &d.Events); err != nil {
			return nil, err
		}
		ex.Daily = append(ex.Daily, d)
	}
	return ex, drows.Err()
}
