package expr

//go:generate go run ../../cmd/adtgen Expr

type exprSpec interface {
	Lit(value int)
	Add(left, right Expr)
	Mul(left, right Expr)
}

// Eval evaluates an arithmetic expression.
func Eval(expr Expr) int {
	return MatchExpr(expr,
		func(value int) int { return value },
		func(left, right Expr) int { return Eval(left) + Eval(right) },
		func(left, right Expr) int { return Eval(left) * Eval(right) },
	)
}
