// Package cmd implements the tom command line interface.
package cmd

import (
	"context"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/nnutter/tom/internal/mapgen"
)

// NewRootCmd builds the tom root command. version is shown by the
// --version flag; main passes the build-time main.version value here.
func NewRootCmd(version string) *cobra.Command {
	var depth depthValue = 1
	var hideUnexported bool
	cmd := &cobra.Command{
		Use:     "tom [package]",
		Version: version,
		Short:   "Generate a text map of Go source code structure",
		Long: `tom (tomography) renders the structure of Go source code as a text
map with successively deeper slices for LLM consumption.

A package argument selects what to map:
  (empty)    the whole module containing the current directory
  pkg        one package, e.g. tom/cmd
  pkg/...    the package subtree, e.g. tom/cmd/...

Full import paths work too. Absolute paths and dot-relative paths
are plain directories, which always scan recursively.

Depth layers, as numbers or aliases:
  0, package       packages with all import lines
  1, file          top-level declarations plus one-line purposes
  2, function      signatures, struct/interface bodies with member
                   purposes, methods grouped with their receiver type,
                   and struct tags
  3, sub-function  positions and function metrics

At depth 0 every import is listed; deeper layers prune to
same-module imports. Test-only imports render in a separate block
under a // Test only imports comment.
Function metrics render as // comment lines on the declaration.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := ""
			if len(args) > 0 {
				pattern = args[0]
			}
			return mapgen.Generate(cmd.Context(), cmd.OutOrStdout(), mapgen.Options{
				Root:         pattern,
				Depth:        int(depth),
				ExportedOnly: hideUnexported,
			})
		},
	}
	cmd.Flags().VarP(&depth, "depth", "d", "map depth ("+depth.Type()+")")
	cmd.Flags().BoolVarP(&hideUnexported, "hide-unexported", "u", false, "hide unexported declarations")
	_ = cmd.RegisterFlagCompletionFunc("depth", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return depthCompletions(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

// Execute runs the tom CLI with Fang-enhanced help and errors.
func Execute(version string) error {
	return fang.Execute(context.Background(), NewRootCmd(version), fang.WithVersion(version))
}
