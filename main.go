// Command tom generates layered text maps of Go source code.
package main

import (
	"os"

	"github.com/nnutter/tom/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
