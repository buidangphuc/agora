package store

import "time"

type OrderSettings struct {
	AutoCompleteAfter time.Duration
	ReturnWindow      time.Duration
	ShipByAfter       time.Duration
}

func ResolveOrderOptions(opts ...OrderRepoOption) OrderSettings {
	c := newOrderRepoConfig(opts)
	return OrderSettings{AutoCompleteAfter: c.autoCompleteAfter, ReturnWindow: c.returnWindow, ShipByAfter: c.shipByAfter}
}

type StatusSettings struct {
	CancelReason int32
	CompletedBy  int32
}

func ResolveStatusOptions(opts ...StatusOption) StatusSettings {
	c := newStatusConfig(opts)
	return StatusSettings{CancelReason: c.cancelReason, CompletedBy: c.completedBy}
}

func ResolveReturnAutoApproveAfter(opts ...ReturnRepoOption) time.Duration {
	return newReturnRepoConfig(opts).autoApproveAfter
}

func StampAfter(t time.Time, d time.Duration) *time.Time { return stampAfter(t, d) }
