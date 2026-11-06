package main

import (
	"context"
	"fmt"
	"os"

	"github.com/OhRats-Technologies/rc/pkg/cli"
)

func main() {
	ctx := context.Background()
	if err := cli.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
