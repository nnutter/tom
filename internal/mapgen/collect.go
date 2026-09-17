package mapgen

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func scan(opts Options, dir string, recurse bool) ([]pkgInfo, error) {
	s := &scanner{fset: token.NewFileSet(), opts: opts, root: dir}
	s.module, s.modRoot = findModule(dir)
	if recurse {
		if err := filepath.WalkDir(dir, s.visit); err != nil {
			return nil, err
		}
	} else if err := s.collect(dir); err != nil {
		return nil, err
	}
	slices.SortFunc(s.pkgs, func(a, b pkgInfo) int { return cmp.Compare(a.dir, b.dir) })
	s.resolveImports()
	return s.pkgs, nil
}

type scanner struct {
	fset    *token.FileSet
	opts    Options
	pkgs    []pkgInfo
	module  string
	modRoot string
	root    string
}

func (s *scanner) collect(path string) error {
	p, ok, err := parseDir(s.fset, s.root, path, s.opts)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if s.module != "" {
		if anchored, ok := anchorPackageDir(s.module, s.modRoot, path); ok {
			p.dir = anchored
		}
	}
	s.pkgs = append(s.pkgs, p)
	return nil
}

// resolveImports reduces each package's collected imports to the sorted
// same-module paths shown as Go import lines. Standard-library and
// third-party imports are pruned.
func (s *scanner) resolveImports() {
	for i := range s.pkgs {
		resolvePackageImports(&s.pkgs[i], s.module)
	}
}

func (s *scanner) visit(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}
	if !d.IsDir() {
		return nil
	}
	if s.root != path && skippableDir(d.Name()) {
		return filepath.SkipDir
	}
	return s.collect(path)
}

func parseDir(fset *token.FileSet, root, dir string, opts Options) (pkgInfo, bool, error) {
	files, err := goFiles(dir)
	if err != nil {
		return pkgInfo{}, false, err
	}
	if len(files) == 0 {
		return pkgInfo{}, false, nil
	}
	b := &pkgBuilder{
		methods:     map[string][]memberInfo{},
		imports:     map[string]struct{}{},
		testImports: map[string]struct{}{},
		sources:     map[string][]byte{},
	}
	for _, fname := range files {
		if err := b.collectFile(fset, dir, fname, opts); err != nil {
			return pkgInfo{}, false, err
		}
	}
	return b.finish(root, dir)
}

// goFiles lists the Go source files in dir in sorted order.
func goFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			files = append(files, e.Name())
		}
	}
	slices.Sort(files)
	return files, nil
}

type pkgBuilder struct {
	pkg         pkgInfo
	testName    string
	order       int
	methods     map[string][]memberInfo
	imports     map[string]struct{}
	testImports map[string]struct{}
	sources     map[string][]byte
}

func (b *pkgBuilder) collectDoc(f *ast.File) {
	if b.pkg.purpose == "" {
		b.pkg.purpose = purpose(f.Doc)
	}
}

func (b *pkgBuilder) collectFile(fset *token.FileSet, dir, fname string, opts Options) error {
	path := filepath.Join(dir, fname)
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	b.collectNames(f, fname)
	b.collectDoc(f)
	b.collectImports(f, strings.HasSuffix(fname, "_test.go"))
	b.sources[fname] = src
	b.pkg.files = append(b.pkg.files, fname)
	for _, node := range f.Decls {
		switch d := node.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				b.order = collectSpec(fset, d, spec, fname, opts, &b.pkg, b.order)
			}
		case *ast.FuncDecl:
			b.order = collectFunc(fset, d, fname, b.sources[fname], opts, &b.pkg, b.methods, b.order)
		}
	}
	return nil
}

func (b *pkgBuilder) collectImports(f *ast.File, isTest bool) {
	for _, imp := range f.Imports {
		path := unquote(imp.Path.Value)
		if path == "" {
			continue
		}
		if isTest {
			b.testImports[path] = struct{}{}
		} else {
			b.imports[path] = struct{}{}
		}
	}
}

func (b *pkgBuilder) collectNames(f *ast.File, fname string) {
	if strings.HasSuffix(fname, "_test.go") {
		if b.testName == "" {
			b.testName = f.Name.Name
		}
		return
	}
	if b.pkg.name == "" {
		b.pkg.name = f.Name.Name
	}
}

