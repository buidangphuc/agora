package store_test

import (
	"os"
	"testing"

	"github.com/buidangphuc/team-payment/internal/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Run(m)) }
