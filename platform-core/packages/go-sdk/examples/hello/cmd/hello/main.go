package main

import (
	"context"
	"log"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/app"
	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/config"
)

func main() {
	s, err := config.Load()
	if err == nil {
		err = app.Run(context.Background(), s)
	}
	if err != nil {
		log.Fatal(err)
	}
}
