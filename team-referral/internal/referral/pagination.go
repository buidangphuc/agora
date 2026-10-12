package service

import (
	"strconv"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// decodePage resolves the effective (limit, offset) from a paginated request.
// The cursor is an opaque decimal offset produced by encodeCursor; an empty or
// unparseable cursor starts at offset 0. page_size is clamped to a sane range.
func decodePage(req *referralv1.ListReferralRewardsRequest) (limit, offset int) {
	limit = defaultPageSize
	if p := req.GetPage(); p != nil {
		if ps := int(p.GetPageSize()); ps > 0 {
			limit = ps
		}
		offset = decodeCursor(p.GetCursor())
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	return limit, offset
}

// encodeCursor turns a row offset into the opaque cursor clients echo back.
func encodeCursor(offset int) string {
	return strconv.Itoa(offset)
}

// decodeCursor parses an offset cursor; anything invalid resets to the start.
func decodeCursor(cursor string) int {
	if cursor == "" {
		return 0
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