func (b *pkgBuilder) finish(root, dir string) (pkgInfo, bool, error) {
	if b.pkg.name == "" {
		b.pkg.name = b.testName
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return pkgInfo{}, false, err
	}
	b.pkg.dir = rel
	b.pkg.rawImports = b.imports
	b.pkg.rawTestImports = b.testImports
	placeMethods(&b.pkg, b.methods)
	return b.pkg, true, nil
}

// unquote strips the quotes from an import path, returning "" when invalid.
func unquote(s string) string {
	path, err := strconv.Unquote(s)
	if err != nil {
		return ""
	}
	return path
}

func resolvePackageImports(p *pkgInfo, module string) {
	main := sortedImports(p.rawImports, module)
	var testOnly []string
	for _, path := range sortedImports(p.rawTestImports, module) {
		if _, ok := p.rawImports[path]; !ok {
			testOnly = append(testOnly, path)
		}
	}
	p.imports = internalOnly(main, module)
	p.testImports = internalOnly(testOnly, module)
	p.allImports = main
	p.allTestImports = testOnly
	p.rawImports = nil
	p.rawTestImports = nil
}

// sortedImports returns the paths of set ordered stdlib first, then
// third-party, then same-module imports, alphabetical within each group.
func sortedImports(set map[string]struct{}, module string) []string {
	keys := make([]string, 0, len(set))
	for path := range set {
		keys = append(keys, path)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if group := cmp.Compare(importGroup(module, a), importGroup(module, b)); group != 0 {
			return group
		}
		return cmp.Compare(a, b)
	})
	return keys
}

// importGroup ranks an import path: stdlib first, then third-party,
// then same-module imports.
func importGroup(module, path string) int {
	if isInternalImport(module, path) {
		return 2
	}
	if isFullImportPath(path) {
		return 1
	}
	return 0
}

// internalOnly keeps the paths belonging to module.
func internalOnly(paths []string, module string) []string {
	var kept []string
	for _, path := range paths {
		if isInternalImport(module, path) {
			kept = append(kept, path)
		}
	}
	return kept
}

// isInternalImport reports whether path belongs to module.
func isInternalImport(module, path string) bool {
	if module == "" {
		return false
	}
	if path == module {
		return true
	}
	return hasModulePrefix(module, path)
}

// hasModulePrefix reports whether path is under module.
func hasModulePrefix(module, path string) bool {
	return strings.HasPrefix(path, module+"/")
}

// collectSpec records one type, var, or const spec. It returns the next order.
func collectSpec(
	fset *token.FileSet,
	gen *ast.GenDecl,
	spec ast.Spec,
	fname string,
	opts Options,
	p *pkgInfo,
	order int,
) int {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return collectTypeSpec(fset, gen, s, fname, opts, p, order)
	case *ast.ValueSpec:
		return collectValueSpec(fset, gen, s, fname, opts, p, order)
	default:
		return order
	}
}

func collectTypeSpec(
	fset *token.FileSet,
	gen *ast.GenDecl,
	s *ast.TypeSpec,
	fname string,
	opts Options,
	p *pkgInfo,
	order int,
) int {
	if hidden(opts, s.Name.Name) {
		return order
	}
	d := declInfo{kind: "type", name: s.Name.Name, file: fname, order: order}
	d.purpose = purpose(firstDoc(s.Doc, gen.Doc, s.Comment))
	d.pos = shortPos(fset, fname, s.Pos())
	d.head, d.members = typeHeadMembers(fset, s, fname, opts)
	p.decls = append(p.decls, d)
	return order + 1
}

func typeHeadMembers(
	fset *token.FileSet,
	s *ast.TypeSpec,
	fname string,
	opts Options,
) (string, []memberInfo) {
	switch t := s.Type.(type) {
	case *ast.StructType:
		return "struct", structFields(fset, t, fname, opts)
	case *ast.InterfaceType:
		return "interface", interfaceMethods(fset, t, fname, opts)
	default:
		underlying := flatten(fset, s.Type)
		if s.Assign.IsValid() {
			return "= " + underlying, nil
		}
		return underlying, nil
	}
}

func collectValueSpec(
	fset *token.FileSet,
	gen *ast.GenDecl,
	s *ast.ValueSpec,
	fname string,
	opts Options,
	p *pkgInfo,
	order int,
) int {
	kind := valueKind(gen)
	sigType := valueType(fset, s)
	for _, name := range s.Names {
		if hidden(opts, name.Name) {
			continue
		}
		d := declInfo{kind: kind, name: name.Name, sigType: sigType, file: fname, order: order}
		order++
		d.purpose = purpose(firstDoc(s.Doc, gen.Doc, s.Comment))
		d.pos = shortPos(fset, fname, name.Pos())
		p.decls = append(p.decls, d)
	}
	return order
}

