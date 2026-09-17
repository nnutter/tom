# tom

tom (short for tomography) is a CLI that generates a text map of Go source code in successively deeper slices, so an AI agent can see the structure of a codebase without loading all of the source into context.

## Usage

```sh
tom [package] [--depth N|alias] [-u]
```

Depth aliases are `package`, `file`, `function`, and `sub-function` for layers 0-3.

With no argument, tom maps the whole Go module containing the current directory.
Otherwise the argument is a package: `tom/cmd` maps one package, `tom/cmd/...` maps its subtree, and a full import path works too.
Absolute paths and dot-relative paths are plain directories, which always scan recursively.

| Depth | Slice |
| ----- | ----- |
| 0, package | Packages with all import lines: name, directory, file count |
| 1, file | + top-level declarations with one-line purposes from doc comments (default) |
| 2, function | + signatures, struct/interface bodies with member doc summaries, methods grouped with their receiver type, and struct tags |
| 3, sub-function | + file positions and function metrics as `//` lines |

`-u` (`--hide-unexported`) shows only exported declarations; unexported ones are included by default.

Example (this repo at depth 0):

```text
package main (tom, 1 files)
  import "github.com/nnutter/tom/cmd"
package cmd (tom/cmd, 1 files)
  import "github.com/nnutter/tom/internal/mapgen"
package mapgen (tom/internal/mapgen, 2 files)
```

Doc comments render Go-style as `//` lines above their element, and nesting is two spaces per level — no tree-drawing characters.
A blank line precedes each commented declaration.

## Notes

- Hidden directories, directories starting with `_`, and `vendor` are skipped; everything else with `.go` files (including `_test.go`) is mapped.
- Declarations appear in source order, grouped by file; packages are sorted by directory.
- A method renders under its receiver type only when it directly follows the type in the same file.
- Any other declaration between the type and the method breaks the run, and the method appears at its own position instead.
- Each package shows its imports as Go `import` lines, unioned across its files and sorted stdlib first, then third-party, then same-module.
- At depth 0 all imports are shown; at deeper layers standard-library and third-party imports are pruned, leaving same-module imports.
- Imports appearing only in test files render in a separate block under a `// Test only imports` comment.
  Outside a Go module, no import lines are shown.
