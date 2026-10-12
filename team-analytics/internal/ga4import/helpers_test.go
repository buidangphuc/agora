package ga4import_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/marcboeker/go-duckdb"
)

const evDDL = "CREATE TABLE ev(event_date VARCHAR, event_timestamp BIGINT, event_name VARCHAR, " +
	"user_pseudo_id VARCHAR, event_params STRUCT(key VARCHAR, value STRUCT(string_value VARCHAR, int_value BIGINT))[], " +
	"traffic_source STRUCT(name VARCHAR, medium VARCHAR, source VARCHAR))"

type ev struct {
	user, source, medium, campaign, page string
	sid                                  int64
	at                                   time.Time
}

func q(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func compact(d time.Time) string { return d.Format("20060102") }

// writeFixture writes events_YYYYMMDD.parquet (GA4 export schema) into dir.
func writeFixture(t *testing.T, dir string, day time.Time, evs []ev) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(evDDL); err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		params := fmt.Sprintf("[{'key':'ga_session_id','value':{'string_value':NULL,'int_value':%d}},"+
			"{'key':'source','value':{'string_value':%s,'int_value':NULL}},"+
			"{'key':'medium','value':{'string_value':%s,'int_value':NULL}},"+
			"{'key':'campaign','value':{'string_value':%s,'int_value':NULL}},"+
			"{'key':'page_location','value':{'string_value':%s,'int_value':NULL}}]",
			e.sid, q(e.source), q(e.medium), q(e.campaign), q(e.page))
		ins := fmt.Sprintf("INSERT INTO ev VALUES (%s, %d, 'view_item', %s, %s::STRUCT(key VARCHAR, value STRUCT(string_value VARCHAR, int_value BIGINT))[], "+
			"{'name':%s,'medium':%s,'source':%s})",
			q(compact(day)), e.at.UnixMicro(), q(e.user), params, q(e.campaign), q(e.medium), q(e.source))
		if _, err := db.Exec(ins); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "events_"+compact(day)+".parquet")
	if _, err := db.Exec(fmt.Sprintf("COPY ev TO %s (FORMAT PARQUET)", q(out))); err != nil {
		t.Fatal(err)
	}
}