func valueKind(gen *ast.GenDecl) string {
	if gen.Tok == token.CONST {
		return "const"
	}
	return "var"
}

func valueType(fset *token.FileSet, s *ast.ValueSpec) string {
	if s.Type == nil {
		return ""
	}
	return flatten(fset, s.Type)
}

// collectFunc records one function or method declaration.
// It returns the next order.
func collectFunc(
	fset *token.FileSet,
	d *ast.FuncDecl,
	fname string,
	src []byte,
	opts Options,
	p *pkgInfo,
	methods map[string][]memberInfo,
	order int,
) int {
	if hasReceiver(d) {
		return collectMethod(fset, d, fname, src, opts, methods, order)
	}
	if hidden(opts, d.Name.Name) {
		return order
	}
	p.decls = append(p.decls, declInfo{
		kind:    "func",
		name:    d.Name.Name,
		head:    strings.TrimPrefix(flatten(fset, d.Type), "func"),
		metrics: funcMetrics(fset, src, d),
		purpose: purpose(firstDoc(d.Doc, nil, nil)),
		pos:     shortPos(fset, fname, d.Pos()),
		file:    fname,
		order:   order,
	})
	return order + 1
}

func collectMethod(
	fset *token.FileSet,
	d *ast.FuncDecl,
	fname string,
	src []byte,
	opts Options,
	methods map[string][]memberInfo,
	order int,
) int {
	recv := baseName(d.Recv.List[0].Type)
	if hiddenMethod(opts, d.Name.Name, recv) {
		return order
	}
	methods[recv] = append(methods[recv], memberInfo{
		kind:      "func",
		text:      d.Name.Name + strings.TrimPrefix(flatten(fset, d.Type), "func"),
		recv:      recvClause(fset, d.Recv),
		shortRecv: shortRecv(d.Recv),
		metrics:   funcMetrics(fset, src, d),
		purpose:   purpose(firstDoc(d.Doc, nil, nil)),
		pos:       shortPos(fset, fname, d.Pos()),
		file:      fname,
		order:     order,
	})
	return order + 1
}

// placeMethods walks declarations and methods in source order, indenting a
// method under its receiver type only while the method directly follows
// that type in the same file. Any other declaration between the type and
// the method breaks the run, and the method renders as a standalone
// declaration at its own position instead. Methods whose receiver type is
// not declared in the package are kept as standalone declarations so
// nothing is silently dropped.
func placeMethods(p *pkgInfo, methods map[string][]memberInfo) {
	var ordered []placedMethod
	for recv, ms := range methods {
		for _, m := range ms {
			ordered = append(ordered, placedMethod{recv: recv, info: m})
		}
	}
	slices.SortFunc(ordered, func(a, b placedMethod) int { return cmp.Compare(a.info.order, b.info.order) })
	out := make([]declInfo, 0, len(p.decls)+len(ordered))
	anchor := -1
	di, mi := 0, 0
	for itemsRemain(p.decls, ordered, di, mi) {
		if pm, ok := nextMethod(p.decls, ordered, di, mi); ok {
			mi++
			if followsAnchor(out, anchor, pm) {
				out[anchor].members = append(out[anchor].members, pm.info)
				continue
			}
			anchor = -1
			out = append(out, standaloneMethod(pm.info))
			continue
		}
		d := p.decls[di]
		di++
		out = append(out, d)
		if d.kind == "type" {
			anchor = len(out) - 1
		} else {
			anchor = -1
		}
	}
	p.decls = out
}

// itemsRemain reports whether declarations or methods are left to place.
func itemsRemain(decls []declInfo, ordered []placedMethod, di, mi int) bool {
	if di < len(decls) {
		return true
	}
	return mi < len(ordered)
}

// nextMethod returns the next method in source order, reporting false when
// the next item is a declaration rather than a method.
func nextMethod(decls []declInfo, ordered []placedMethod, di, mi int) (placedMethod, bool) {
	if mi >= len(ordered) {
		return placedMethod{}, false
	}
	if di >= len(decls) {
		return ordered[mi], true
	}
	if ordered[mi].info.order >= decls[di].order {
		return placedMethod{}, false
	}
	return ordered[mi], true
}

