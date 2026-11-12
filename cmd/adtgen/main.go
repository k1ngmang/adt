package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/k1ngmang/adt/internal/generate"
)

func main() {
	input := flag.String("in", os.Getenv("GOFILE"), "Go file containing the variant specification (defaults to GOFILE)")
	typeName := flag.String("type", "", "name of the generated type")
	zero := flag.String("zero", "", "variant used for the zero value (defaults to the first variant)")
	output := flag.String("out", "", "destination Go file (defaults to <type>_gen.go next to the input)")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: adtgen [flags] Type")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 1 && *typeName == "" {
		*typeName = flag.Arg(0)
	} else if flag.NArg() != 0 || *typeName == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *input == "" {
		fmt.Fprintln(os.Stderr, "adtgen: provide -in or run adtgen through go generate")
		os.Exit(2)
	}
	if *output == "" {
		*output = filepath.Join(filepath.Dir(*input), generate.DefaultOutput(*typeName))
	}
	if err := generate.WriteFile(*input, *output, *typeName, *zero); err != nil {
		fmt.Fprintln(os.Stderr, "adtgen:", err)
		os.Exit(1)
	}
}
