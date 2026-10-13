// Package store owns Postgres access. hello keeps no data, so it holds no
// queries; a real service puts them here. Its migrations live outside, in the
// root migrations package, and are applied by a migrate job, not at boot.
package store
