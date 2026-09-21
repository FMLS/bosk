package main

import (
	"os"

	"github.com/FMLS/bosk/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
