package mapgen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const memoSrc = `// Package memo provides an in-memory cache.
package memo

import (
	"errors"
	"io"
	"sync"
)

// Store caches computed values.
type Store struct {
	// Mu guards Data.
	Mu   sync.Mutex ` + "`json:\"-\"`" + `
	data map[string]string
	ttl  int
}

// Cache is something retrievable.
type Cache interface {
	// Get returns the value for key.
	Get(key string) (string, error)
	io.Closer
}

// Status is a cache status.
type Status string

// Empty has no fields.
type Empty struct{}

// Marker has no methods.
type Marker interface{}

// DefaultTTL is the default cache TTL.
const DefaultTTL = 60

// ErrMiss is returned on absence.
var ErrMiss = errors.New("miss")

// NewStore creates a Store.
func NewStore(ttl int) *Store {
	return &Store{ttl: ttl}
}

// Get returns the value for key.
func (s *Store) Get(key string) (string, error) {
	return "", nil
}

func helper() {}
`

const subSrc = `// Package sub is a helper package.
package sub

// Run runs the helper.
func Run() {}
`

func writeSample(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "memo"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "memo", "memo.go"), []byte(memoSrc), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "sub.go"), []byte(subSrc), 0o644))
	return root
}

func generate(t *testing.T, root string, opts Options) string {
	t.Helper()
	opts.Root = root
	var buf bytes.Buffer
	require.NoError(t, Generate(t.Context(), &buf, opts))
	return buf.String()
}

func TestDepthZeroShowsPackagesOnly(t *testing.T) {
	out := generate(t, writeSample(t), Options{Depth: 0})
	assert.Contains(t, out, "package memo (memo, 1 files)")
	assert.Contains(t, out, "package sub (sub, 1 files)")
	assert.NotContains(t, out, "type Store")
	assert.NotContains(t, out, "func NewStore")
	assert.NotContains(t, out, "func Run")
}

func TestDepthOneShowsDeclarationsAndPurposes(t *testing.T) {
	out := generate(t, writeSample(t), Options{Depth: 1, ExportedOnly: true})
	assert.Contains(t, out, "    // Store caches computed values.\n    type Store\n")
	assert.Contains(t, out, "    // NewStore creates a Store.\n    func NewStore\n")
	assert.Contains(t, out, "    // DefaultTTL is the default cache TTL.\n    const DefaultTTL\n")
	assert.Contains(t, out, "    // Run runs the helper.\n    func Run\n")
	assert.Contains(t, out, "    // NewStore creates a Store.\n    func NewStore\n\n    // Get returns the value for key.\n    func (*Store) Get\n")
	assert.NotContains(t, out, "func helper")
	assert.NotContains(t, out, "field Mu")
	assert.NotContains(t, out, "method Get")
}

func TestGoStyleFormat(t *testing.T) {
	out := generate(t, writeSample(t), Options{Depth: 3})
	assert.NotContains(t, out, "──")
	assert.NotContains(t, out, "│")
	assert.NotContains(t, out, " — ")
	assert.Contains(t, out, "      // Mu guards Data.\n      Mu sync.Mutex `json:\"-\"` (memo.go:")
	assert.Contains(t, out, "    // Get returns the value for key.\n    // 3 lines, 2 statements, complexity 1.\n    func (s *Store) Get(key string) (string, error) (memo.go:")
}

func TestUnexportedShownByDefault(t *testing.T) {
	out := generate(t, writeSample(t), Options{Depth: 2})
	assert.Contains(t, out, "    func helper()\n")
	assert.Contains(t, out, "      data map[string]string\n")
}

func TestHideUnexported(t *testing.T) {
	out := generate(t, writeSample(t), Options{Depth: 1, ExportedOnly: true})
	assert.NotContains(t, out, "func helper")
	assert.Contains(t, out, "    func NewStore\n")
}

func TestDepthThreeShowsMetrics(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "m", "m.go"), "package m\n\n// Classify returns a size label.\nfunc Classify(n int) string {\n\tif n < 0 {\n\t\treturn \"neg\"\n\t}\n\tif n == 0 || n == 1 {\n\t\treturn \"small\"\n\t}\n\ttotal := 0\n\tfor i := 0; i < n; i++ {\n\t\ttotal += i\n\t}\n\treturn \"big\"\n}\n")
	out := generate(t, root, Options{Depth: 3})
	assert.Contains(t, out, "    // Classify returns a size label.\n    // 13 lines, 14 statements, complexity 5.\n    func Classify(n int) string (m.go:4)\n")

	plain := generate(t, root, Options{Depth: 2})
	assert.NotContains(t, plain, "complexity")

	methods := generate(t, writeSample(t), Options{Depth: 3})
	assert.Contains(t, methods, "    // 3 lines, 2 statements, complexity 1.\n    func (s *Store) Get(key string) (string, error) (memo.go:")
}

