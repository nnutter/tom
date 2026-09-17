package mapgen

import (
	"fmt"
	"io"
	"strings"
)

func render(w io.Writer, opts Options, pkgs []pkgInfo) error {
	lw := &lineWriter{w: w}
	for _, p := range pkgs {
		renderPackage(lw, opts, p)
	}
	return lw.err
}

// lineWriter writes indent-prefixed lines, retaining the first write error.
// A requested blank line materializes only before a following line, so
// blanks never lead, trail, or repeat.
type lineWriter struct {
	w       io.Writer
	err     error
	wrote   bool
	pending bool
}

// blank requests a blank line around the surrounding commented block.
func (l *lineWriter) blank() {
	l.pending = true
}

func (l *lineWriter) line(indent int, text string) {
	if l.err != nil {
		return
	}
	if l.pending {
		l.pending = false
		if l.wrote {
			if _, err := fmt.Fprintln(l.w); err != nil {
				l.err = err
				return
			}
		}
	}
	_, l.err = fmt.Fprintf(l.w, "%s%s\n", strings.Repeat("  ", indent), text)
	l.wrote = true
}

func renderPackage(lw *lineWriter, opts Options, p pkgInfo) {
	commented := showText(opts, 1, p.purpose)
	if commented {
		lw.blank()
		lw.line(0, "// "+p.purpose)
	}
	lw.line(0, fmt.Sprintf("package %s (%s, %d files)", p.name, p.dir, len(p.files)))
	renderImportBlock(lw, opts, p)
	if !atDepth(opts, 1) {
		return
	}
	for _, fname := range p.files {
		lw.line(1, fname)
		for _, d := range p.decls {
			if d.file != fname {
				continue
			}
			renderDecl(lw, opts, d)
		}
	}
}

// renderImportBlock writes the package's import lines, isolating test-only
// imports in their own block. Depth 0 shows every import; deeper layers
// show only same-module imports.
func renderImportBlock(lw *lineWriter, opts Options, p pkgInfo) {
	imports, testOnly := p.imports, p.testImports
	if !atDepth(opts, 1) {
		imports, testOnly = p.allImports, p.allTestImports
	}
	for _, imp := range imports {
		lw.line(1, `import "`+imp+`"`)
	}
	if len(testOnly) == 0 {
		return
	}
	if len(imports) > 0 {
		lw.blank()
	}
	lw.line(1, "// Test only imports")
	for _, imp := range testOnly {
		lw.line(1, `import "`+imp+`"`)
	}
}

func renderDecl(lw *lineWriter, opts Options, d declInfo) {
	if d.kind == "method" {
		renderAttachedMethod(lw, opts, d.method, 2)
		return
	}
	renderDocLines(lw, opts, d.purpose, d.metrics, 2)
	body, attached := splitMembers(d.members)
	if bracedType(d, opts) {
		renderBraceType(lw, opts, d, body)
	} else {
		lw.line(2, d.kind+" "+d.name+declHead(opts, d)+declSuffix(opts, d))
	}
	indent := 2
	if !atDepth(opts, 2) {
		indent = 3
	}
	for _, m := range attached {
		renderAttachedMethod(lw, opts, m, indent)
	}
}

// renderDocLines writes the blank separator and // comment lines above a
// declaration or method, shown at the given indent.
func renderDocLines(lw *lineWriter, opts Options, purpose, metrics string, indent int) {
	if purpose == "" && !showText(opts, 3, metrics) {
		return
	}
	lw.blank()
	if purpose != "" {
		lw.line(indent, "// "+purpose)
	}
	if showText(opts, 3, metrics) {
		lw.line(indent, "// "+metrics)
	}
}

// splitMembers divides body members from attached methods, which render as
// sibling func declarations rather than brace contents.
func splitMembers(members []memberInfo) (body, attached []memberInfo) {
	for _, m := range members {
		if m.kind == "func" {
			attached = append(attached, m)
		} else {
			body = append(body, m)
		}
	}
	return body, attached
}

// bracedType reports whether d renders with a member block.
func bracedType(d declInfo, opts Options) bool {
	if d.kind != "type" {
		return false
	}
	if !atDepth(opts, 2) {
		return false
	}
	return isBraceHead(d.head)
}

// isBraceHead reports whether head opens a member block.
func isBraceHead(head string) bool {
	if head == "struct" {
		return true
	}
	return isInterfaceHead(head)
}

// isInterfaceHead reports whether head is an interface body.
func isInterfaceHead(head string) bool {
	return head == "interface"
}

func renderBraceType(lw *lineWriter, opts Options, d declInfo, body []memberInfo) {
	if len(body) == 0 {
		lw.line(2, "type "+d.name+" "+d.head+" {}"+declSuffix(opts, d))
		return
	}
	lw.line(2, "type "+d.name+" "+d.head+" {"+declSuffix(opts, d))
	for _, m := range body {
		renderMember(lw, opts, m)
	}
	lw.line(2, "}")
}

func renderAttachedMethod(lw *lineWriter, opts Options, m memberInfo, indent int) {
	renderDocLines(lw, opts, m.purpose, m.metrics, indent)
	if atDepth(opts, 2) {
		lw.line(indent, "func "+m.recv+" "+m.text+attachedSuffix(opts, m))
	} else {
		name, _ := splitMethod(m.text)
		lw.line(indent, "func "+m.shortRecv+" "+name)
	}
}

func attachedSuffix(opts Options, m memberInfo) string {
	if !atDepth(opts, 3) {
		return ""
	}
	return " (" + m.pos + ")"
}

func declHead(opts Options, d declInfo) string {
	if !atDepth(opts, 2) {
		return ""
	}
	switch d.kind {
	case "type":
		return " " + d.head
	case "func":
		return d.head
	case "var", "const":
		return typedSuffix(d)
	default:
		return ""
	}
}

func typedSuffix(d declInfo) string {
	if d.sigType == "" {
		return ""
	}
	return " " + d.sigType
}

func declSuffix(opts Options, d declInfo) string {
	if !atDepth(opts, 3) {
		return ""
	}
	return " (" + d.pos + ")"
}

func renderMember(lw *lineWriter, opts Options, m memberInfo) {
	if showText(opts, 2, m.purpose) {
		lw.blank()
		lw.line(3, "// "+m.purpose)
	}
	lw.line(3, m.text+memberSuffix(opts, m))
}

func memberSuffix(opts Options, m memberInfo) string {
	suffix := ""
	if showText(opts, 2, m.tag) {
		suffix = " " + m.tag
	}
	if atDepth(opts, 3) {
		suffix += " (" + m.pos + ")"
	}
	return suffix
}
