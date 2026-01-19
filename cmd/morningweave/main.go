package main

import (
	"os"

	"morningweave/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
