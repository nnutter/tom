// Package mapgen scans Go source under a directory and renders a layered
// text map of its structure for LLM consumption.
package mapgen

import (
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxDepth is the deepest map layer tom renders.
const MaxDepth = 3

// Options controls what Generate includes in the map.
type Options struct {
	// Root selects what to scan: empty for the whole module, a package
	// path (optionally suffixed with "/..."), or a plain directory.
	Root string
	// Depth selects the map layer: 0 packages, 1 declarations,
	// 2 signatures, members, and struct tags, 3 details and metrics.
	Depth int
	// ExportedOnly shows only exported declarations when true.
	// By default unexported declarations are included.
	ExportedOnly bool
}

// Generate writes the text map of the Go source selected by opts.Root to w.
func Generate(ctx context.Context, w io.Writer, opts Options) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validDepth(opts.Depth) {
		return fmt.Errorf("invalid depth %d: must be 0-%d", opts.Depth, MaxDepth)
	}
	dir, recurse, err := resolveTarget(opts.Root)
	if err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("scan %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("scan %s: not a directory", dir)
	}
	pkgs, err := scan(opts, dir, recurse)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("no Go packages found under %s", dir)
	}
	return render(w, opts, pkgs)
}

// resolveTarget turns a CLI package argument into a scan directory and a
// recursion flag. An empty argument selects the whole module containing
// the current directory. A trailing "/..." selects a package subtree;
// anything else selects a single package. Absolute paths and dot-relative
// paths are plain directories, which always scan recursively; other forms
// resolve as packages (module-relative or full import paths) in the
// current directory's module, falling back to plain directories outside
// any module.
func resolveTarget(pattern string) (string, bool, error) {
	if pattern == "" {
		return resolveModuleRoot()
	}
	if pattern == "..." {
		return resolveModuleRoot()
	}
	recurse := false
	if base, ok := strings.CutSuffix(pattern, "/..."); ok {
		recurse = true
		pattern = base
	}
	if pattern == "" {
		return resolveModuleRoot()
	}
	if filepath.IsAbs(pattern) {
		return pattern, true, nil
	}
	if pattern == "." || strings.HasPrefix(pattern, "./") || strings.HasPrefix(pattern, "../") {
		return pattern, recurse, nil
	}
	return resolvePackage(pattern, recurse)
}

// resolveModuleRoot selects the whole module containing the current
// directory, or the current directory outside any module.
func resolveModuleRoot() (string, bool, error) {
	_, modRoot := findModule(".")
	if modRoot == "" {
		return ".", true, nil
	}
	return modRoot, true, nil
}

// resolvePackage resolves a module-relative or full import path to a
// directory. Outside any module the pattern is a plain directory instead.
func resolvePackage(pattern string, recurse bool) (string, bool, error) {
	module, modRoot := findModule(".")
	if module == "" {
		return pattern, true, nil
	}
	rel := pattern
	if rel == module {
		rel = ""
	}
	if rel == path.Base(module) {
		rel = ""
	}
	if stripped, ok := strings.CutPrefix(rel, module+"/"); ok {
		rel = stripped
	}
	if stripped, ok := strings.CutPrefix(rel, path.Base(module)+"/"); ok {
		rel = stripped
	}
	if isFullImportPath(rel) && !inModule(module, rel) {
		return "", false, fmt.Errorf("package %q is not in module %q", pattern, module)
	}
	return filepath.Join(modRoot, rel), recurse, nil
}

// isFullImportPath reports whether p looks like a full import path.
func isFullImportPath(p string) bool {
	first, _, _ := strings.Cut(p, "/")
	return strings.Contains(first, ".")
}

// inModule reports whether import path p belongs to module.
func inModule(module, p string) bool {
	if p == module {
		return true
	}
	return hasModulePrefix(module, p)
}

type memberInfo struct {
	kind      string // "field", "embed", "method", or "func"
	text      string // body text, or "Name(signature)" for attached methods
	recv      string // receiver clause for attached methods, e.g. "(s *Store)"
	shortRecv string // receiver type without variable name, e.g. "(*Store)"
	metrics   string // function metrics for attached methods, shown at depth 3
	tag       string
	purpose   string
	pos       string
	file      string // defining source file base name for attached methods
	order     int
}

type declInfo struct {
	kind    string // "type", "func", "var", "const", or "method"
	name    string
	head    string // signature fragment shown at depth >= 2
	sigType string // declared type for vars and consts
	metrics string // function metrics shown at depth 3, e.g. "12 lines, 5 statements, complexity 2."
	purpose string
	pos     string
	file    string // owning source file base name, e.g. "server.go"
	order   int
	members []memberInfo
	method  memberInfo // standalone method body, set when kind is "method"
}

type pkgInfo struct {
	name           string
	dir            string
	files          []string
	purpose        string
	decls          []declInfo
	rawImports     map[string]struct{}
	rawTestImports map[string]struct{}
	imports        []string // same-module imports, shown at depth >= 1
	testImports    []string // same-module test-only imports, shown at depth >= 1
	allImports     []string // all imports, shown at depth 0
	allTestImports []string // all test-only imports, shown at depth 0
}

