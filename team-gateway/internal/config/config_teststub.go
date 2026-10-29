package config

import "errors"

// TEST-STUB (task 1.2): compile-only placeholder so the red tests build. The implementation
// replaces this file by defining Load on config.Load[Settings].
func Load() (Settings, error) { return Settings{}, errors.New("config.Load not implemented") }