// followsAnchor reports whether method pm continues the run indented under
// the type at out[anchor]: same receiver name and same file.
func followsAnchor(out []declInfo, anchor int, pm placedMethod) bool {
	if anchor < 0 {
		return false
	}
	if out[anchor].name != pm.recv {
		return false
	}
	return out[anchor].file == pm.info.file
}

// placedMethod pairs a collected method with its receiver type name.
type placedMethod struct {
	recv string
	info memberInfo
}

func standaloneMethod(m memberInfo) declInfo {
	name, _ := splitMethod(m.text)
	return declInfo{
		kind:   "method",
		name:   name,
		method: m,
		file:   m.file,
		order:  m.order,
	}
}

type memberBuilder struct {
	fset  *token.FileSet
	fname string
	opts  Options
	out   []memberInfo
}

func (b *memberBuilder) add(kind, text, tag, doc, pos string) {
	b.out = append(b.out, memberInfo{
		kind: kind, text: text, tag: tag, purpose: doc, pos: pos, order: len(b.out),
	})
}

func (b *memberBuilder) collectIfaceField(field *ast.Field) {
	doc := purpose(firstDoc(field.Doc, nil, field.Comment))
	pos := shortPos(b.fset, b.fname, field.Pos())
	if len(field.Names) == 0 {
		if hiddenEmbed(b.opts, field.Type) {
			return
		}
		b.add("embed", flatten(b.fset, field.Type), "", doc, pos)
		return
	}
	for _, name := range field.Names {
		if hidden(b.opts, name.Name) {
			continue
		}
		b.add("method", name.Name+ifaceSig(b.fset, field), "", doc, pos)
	}
}

func (b *memberBuilder) collectStructField(field *ast.Field) {
	typ := flatten(b.fset, field.Type)
	doc := purpose(firstDoc(field.Doc, nil, field.Comment))
	pos := shortPos(b.fset, b.fname, field.Pos())
	if len(field.Names) == 0 {
		if hiddenEmbed(b.opts, field.Type) {
			return
		}
		b.add("embed", typ, "", doc, pos)
		return
	}
	tag := structTag(field)
	for _, name := range field.Names {
		if hidden(b.opts, name.Name) {
			continue
		}
		b.add("field", name.Name+" "+typ, tag, doc, pos)
	}
}

func structFields(fset *token.FileSet, t *ast.StructType, fname string, opts Options) []memberInfo {
	b := &memberBuilder{fset: fset, fname: fname, opts: opts}
	for _, field := range t.Fields.List {
		b.collectStructField(field)
	}
	return b.out
}

func structTag(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	return field.Tag.Value
}

func interfaceMethods(fset *token.FileSet, t *ast.InterfaceType, fname string, opts Options) []memberInfo {
	b := &memberBuilder{fset: fset, fname: fname, opts: opts}
	if t.Methods == nil {
		return nil
	}
	for _, field := range t.Methods.List {
		b.collectIfaceField(field)
	}
	return b.out
}

func ifaceSig(fset *token.FileSet, field *ast.Field) string {
	if ft, ok := field.Type.(*ast.FuncType); ok {
		return strings.TrimPrefix(flatten(fset, ft), "func")
	}
	return flatten(fset, field.Type)
}

// hidden reports whether name is excluded from the map.
func hidden(opts Options, name string) bool {
	return opts.ExportedOnly && !isExported(name)
}

// hiddenMethod reports whether a method is excluded from the map.
func hiddenMethod(opts Options, name, recv string) bool {
	if hidden(opts, name) {
		return true
	}
	return hidden(opts, recv)
}

// hiddenEmbed reports whether an embedded field is excluded from the map.
func hiddenEmbed(opts Options, typ ast.Expr) bool {
	name := baseName(typ)
	if isAnonymous(name) {
		return false
	}
	return hidden(opts, name)
}

// skippableDir reports whether a directory is never scanned.
func skippableDir(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	return isVendorDir(name)
}

// isVendorDir reports whether name is the vendor directory.
func isVendorDir(name string) bool {
	return name == "vendor"
}

// hasReceiver reports whether d declares a method receiver.
func hasReceiver(d *ast.FuncDecl) bool {
	if d.Recv == nil {
		return false
	}
	return hasFields(d.Recv)
}

// hasFields reports whether a field list is non-empty.
func hasFields(fl *ast.FieldList) bool {
	return len(fl.List) > 0
}
