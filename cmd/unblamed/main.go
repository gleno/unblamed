package main

import (
	"os"

	"github.com/gleno/unblamed/internal/cli"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(cli.Run(cli.Env{Dir: cwd, Stdout: os.Stdout, Stderr: os.Stderr}, os.Args[1:]))
}