func TestDepthTwoShowsSignaturesAndMembers(t *testing.T) {
	out := generate(t, writeSample(t), Options{Depth: 2, ExportedOnly: true})
	assert.Contains(t, out, "    type Store struct {\n\n      // Mu guards Data.\n      Mu sync.Mutex `json:\"-\"`\n    }")
	assert.Contains(t, out, "    func (s *Store) Get(key string) (string, error)\n")
	assert.Contains(t, out, "    func NewStore(ttl int) *Store")
	assert.Contains(t, out, "    type Cache interface {\n\n      // Get returns the value for key.\n      Get(key string) (string, error)\n      io.Closer\n    }")
	assert.Contains(t, out, "    type Status string")
	assert.Contains(t, out, "    type Empty struct {}")
	assert.Contains(t, out, "    type Marker interface {}")
	assert.Contains(t, out, "    const DefaultTTL")
	assert.Contains(t, out, "    var ErrMiss")
	assert.NotContains(t, out, "memo.go:")
	assert.NotContains(t, out, "= 60")
	assert.Contains(t, out, "Mu guards Data.")
	assert.NotContains(t, out, "field ")
	assert.NotContains(t, out, "method ")
	assert.NotContains(t, out, "embed ")
}

func TestDepthThreeShowsDetails(t *testing.T) {
	out := generate(t, writeSample(t), Options{Depth: 3})
	assert.Contains(t, out, "const DefaultTTL (memo.go:")
	assert.Contains(t, out, "      Mu sync.Mutex `json:\"-\"` (memo.go:")
	assert.Contains(t, out, "Mu guards Data.")
	assert.Contains(t, out, "func NewStore(ttl int) *Store (memo.go:")
	assert.Contains(t, out, "func (s *Store) Get(key string) (string, error) (memo.go:")
}

func TestRootDirItselfIsScanned(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "main.go"),
		[]byte("package main\n\n// Main is the entry point.\nfunc Main() {}\n"),
		0o644,
	))
	out := generate(t, root, Options{Depth: 1})
	assert.Contains(t, out, "package main (., 1 files)")
	assert.Contains(t, out, "    // Main is the entry point.\n    func Main\n")
}

func writeGoFile(t *testing.T, path, src string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
}

func TestInternalImportLines(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n")
	writeGoFile(t, filepath.Join(root, "a", "a.go"), `package a

import (
	"fmt"

	"example.com/sample/b"
	"github.com/x/y"
)

var Word = fmt.Sprint("x")
`)
	writeGoFile(t, filepath.Join(root, "a", "extra.go"), `package a

import "example.com/sample/c"
`)
	writeGoFile(t, filepath.Join(root, "b", "b.go"), "package b\n")
	writeGoFile(t, filepath.Join(root, "b", "b_test.go"), `package b

import (
	"testing"

	"example.com/sample/c"
)

func TestNothing(t *testing.T) {}
`)
	writeGoFile(t, filepath.Join(root, "c", "c.go"), `package c

import "os"
`)

	depth0 := generate(t, root, Options{Depth: 0})
	assert.Contains(t, depth0, "package a (sample/a, 2 files)")
	assert.Contains(t, depth0, "package b (sample/b, 2 files)")
	assert.Contains(t, depth0, `import "example.com/sample/b"`)
	assert.Contains(t, depth0, `import "example.com/sample/c"`)
	assert.Contains(t, depth0, `import "fmt"`)
	assert.Contains(t, depth0, `import "os"`)
	assert.Contains(t, depth0, `import "testing"`)
	assert.Contains(t, depth0, `import "github.com/x/y"`)
	assert.Contains(t, depth0, "package a (sample/a, 2 files)\n  import \"fmt\"\n  import \"github.com/x/y\"\n  import \"example.com/sample/b\"\n  import \"example.com/sample/c\"\n")
	assert.Contains(t, depth0, "package b (sample/b, 2 files)\n  // Test only imports\n")
	assert.Equal(t, 1, strings.Count(depth0, "// Test only imports"))
	assert.Equal(t, 7, strings.Count(depth0, "import \""))

	pruned := generate(t, root, Options{Depth: 1})
	aIdx := strings.Index(pruned, "package a (sample/a, 2 files)")
	bIdx := strings.Index(pruned, "package b (sample/b, 2 files)")
	seg := pruned[aIdx:bIdx]
	assert.Contains(t, seg, `import "example.com/sample/b"`)
	assert.Contains(t, seg, `import "example.com/sample/c"`)
	assert.NotContains(t, seg, "Test only")
	assert.NotContains(t, pruned, `"fmt"`)
	assert.NotContains(t, pruned, `"os"`)
	assert.NotContains(t, pruned, `"testing"`)
	assert.NotContains(t, pruned, "github.com/x/y")
	assert.Contains(t, pruned, "  // Test only imports\n  import \"example.com/sample/c\"\n")
}

