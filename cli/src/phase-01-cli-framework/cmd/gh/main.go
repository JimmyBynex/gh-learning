package main

import (
	"os"

	"github.com/learngh/gh-impl/internal/ghcmd"
)

func main() {
	code := ghcmd.Main()
	os.Exit(int(code))
}
