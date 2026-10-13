package outbox

// HeadsSQL exposes the claim's head scan so a test can read its query plan.
func HeadsSQL(table string) string { return headsSQL(table) }