func TestTestOnlyImportBlock(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n")
	writeGoFile(t, filepath.Join(root, "w", "w.go"), `package w

import (
	"fmt"

	"example.com/sample/x"
)

var Word = fmt.Sprint("x")
`)
	writeGoFile(t, filepath.Join(root, "w", "w_test.go"), `package w

import (
	"testing"

	"example.com/sample/x"
	"example.com/sample/y"
)

func TestNothing(t *testing.T) {}
`)
	writeGoFile(t, filepath.Join(root, "x", "x.go"), "package x\n")
	writeGoFile(t, filepath.Join(root, "y", "y.go"), "package y\n")

	pruned := generate(t, root, Options{Depth: 1})
	assert.Contains(t, pruned, "package w (sample/w, 2 files)\n  import \"example.com/sample/x\"\n\n  // Test only imports\n  import \"example.com/sample/y\"\n")
	assert.NotContains(t, pruned, `"testing"`)
	assert.NotContains(t, pruned, `"fmt"`)

	all := generate(t, root, Options{Depth: 0})
	assert.Contains(t, all, "  import \"fmt\"\n  import \"example.com/sample/x\"\n\n  // Test only imports\n  import \"testing\"\n  import \"example.com/sample/y\"\n")
}

func TestImportLinesPrecedeDeclarations(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n")
	writeGoFile(t, filepath.Join(root, "a", "a.go"), `package a

import "example.com/sample/b"

// Word is exported.
var Word = "x"
`)
	writeGoFile(t, filepath.Join(root, "b", "b.go"), "package b\n")

	out := generate(t, root, Options{Depth: 1})
	assert.Less(t,
		strings.Index(out, `import "example.com/sample/b"`),
		strings.Index(out, "var Word"),
	)
}

func TestDepthZeroShowsAllImports(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "d", "d.go"), `package d

import (
	"fmt"

	"github.com/x/y"
)

var Word = fmt.Sprint(y.Name)
`)
	out := generate(t, root, Options{Depth: 0})
	assert.Contains(t, out, "package d (d, 1 files)")
	assert.Contains(t, out, `import "fmt"`)
	assert.Contains(t, out, `import "github.com/x/y"`)

	pruned := generate(t, root, Options{Depth: 1})
	assert.NotContains(t, pruned, "import \"")
}

func TestBlankLinesAroundComments(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "q", "q.go"), `package q

// ADoc.
func A() {}

func B() {}
`)
	writeGoFile(t, filepath.Join(root, "q", "extra.go"), `package q

// CDoc.
func C() {}
`)
	out := generate(t, root, Options{Depth: 1})
	assert.Contains(t, out, "  extra.go\n\n    // CDoc.\n    func C\n  q.go\n\n    // ADoc.\n    func A\n    func B\n")
	assert.True(t, strings.HasPrefix(out, "package q (q, 2 files)\n"), "output should start with the first package")
	assert.False(t, strings.HasSuffix(out, "\n\n"), "output should not end with a blank line")
	assert.NotContains(t, out, "\n\n\n")
}

func TestValuesHidden(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "v", "v.go"), "package v\n\n// Long is a long value.\nconst Long = \""+strings.Repeat("x", 200)+"\"\n")
	out := generate(t, root, Options{Depth: 3})
	assert.Contains(t, out, "    const Long (v.go:4)\n")
	assert.NotContains(t, out, "= \"")
}

func TestFilesGroupDeclarations(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "q", "doc.go"), "// Package q does things.\npackage q\n")
	writeGoFile(t, filepath.Join(root, "q", "alpha.go"), "package q\n\nfunc Alpha() {}\n")
	writeGoFile(t, filepath.Join(root, "q", "zeta.go"), "package q\n\nfunc Zeta() {}\n")
	out := generate(t, root, Options{Depth: 1})
	assert.Contains(t, out, "// Package q does things.\npackage q (q, 3 files)\n  alpha.go\n    func Alpha\n  doc.go\n  zeta.go\n    func Zeta\n")
}

func TestModuleAnchoredDirs(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n")
	writeGoFile(t, filepath.Join(root, "main.go"), "package sample\n")
	writeGoFile(t, filepath.Join(root, "sub", "sub.go"), "package sub\n")
	out := generate(t, root, Options{Depth: 0})
	assert.Contains(t, out, "package sample (sample, 1 files)")
	assert.Contains(t, out, "package sub (sample/sub, 1 files)")

	nested := generate(t, filepath.Join(root, "sub"), Options{Depth: 0})
	assert.Contains(t, nested, "package sub (sample/sub, 1 files)")
}

