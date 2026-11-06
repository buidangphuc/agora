package events

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	eventsv1 "github.com/buidangphuc/team-referral/generated/platform/events/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
)

const (
	AggregateType = "referral"
	typePrefix    = "platform.referral.v1."

	MessageAttributed = "ReferralAttributed"
	MessageRejected   = "ReferralRejected"
	MessageReleased   = "ReferralRewardReleased"
)

var idNamespace = uuid.NewSHA1(uuid.NameSpaceOID, []byte("team-referral.events"))

func StableEventID(message, key string) string {
	return uuid.NewSHA1(idNamespace, []byte(message+"\x00"+key)).String()
}

func Attributed(refereeID, referrerID string, at time.Time) (outbox.Row, error) {
	return row(MessageAttributed, refereeID, refereeID, at, &referralv1.ReferralAttributed{
		RefereeId:    refereeID,
		ReferrerId:   referrerID,
		AttributedAt: timestamppb.New(at),
	})
}

func Rejected(refereeID string, reason referralv1.ReferralRejectReason, at time.Time) (outbox.Row, error) {
	return row(MessageRejected, refereeID, refereeID, at, &referralv1.ReferralRejected{
		RefereeId: refereeID,
		Reason:    reason,
	})
}

func Released(rewardID, referrerID, refereeID string, amountXU int64, at time.Time) (outbox.Row, error) {
	return row(MessageReleased, rewardID, refereeID, at, &referralv1.ReferralRewardReleased{
		RewardId:   rewardID,
		ReferrerId: referrerID,
		RefereeId:  refereeID,
		AmountXu:   amountXU,
		ReleasedAt: timestamppb.New(at),
	})
}

func row(message, idKey, refereeID string, at time.Time, payload proto.Message) (outbox.Row, error) {
	body, err := proto.Marshal(payload)
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal %s: %w", message, err)
	}
	eventID := StableEventID(message, idKey)
	eventType := typePrefix + message
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       eventType,
		OccurredAt: timestamppb.New(at),
		Payload:    body,
	})
	if err != nil {
		return outbox.Row{}, fmt.Errorf("marshal envelope for %s: %w", message, err)
	}
	return outbox.Row{
		EventID:       eventID,
		AggregateType: AggregateType,
		AggregateID:   refereeID,
		EventType:     eventType,
		Payload:       value,
	}, nil
}
