package config_test

import (
	"testing"

	"github.com/buidangphuc/team-analytics/internal/config"
)

func TestListingReplayGroup_DefaultDiffersFromLiveGroup(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if s.Kafka.ListingReplayGroup != "team-analytics-listing-replay" {
		t.Fatalf("ListingReplayGroup = %q", s.Kafka.ListingReplayGroup)
	}
	if s.Kafka.ListingReplayGroup == s.Kafka.ListingConsumerGroup {
		t.Fatal("replay group equals the live group")
	}
}

func TestListingReplayGroup_EqualToLiveGroupIsRejected(t *testing.T) {
	localEnv(t)
	t.Setenv("KAFKA_LISTING_CONSUMER_GROUP", "shared-group")
	t.Setenv("KAFKA_LISTING_REPLAY_GROUP", "shared-group")
	_, err := config.Load()
	mustErrNaming(t, err, "KAFKA_LISTING_REPLAY_GROUP", "KAFKA_LISTING_CONSUMER_GROUP")
}

func TestListingReplayGroup_EmptyIsRejected(t *testing.T) {
	localEnv(t)
	t.Setenv("KAFKA_LISTING_REPLAY_GROUP", "   ")
	_, err := config.Load()
	mustErrNaming(t, err, "KAFKA_LISTING_REPLAY_GROUP")
}
