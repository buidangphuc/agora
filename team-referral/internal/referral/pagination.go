package referral

import (
	"strconv"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

func decodePage(req *referralv1.ListReferralRewardsRequest) (limit, offset int) {
	size := 0
	if p := req.GetPage(); p != nil {
		size = int(p.GetPageSize())
		offset = decodeCursor(p.GetCursor())
	}
	return pagination.Clamp(size, defaultPageSize, maxPageSize), offset
}

func encodeCursor(offset int) string {
	return strconv.Itoa(offset)
}

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
