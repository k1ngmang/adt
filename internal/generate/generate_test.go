package generate

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedMatchRequiresNewHandler(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "expr.go")
	output := filepath.Join(dir, "expr_gen.go")
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	generate := func() {
		t.Helper()
		if err := WriteFile(input, output, "Expr", "Lit"); err != nil {
			t.Fatal(err)
		}
	}
	build := func() (string, error) {
		t.Helper()
		cmd := exec.Command("go", "test", ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOTOOLCHAIN=local")
		log, err := cmd.CombinedOutput()
		return string(log), err
	}
	write("go.mod", "module test.example/expr\n\ngo 1.22\n")
	write("state.go", `package expr

type stateSpec interface {
	Open()
	Closed()
}
`)
	if err := WriteFile(filepath.Join(dir, "state.go"), filepath.Join(dir, "state_gen.go"), "State", "Closed"); err != nil {
		t.Fatal(err)
	}
	spec := `package expr
import clock "time"

type exprSpec interface {
	Lit(value clock.Duration)
	Empty()
	Add(left, right Expr)
`
	caller := `package expr_test
import (
	"testing"
	"time"
	"test.example/expr"
)
func eval(value expr.Expr) time.Duration {
	return expr.MatchExpr(value,
		func(value time.Duration) time.Duration { return value },
		func() time.Duration { return 2 },
		func(left, right expr.Expr) time.Duration { return eval(left) + eval(right) },
`
	check := `
	)
}
func TestEval(t *testing.T) {
	if got := expr.MatchState(expr.State{}, func() string { return "open" }, func() string { return "closed" }); got != "closed" {
		t.Fatalf("zero state = %q, want closed", got)
	}
	var zero expr.Expr
	if got := eval(zero); got != 0 { t.Fatalf("zero value = %v, want 0", got) }
	if got := eval(expr.NewExprAdd(expr.NewExprLit(3), expr.NewExprEmpty())); got != 5 {
		t.Fatalf("sum = %v, want 5", got)
	}
}
`
	write("expr.go", spec+"}\n")
	write("expr_test.go", caller+check)
	generate()
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	generate()
	after, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("repeated generation changed the output")
	}
	if log, err := build(); err != nil {
		t.Fatalf("initial example did not compile and pass: %v\n%s", err, log)
	}

	write("expr.go", spec+"Neg(inner Expr)\n}\n")
	generate()
	if log, err := build(); err == nil || !strings.Contains(log, "not enough arguments in call to expr.MatchExpr") {
		t.Fatalf("expected the compiler to reject the missing handler: %v\n%s", err, log)
	}

	write("expr_test.go", caller+`
		func(inner expr.Expr) time.Duration { return -eval(inner) },
`+check+`
func TestNeg(t *testing.T) {
	if got := eval(expr.NewExprNeg(expr.NewExprLit(3))); got != -3 {
		t.Fatalf("negation = %v, want -3", got)
	}
}
`)
	if log, err := build(); err != nil {
		t.Fatalf("example with new handler did not compile and pass: %v\n%s", err, log)
	}
}
