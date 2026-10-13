package outbox

// HeadsSQL exposes the claim's head scans so a test can read their query plans.
func HeadsSQL(table string) string { return headsSQL(table, scanAll) }

// HeadsAfterSQL is the scan that starts after a cursor ($2, $3).
func HeadsAfterSQL(table string) string { return headsSQL(table, scanAfter) }

// HeadsThroughSQL is the wrap-around scan up to a cursor ($2, $3).
func HeadsThroughSQL(table string) string { return headsSQL(table, scanThrough) }
