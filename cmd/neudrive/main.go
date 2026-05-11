package main

import (
	"os"

	"github.com/agi-bar/neuDrive/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
