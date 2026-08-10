package main

import (
	"fmt"
	"os"

	"github.com/anaknegeri/agent-session/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