// anchorPackageDir renders a package directory anchored at the module base
// name: the module root shows as e.g. "tom" and "cmd" as "tom/cmd".
// The second return reports whether anchoring succeeded.
func anchorPackageDir(module, modRoot, pkgDir string) (string, bool) {
	abs, err := filepath.Abs(pkgDir)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(modRoot, abs)
	if err != nil {
		return "", false
	}
	base := path.Base(module)
	if rel == "." {
		return base, true
	}
	return base + "/" + filepath.ToSlash(rel), true
}

// findModule locates the nearest go.mod above root, returning its module
// path and directory, or "" when root is outside any module.
func findModule(root string) (string, string) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", ""
	}
	dir := abs
	for {
		if path := readModulePath(filepath.Join(dir, "go.mod")); path != "" {
			return path, dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

func readModulePath(file string) string {
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if path, ok := moduleField(line); ok {
			return path
		}
	}
	return ""
}

func moduleField(line string) (string, bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return "", false
	}
	if fields[0] != "module" {
		return "", false
	}
	return strings.Trim(fields[1], `"`), true
}

// validDepth reports whether d is a renderable map layer.
func validDepth(d int) bool {
	if d < 0 {
		return false
	}
	return leMaxDepth(d)
}

// leMaxDepth reports whether d is within the deepest map layer.
func leMaxDepth(d int) bool {
	return d <= MaxDepth
}

// atDepth reports whether opts renders layer level and above.
func atDepth(opts Options, level int) bool {
	return opts.Depth >= level
}

// showText reports whether text is rendered as a purpose at level.
func showText(opts Options, level int, text string) bool {
	if !atDepth(opts, level) {
		return false
	}
	return hasText(text)
}

// hasText reports whether text is non-empty.
func hasText(text string) bool {
	return text != ""
}

// isAnonymous reports whether name carries no export identity.
func isAnonymous(name string) bool {
	return name == ""
}

// blankName reports whether name is empty or the blank identifier.
func blankName(name string) bool {
	if name == "" {
		return true
	}
	return isBlankIdent(name)
}

// isBlankIdent reports whether name is the blank identifier.
func isBlankIdent(name string) bool {
	return name == "_"
}

// purpose reduces a doc comment to its first sentence.
func purpose(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	text := strings.TrimSpace(cg.Text())
	if text == "" {
		return ""
	}
	line := strings.Join(strings.Fields(text), " ")
	return truncate(firstSentence(line), 120)
}

// firstSentence returns the text through the first sentence terminator,
// or the whole text when it holds no complete sentence.
func firstSentence(s string) string {
	for i := 0; i < len(s); i++ {
		if !isTerminator(s[i]) {
			continue
		}
		if i+1 == len(s) {
			return s
		}
		if s[i+1] == ' ' {
			return s[:i+1]
		}
	}
	return s
}

// isTerminator reports whether c ends a sentence.
func isTerminator(c byte) bool {
	if c == '.' {
		return true
	}
	if c == '?' {
		return true
	}
	return c == '!'
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// firstDoc returns the first non-nil comment group.
func firstDoc(groups ...*ast.CommentGroup) *ast.CommentGroup {
	for _, g := range groups {
		if g != nil {
			return g
		}
	}
	return nil
}

// flatten renders a node as a single line, stripping the padding the
// printer leaves inside parentheses of multiline signatures.
func flatten(fset *token.FileSet, n ast.Node) string {
	var sb strings.Builder
	if err := printer.Fprint(&sb, fset, n); err != nil {
		return ""
	}
	s := strings.Join(strings.Fields(sb.String()), " ")
	s = strings.ReplaceAll(s, "( ", "(")
	s = strings.ReplaceAll(s, " )", ")")
	return strings.ReplaceAll(s, ",)", ")")
}

func shortPos(fset *token.FileSet, fname string, pos token.Pos) string {
	return fmt.Sprintf("%s:%d", fname, fset.Position(pos).Line)
}

func isExported(name string) bool {
	if blankName(name) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

// baseName reduces a type expression to its outermost type name,
// returning "" for anonymous types.
func baseName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return baseName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return baseName(t.X)
	case *ast.IndexListExpr:
		return baseName(t.X)
	default:
		return ""
	}
}

// recvClause renders a method receiver as Go source, e.g. "(s *Store)".
// The printer cannot print a field list, so the clause is built by hand.
func recvClause(fset *token.FileSet, recv *ast.FieldList) string {
	if len(recv.List) == 0 {
		return "()"
	}
	field := recv.List[0]
	var names []string
	for _, name := range field.Names {
		names = append(names, name.Name)
	}
	clause := strings.Join(names, ", ")
	if clause != "" {
		clause += " "
	}
	return "(" + clause + flatten(fset, field.Type) + ")"
}

// shortRecv renders a receiver type without its variable name,
// e.g. "(*Store)", for name-only method lines.
func shortRecv(recv *ast.FieldList) string {
	typ := recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		return "(*" + baseName(star.X) + ")"
	}
	return "(" + baseName(typ) + ")"
}

// splitMethod divides a collected "Name(signature)" method text.
func splitMethod(text string) (string, string) {
	if i := strings.Index(text, "("); i >= 0 {
		return text[:i], text[i:]
	}
	return text, ""
}
