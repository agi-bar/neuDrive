package main

import (
	"os"

	"github.com/agi-bar/agenthub/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
