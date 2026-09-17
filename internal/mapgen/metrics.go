package mapgen

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// funcMetrics summarizes a function body as "N lines, M statements,
// complexity C", or "" when the declaration has no body. Lines exclude
// blank lines; statements count AST statement nodes; complexity starts
// at 1 and adds one per branch, case, or logical operator.
func funcMetrics(fset *token.FileSet, src []byte, fn *ast.FuncDecl) string {
	if fn.Body == nil {
		return ""
	}
	lines := bodyLines(fset, src, fn.Body)
	statements, complexity := bodyStats(fn.Body)
	return fmt.Sprintf("%s, %s, complexity %d.", plural(lines, "line"), plural(statements, "statement"), complexity)
}

// plural renders n with a correctly pluralized unit.
func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func bodyLines(fset *token.FileSet, src []byte, body *ast.BlockStmt) int {
	lines := strings.Split(string(src), "\n")
	start := fset.Position(body.Pos()).Line
	end := fset.Position(body.End()).Line
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	count := 0
	for i := start; i <= end; i++ {
		if strings.TrimSpace(lines[i-1]) != "" {
			count++
		}
	}
	return count
}

func bodyStats(body *ast.BlockStmt) (statements, complexity int) {
	complexity = 1
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(ast.Stmt); ok {
			statements++
		}
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			complexity++
		}
		if be, ok := n.(*ast.BinaryExpr); ok && isLogicalOp(be.Op) {
			complexity++
		}
		return true
	})
	return statements, complexity
}

// isLogicalOp reports whether op is a short-circuiting logical operator.
func isLogicalOp(op token.Token) bool {
	if op == token.LAND {
		return true
	}
	return isLorOp(op)
}

// isLorOp reports whether op is the logical OR operator.
func isLorOp(op token.Token) bool {
	return op == token.LOR
}
