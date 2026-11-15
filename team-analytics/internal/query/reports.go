package query

import (
	"context"
	"fmt"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

const (
	retentionRepeatDays = 30
	maxWeeklyBuckets    = 64
)

func utcMonday(t time.Time) time.Time {
	t = t.UTC()
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

func (r *DuckDBRepository) RetentionCohorts(ctx context.Context, from time.Time, weeks int, now time.Time) ([]RetentionCohortRow, error) {
	start := utcMonday(from)
	end := start.AddDate(0, 0, weeks*7)
	now = now.UTC()
	//nolint:gosec // only fixed identifiers are interpolated
	q := fmt.Sprintf(`
WITH su AS (
  SELECT user_key, min(occurred_at) AS first_at
  FROM %[1]s
  WHERE event_type = 'sign_up' AND user_key NOT LIKE 'anon:%%'
  GROUP BY user_key
),
co AS (
  SELECT user_key, CAST(first_at AS DATE) AS sday, CAST(date_trunc('week', first_at) AS DATE) AS cw
  FROM su
),
co_in AS (
  SELECT * FROM co WHERE cw >= CAST(? AS DATE) AND cw < CAST(? AS DATE)
),
act AS (
  SELECT DISTINCT e.user_key, CAST(e.occurred_at AS DATE) AS d
  FROM %[1]s e JOIN co_in c ON e.user_key = c.user_key
),
dn AS (
  SELECT c.cw,
    count(*) FILTER (WHERE CAST(c.sday + 2 AS TIMESTAMP) <= CAST(? AS TIMESTAMP)) AS d1e,
    count(*) FILTER (WHERE CAST(c.sday + 2 AS TIMESTAMP) <= CAST(? AS TIMESTAMP)
      AND EXISTS (SELECT 1 FROM act a WHERE a.user_key = c.user_key AND a.d = c.sday + 1)) AS d1a,
    count(*) FILTER (WHERE CAST(c.sday + 8 AS TIMESTAMP) <= CAST(? AS TIMESTAMP)) AS d7e,
    count(*) FILTER (WHERE CAST(c.sday + 8 AS TIMESTAMP) <= CAST(? AS TIMESTAMP)
      AND EXISTS (SELECT 1 FROM act a WHERE a.user_key = c.user_key AND a.d = c.sday + 7)) AS d7a,
    count(*) FILTER (WHERE CAST(c.sday + 31 AS TIMESTAMP) <= CAST(? AS TIMESTAMP)) AS d30e,
    count(*) FILTER (WHERE CAST(c.sday + 31 AS TIMESTAMP) <= CAST(? AS TIMESTAMP)
      AND EXISTS (SELECT 1 FROM act a WHERE a.user_key = c.user_key AND a.d = c.sday + 30)) AS d30a,
    count(*) AS size
  FROM co_in c GROUP BY c.cw
),
ord AS (
  SELECT o.buyer_id AS user_key, o.order_id, min(o.occurred_at) AS t
  FROM %[2]s o JOIN co_in c ON o.buyer_id = c.user_key
  WHERE o.status = 'PAID'
  GROUP BY o.buyer_id, o.order_id
),
fst AS (
  SELECT user_key, min(t) AS ft FROM ord GROUP BY user_key
),
rep AS (
  SELECT c.cw,
    count(*) FILTER (WHERE f.ft + INTERVAL %[3]d DAY <= CAST(? AS TIMESTAMP)) AS re,
    count(*) FILTER (WHERE f.ft + INTERVAL %[3]d DAY <= CAST(? AS TIMESTAMP)
      AND EXISTS (SELECT 1 FROM ord x WHERE x.user_key = f.user_key AND x.t > f.ft
                  AND x.t <= f.ft + INTERVAL %[3]d DAY)) AS rb
  FROM fst f JOIN co_in c ON c.user_key = f.user_key GROUP BY c.cw
)
SELECT CAST(dn.cw AS VARCHAR), dn.size, dn.d1a, dn.d1e, dn.d7a, dn.d7e, dn.d30a, dn.d30e,
       COALESCE(rep.rb, 0), COALESCE(rep.re, 0)
FROM dn LEFT JOIN rep USING (cw)
ORDER BY dn.cw`, warehouse.ResolvedViewName, warehouse.OrderFactsTableName, retentionRepeatDays)
	rows, err := r.db.QueryContext(ctx, q, start, end, now, now, now, now, now, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("retention cohorts query: %w", err)
	}
	var out []RetentionCohortRow
	for rows.Next() {
		var c RetentionCohortRow
		if err := rows.Scan(&c.CohortWeek, &c.CohortSize, &c.D1Active, &c.D1Eligible, &c.D7Active, &c.D7Eligible,
			&c.D30Active, &c.D30Eligible, &c.RepeatBuyers, &c.RepeatEligible); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan retention cohort: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate retention cohorts: %w", err)
	}
	_ = rows.Close()
	if len(out) == 0 {
		return out, nil
	}

	//nolint:gosec // only fixed identifiers are interpolated
	wq := fmt.Sprintf(`
WITH co AS (
  SELECT user_key, CAST(date_trunc('week', min(occurred_at)) AS DATE) AS cw
  FROM %[1]s
  WHERE event_type = 'sign_up' AND user_key NOT LIKE 'anon:%%'
  GROUP BY user_key
),
co_in AS (
  SELECT * FROM co WHERE cw >= CAST(? AS DATE) AND cw < CAST(? AS DATE)
)
SELECT CAST(c.cw AS VARCHAR), CAST(date_diff('day', c.cw, CAST(e.occurred_at AS DATE)) // 7 AS BIGINT) AS wk,
       count(DISTINCT e.user_key)
FROM %[1]s e JOIN co_in c ON e.user_key = c.user_key
WHERE CAST(e.occurred_at AS DATE) >= c.cw
GROUP BY 1, 2`, warehouse.ResolvedViewName)
	wrows, err := r.db.QueryContext(ctx, wq, start, end)
	if err != nil {
		return nil, fmt.Errorf("retention weekly query: %w", err)
	}
	defer wrows.Close()
	active := map[string]map[int64]int64{}
	for wrows.Next() {
		var cw string
		var wk, n int64
		if err := wrows.Scan(&cw, &wk, &n); err != nil {
			return nil, fmt.Errorf("scan retention weekly: %w", err)
		}
		if active[cw] == nil {
			active[cw] = map[int64]int64{}
		}
		active[cw][wk] = n
	}
	if err := wrows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retention weekly: %w", err)
	}
	for i := range out {
		cw, err := time.Parse("2006-01-02", out[i].CohortWeek)
		if err != nil {
			return nil, fmt.Errorf("parse cohort week: %w", err)
		}
		for k := int64(0); k < maxWeeklyBuckets; k++ {
			if cw.AddDate(0, 0, int(k+1)*7).After(now) {
				break
			}
			out[i].WeeklyActive = append(out[i].WeeklyActive, active[out[i].CohortWeek][k])
		}
	}
	return out, nil
}

func (r *DuckDBRepository) ExperimentReadout(ctx context.Context, flag string, from, to time.Time, windowDays int) ([]ExperimentRow, error) {
	//nolint:gosec // only fixed identifiers are interpolated
	q := fmt.Sprintf(`
WITH first_exp AS (
  SELECT user_key, json_extract_string(properties, '$.variant') AS variant, occurred_at AS fe
  FROM %[1]s
  WHERE event_type = 'experiment_exposure'
    AND json_extract_string(properties, '$.flag') = ?
    AND occurred_at >= ? AND occurred_at <= ?
  QUALIFY row_number() OVER (PARTITION BY user_key ORDER BY occurred_at, event_id) = 1
)
SELECT f.variant, count(*) AS exposed,
       count(*) FILTER (WHERE EXISTS (
         SELECT 1 FROM %[2]s o
         WHERE o.status = 'PAID' AND o.buyer_id = f.user_key
           AND o.occurred_at >= f.fe AND o.occurred_at <= f.fe + to_days(CAST(? AS INTEGER)))) AS converted
FROM first_exp f
WHERE f.variant IS NOT NULL AND f.variant <> 'unassigned'
GROUP BY f.variant
ORDER BY f.variant`, warehouse.ResolvedViewName, warehouse.OrderFactsTableName)
	rows, err := r.db.QueryContext(ctx, q, flag, from.UTC(), to.UTC(), windowDays)
	if err != nil {
		return nil, fmt.Errorf("experiment readout query: %w", err)
	}
	defer rows.Close()
	var out []ExperimentRow
	for rows.Next() {
		var e ExperimentRow
		if err := rows.Scan(&e.Variant, &e.Exposed, &e.Converted); err != nil {
			return nil, fmt.Errorf("scan experiment readout: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate experiment readout: %w", err)
	}
	return out, nil
}

func (r *DuckDBRepository) AiSurfaceReport(ctx context.Context, from, to time.Time) ([]AiSurfaceRow, error) {
	//nolint:gosec // only fixed identifiers are interpolated
	q := fmt.Sprintf(`
SELECT json_extract_string(properties, '$.surface') AS surface, COALESCE(model_version, '') AS mv,
  count(*) FILTER (WHERE json_extract_string(properties, '$.action') = 'shown'),
  count(*) FILTER (WHERE json_extract_string(properties, '$.action') = 'accepted'),
  count(*) FILTER (WHERE json_extract_string(properties, '$.action') = 'edited'),
  count(*) FILTER (WHERE json_extract_string(properties, '$.action') = 'dismissed')
FROM %s
WHERE event_type = 'ai_interaction' AND occurred_at >= ? AND occurred_at <= ?
  AND json_extract_string(properties, '$.action') IN ('shown','accepted','edited','dismissed')
  AND COALESCE(json_extract_string(properties, '$.surface'), '') <> ''
GROUP BY 1, 2
ORDER BY 1, 2`, warehouse.ResolvedViewName)
	rows, err := r.db.QueryContext(ctx, q, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("ai surface report query: %w", err)
	}
	defer rows.Close()
	var out []AiSurfaceRow
	for rows.Next() {
		var a AiSurfaceRow
		if err := rows.Scan(&a.Surface, &a.ModelVersion, &a.Shown, &a.Accepted, &a.Edited, &a.Dismissed); err != nil {
			return nil, fmt.Errorf("scan ai surface report: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ai surface report: %w", err)
	}
	return out, nil
}

var (
	_ RetentionRepository  = (*DuckDBRepository)(nil)
	_ ExperimentRepository = (*DuckDBRepository)(nil)
	_ AiSurfaceRepository  = (*DuckDBRepository)(nil)
)
