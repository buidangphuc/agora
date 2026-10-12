package ga4import

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

// BigQuerySource aggregates the GA4 export table server-side in BigQuery.
type BigQuerySource struct {
	client  *bigquery.Client
	dataset string // "dataset" or "project.dataset"
}

// NewBigQuerySource opens a client for project; dataset holds events_YYYYMMDD.
func NewBigQuerySource(ctx context.Context, project, dataset string) (*BigQuerySource, error) {
	if i := strings.Index(dataset, "."); i > 0 && project == "" {
		project = dataset[:i]
	}
	c, err := bigquery.NewClient(ctx, project)
	if err != nil {
		return nil, err
	}
	return &BigQuerySource{client: c, dataset: dataset}, nil
}

// Close releases the client.
func (b *BigQuerySource) Close() error { return b.client.Close() }

func bqParam(name, field string) string {
	return fmt.Sprintf("(SELECT value.%s FROM UNNEST(event_params) WHERE key = '%s')", field, name)
}

// Extract reads events_<day> (failing when the table is missing).
func (b *BigQuerySource) Extract(ctx context.Context, day time.Time) (*Extract, error) {
	table := "events_" + day.Format("20060102")
	ds := b.dataset
	if i := strings.Index(ds, "."); i > 0 {
		ds = ds[i+1:]
	}
	if _, err := b.client.Dataset(ds).Table(table).Metadata(ctx); err != nil {
		return nil, fmt.Errorf("export table %s not found: %w", table, err)
	}
	ref := "`" + b.dataset + "." + table + "`"
	if !strings.Contains(b.dataset, ".") {
		ref = "`" + b.client.Project() + "." + b.dataset + "." + table + "`"
	}
	ex := &Extract{}

	sessSQL := `WITH e AS (
	  SELECT user_pseudo_id, TIMESTAMP_MICROS(event_timestamp) AS ts,
	    CAST(` + bqParam("ga_session_id", "int_value") + ` AS STRING) AS sid,
	    COALESCE(NULLIF(` + bqParam("source", "string_value") + `, ''), NULLIF(traffic_source.source, ''), '(direct)') AS source,
	    COALESCE(NULLIF(` + bqParam("medium", "string_value") + `, ''), NULLIF(traffic_source.medium, ''), '(none)') AS medium,
	    COALESCE(NULLIF(` + bqParam("campaign", "string_value") + `, ''), NULLIF(traffic_source.name, ''), '(direct)') AS campaign,
	    ` + bqParam("page_location", "string_value") + ` AS page
	  FROM ` + ref + `)
	SELECT user_pseudo_id, sid, ARRAY_AGG(source ORDER BY ts LIMIT 1)[OFFSET(0)] AS source,
	  ARRAY_AGG(medium ORDER BY ts LIMIT 1)[OFFSET(0)] AS medium,
	  ARRAY_AGG(campaign ORDER BY ts LIMIT 1)[OFFSET(0)] AS campaign,
	  COALESCE(REGEXP_REPLACE(ARRAY_AGG(page IGNORE NULLS ORDER BY ts LIMIT 1)[SAFE_OFFSET(0)], r'[?#].*$', ''), '') AS landing_page,
	  MIN(ts) AS start_ts, MAX(ts) AS end_ts
	FROM e WHERE sid IS NOT NULL AND user_pseudo_id IS NOT NULL GROUP BY user_pseudo_id, sid`
	it, err := b.client.Query(sessSQL).Read(ctx)
	if err != nil {
		return nil, err
	}
	for {
		var r struct {
			UserPseudoID string    `bigquery:"user_pseudo_id"`
			Sid          string    `bigquery:"sid"`
			Source       string    `bigquery:"source"`
			Medium       string    `bigquery:"medium"`
			Campaign     string    `bigquery:"campaign"`
			LandingPage  string    `bigquery:"landing_page"`
			Start        time.Time `bigquery:"start_ts"`
			End          time.Time `bigquery:"end_ts"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		ex.Sessions = append(ex.Sessions, Session{r.UserPseudoID, r.Sid, r.Source, r.Medium, r.Campaign, r.LandingPage, r.Start, r.End})
	}

	dit, err := b.client.Query("SELECT event_name, COUNT(*) AS events FROM " + ref + " GROUP BY event_name").Read(ctx)
	if err != nil {
		return nil, err
	}
	for {
		var r struct {
			EventName string `bigquery:"event_name"`
			Events    int64  `bigquery:"events"`
		}
		err := dit.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		ex.Daily = append(ex.Daily, DailyCount{r.EventName, r.Events})
		ex.Events += r.Events
	}
	return ex, nil
}
