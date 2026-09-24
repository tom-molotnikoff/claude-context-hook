package main

import (
	"os"
	"time"

	"github.com/tom-molotnikoff/claude-context-hook/internal/cli"
)

func main() {
	os.Exit(cli.Run(cli.Env{
		Args:   os.Args[1:],
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Getenv: os.Getenv,
		Now:    time.Now,
	}))
}