func TestPackageArgument(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "go.mod"), "module example.com/sample\n")
	writeGoFile(t, filepath.Join(root, "a", "a.go"), "package a\n")
	writeGoFile(t, filepath.Join(root, "a", "sub", "sub.go"), "package sub\n")
	t.Chdir(root)

	single := generate(t, "sample/a", Options{Depth: 0})
	assert.Contains(t, single, "package a (sample/a, 1 files)")
	assert.NotContains(t, single, "package sub")

	subtree := generate(t, "sample/a/...", Options{Depth: 0})
	assert.Contains(t, subtree, "package a (sample/a, 1 files)")
	assert.Contains(t, subtree, "package sub (sample/a/sub, 1 files)")

	full := generate(t, "example.com/sample/a", Options{Depth: 0})
	assert.Contains(t, full, "package a (sample/a, 1 files)")
	assert.NotContains(t, full, "package sub")

	whole := generate(t, "", Options{Depth: 0})
	assert.Contains(t, whole, "package a (sample/a, 1 files)")
	assert.Contains(t, whole, "package sub (sample/a/sub, 1 files)")

	var buf bytes.Buffer
	require.Error(t, Generate(t.Context(), &buf, Options{Root: "sample/nope", Depth: 0}))
	require.Error(t, Generate(t.Context(), &buf, Options{Root: "example.com/other/pkg", Depth: 0}))
}

func TestMethodsIndentOnlyAfterReceiverType(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "flow", "a.go"), `package flow

// Orphan runs before its type is declared.
func (f *Flow) Orphan() {}
`)
	writeGoFile(t, filepath.Join(root, "flow", "z.go"), `package flow

// Flow is a pipeline.
type Flow struct{}

// First follows the type.
func (f *Flow) First() {}

// Second also follows.
func (f *Flow) Second() {}

func Break() {}

// Late comes after another declaration.
func (f *Flow) Late() {}
`)
	out := generate(t, root, Options{Depth: 1})
	assert.Contains(t, out, "  a.go\n\n    // Orphan runs before its type is declared.\n    func (*Flow) Orphan\n")
	assert.Contains(t, out, "    type Flow\n\n      // First follows the type.\n      func (*Flow) First\n")
	assert.Contains(t, out, "      // Second also follows.\n      func (*Flow) Second\n")
	assert.Contains(t, out, "      func (*Flow) Second\n    func Break\n")
	assert.Contains(t, out, "    func Break\n\n    // Late comes after another declaration.\n    func (*Flow) Late\n")
	assert.Less(t, strings.Index(out, "func (*Flow) Orphan"), strings.Index(out, "type Flow"))

	deep := generate(t, root, Options{Depth: 2})
	assert.Contains(t, deep, "  a.go\n\n    // Orphan runs before its type is declared.\n    func (f *Flow) Orphan()\n")
	assert.Contains(t, deep, "    type Flow struct {}\n\n    // First follows the type.\n    func (f *Flow) First()\n")
	assert.Contains(t, deep, "    func Break()\n\n    // Late comes after another declaration.\n    func (f *Flow) Late()\n")
}

func TestMultilineSignaturesCollapse(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "m", "m.go"), `package m

// Multi spans lines.
func Multi(
	a int,
	b string,
) error {
	return nil
}

// Iface has a method.
type Iface interface {
	// Call invokes.
	Call(
		a int,
	) error
}
`)
	out := generate(t, root, Options{Depth: 2})
	assert.Contains(t, out, "    func Multi(a int, b string) error\n")
	assert.Contains(t, out, "      Call(a int) error\n")
}

func TestFirstSentenceSummary(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "m", "m.go"), `package m

// Short does one thing. It also does another.
func Short() {}

// Wrapped spans
// two lines.
func Wrapped() {}

// Asked really?
func Asked() {}
`)
	out := generate(t, root, Options{Depth: 1})
	assert.Contains(t, out, "    // Short does one thing.\n    func Short\n")
	assert.Contains(t, out, "    // Wrapped spans two lines.\n    func Wrapped\n")
	assert.Contains(t, out, "    // Asked really?\n    func Asked\n")
}

func TestErrors(t *testing.T) {
	var buf bytes.Buffer
	require.Error(t, Generate(t.Context(), &buf, Options{Root: "does-not-exist", Depth: 1}))
	require.Error(t, Generate(t.Context(), &buf, Options{Root: writeSample(t), Depth: 9}))
	require.Error(t, Generate(t.Context(), &buf, Options{Root: writeSample(t), Depth: 4}))
	require.Error(t, Generate(t.Context(), &buf, Options{Root: t.TempDir(), Depth: 1}))
}
