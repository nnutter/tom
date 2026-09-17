---
name: tom
description: Observe a Go repo's structure with `tom` before reading its source code, to assess its design from architecture down.
---

# tom

tom renders a Go codebase as stripped pseudo-Go source in successively deeper slices.
When asked to understand, review, or assess a Go repo's design, start with tom to learn its shape, then read source files only when needing to implement changes.

## Running tom

```sh
tom --depth 0 [package]  # package organization
tom --depth 1 [package]  # file organization
tom --depth 2 [package]  # data organization
tom --depth 3 [package]  # function/method metrics
```

If desired you can add the -u/--hide-unexported option.

## Assessing a design, slice by slice

Work top-down:

1. Depth 0 shows the full dependency structure.
   Ensure packages organize the code into logical clusters.
   Ensure the imports between packages represent logical dependencies.
   For example, higher-level packages import lower-level packages, and so on.
   Avoid deep package hierarchies.
2. Depth 1 adds the package-level API and file structure: types and package functions, and the package's file structure.
   Declarations are grouped under their file, showing how the package splits across files.
   The surface area of each file should be relatively small, logical, and named consistently.
   Methods should directly follow their receiver type as one consecutive run, in the same file, with no other declarations interleaved; helper functions go before the type or after the last method.
   Constructors should be organized before the type they construct.
   Test files should be testing the contents of their non-test namesake.
3. Depth 2 adds the data/behavior structure: struct fields and interface methods with their doc-comment summaries, and function/method signatures.
   The surface area of each struct should fit its logical purpose; avoid god structs.
4. Depth 3 adds file positions on every declaration and function/method metrics — lines, statements, and complexity — as `//` lines.
   Functions/methods with high line, statement, or complexity count should be assessed for refactoring options.

## Caveats

- Unexported declarations are included by default; use `-u` to narrow any slice to the exported surface.
- The map shows structure, not behavior: no call graphs, no control flow.
- Import lines are unioned across a package's files; test-only imports are separated.
