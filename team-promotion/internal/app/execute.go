package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/buidangphuc/team-promotion/internal/config"
)

func Execute(ctx context.Context, args []string) error {
	switch {
	case len(args) == 0:
		s, err := config.Load()
		if err != nil {
			return err
		}
		return Run(ctx, s)
	case len(args) == 2 && args[0] == "migrate" && args[1] == "up":
		ms, err := config.LoadMigrate()
		if err != nil {
			return err
		}
		return Migrate(ctx, ms)
	default:
		return fmt.Errorf("unknown subcommand %q", strings.Join(args, " "))
	}
}
