package referral

import (
	"log/slog"
	"time"
)

type Config struct {
	RewardXU            int64
	DailyCap            int
	LifetimeCap         int
	RedeemMaxAccountAge time.Duration
	SweepInterval       time.Duration
	CapLocation         *time.Location
}

func DefaultConfig() Config {
	return Config{
		RewardXU:            10000,
		DailyCap:            5,
		LifetimeCap:         50,
		RedeemMaxAccountAge: 168 * time.Hour,
		SweepInterval:       time.Minute,
		CapLocation:         time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60),
	}
}

type Option func(*settings)

type settings struct {
	cfg    Config
	now    func() time.Time
	logger *slog.Logger
}

func newSettings(opts []Option) settings {
	s := settings{cfg: DefaultConfig(), now: time.Now, logger: slog.Default()}
	for _, o := range opts {
		o(&s)
	}
	return s
}

func WithConfig(c Config) Option { return func(s *settings) { s.cfg = c } }

func WithClock(now func() time.Time) Option { return func(s *settings) { s.now = now } }

func WithLogger(l *slog.Logger) Option { return func(s *settings) { s.logger = l } }
