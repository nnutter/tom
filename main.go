// Command tom generates layered text maps of Go source code.
package main

import (
	"os"

	"github.com/nnutter/tom/cmd"
)

// version is the tom release version. Set it at build time with:
//
//	go build -ldflags "-X main.version=v1.2.3" .
var version = "dev"

func main() {
	if err := cmd.Execute(version); err != nil {
		os.Exit(1)
	}
}
