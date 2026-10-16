package config

import "time"

// TEST-STUB (task 2): compile-only placeholders so the red tests build. The
// implementer deletes this file and adds the real accessors to config.go.

func (s *Settings) PaymentDeadline() (time.Duration, string)        { return 0, "" }
func (s *Settings) PaymentReminderBefore() (time.Duration, string)  { return 0, "" }
func (s *Settings) LifecycleSweepInterval() (time.Duration, string) { return 0, "" }
