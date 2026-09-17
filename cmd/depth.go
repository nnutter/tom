package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/pflag"

	"github.com/nnutter/tom/internal/mapgen"
)

// depthAlias pairs a depth flag alias with its layer and help description.
type depthAlias struct {
	name        string
	depth       int
	description string
}

// depthAliases lists the depth flag aliases in layer order.
var depthAliases = []depthAlias{
	{"package", 0, "packages with all import lines"},
	{"file", 1, "top-level declarations plus one-line purposes"},
	{"function", 2, "signatures, bodies, and methods grouped with their receiver type"},
	{"sub-function", 3, "positions and function metrics"},
}

// depthValue is a pflag.Value accepting a map depth as a number (0-3)
// or an alias (package, file, function, sub-function).
type depthValue int

var _ pflag.Value = (*depthValue)(nil)

// Set parses s as a depth number or alias, matching aliases
// case-insensitively.
func (d *depthValue) Set(s string) error {
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 {
			return fmt.Errorf("invalid depth %d: must be %s", n, depthChoices())
		}
		if n > mapgen.MaxDepth {
			return fmt.Errorf("invalid depth %d: must be %s", n, depthChoices())
		}
		*d = depthValue(n)
		return nil
	}
	for _, a := range depthAliases {
		if strings.EqualFold(s, a.name) {
			*d = depthValue(a.depth)
			return nil
		}
	}
	return fmt.Errorf("invalid depth %q: must be %s", s, depthChoices())
}

// String returns the canonical numeric form of the depth.
func (d depthValue) String() string {
	return strconv.Itoa(int(d))
}

// depthChoices summarizes the accepted depth flag values.
func depthChoices() string {
	names := make([]string, 0, len(depthAliases))
	for _, a := range depthAliases {
		names = append(names, a.name)
	}
	return fmt.Sprintf("0-%d or %s", mapgen.MaxDepth, strings.Join(names, ", "))
}

// Type reports the accepted depth flag values for usage text.
func (d depthValue) Type() string {
	return "0-3|package|file|function|sub-function"
}

// depthCompletions lists depth flag choices for shell completion,
// pairing each value with its description.
func depthCompletions() []string {
	choices := make([]string, 0, len(depthAliases)*2)
	for _, a := range depthAliases {
		choices = append(choices, fmt.Sprintf("%d\t%s", a.depth, a.description))
		choices = append(choices, a.name+"\t"+a.description)
	}
	return choices
}
