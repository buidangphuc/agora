package store

type Commissionable struct {
	PlatformDiscount int64
	Base             int64
	RateBps          int
	OK               bool
}

const (
	LedgerTypeCommission         = "COMMISSION"
	LedgerTypeCommissionReversal = "COMMISSION_REVERSAL"
)

func CommissionAmount(base int64, rateBps int64) int64 { return 0 }

func ReversalTarget(commission, refunded, paid int64) int64 { return 0 }
