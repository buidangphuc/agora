package referral

import (
	"strconv"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pagination"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// decodePage resolves the effective (limit, offset) from a paginated request.
// The cursor is an opaque decimal offset produced by encodeCursor (the SDK's
// base64 token would change what clients already hold, so the encoding stays
// local); an empty or unparseable cursor starts at offset 0. page_size is
// clamped to a sane range.
func decodePage(req *referralv1.ListReferralRewardsRequest) (limit, offset int) {
	var size int
	if p := req.GetPage(); p != nil {
		size = int(p.GetPageSize())
		offset = decodeCursor(p.GetCursor())
	}
	return pagination.Clamp(size, defaultPageSize, maxPageSize), offset
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
