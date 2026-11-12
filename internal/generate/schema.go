package generate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"unicode"
	"unicode/utf8"
)

type schema struct {
	Package    string
	Type       string
	ResultType string
	Imports    []importSpec
	Variants   []variant
	Zero       variant
}

func (s schema) HasFields() bool {
	for _, variant := range s.Variants {
		if len(variant.Fields) != 0 {
			return true
		}
	}
	return false
}

type importSpec struct {
	Alias string
	Path  string
}

type variant struct {
	Name   string
	Fields []field
}

type field struct {
	Name string
	Type string
}

// DefaultOutput returns the conventional output filename for a type.
func DefaultOutput(typeName string) string {
	return typeStem(typeName) + "_gen.go"
}

func typeStem(typeName string) string {
	first, width := utf8.DecodeRuneInString(typeName)
	return string(unicode.ToLower(first)) + typeName[width:]
}

func readSchema(filename, typeName, zero string) (schema, error) {
	if !token.IsIdentifier(typeName) || !ast.IsExported(typeName) {
		return schema{}, fmt.Errorf("type name %q must be an exported Go identifier", typeName)
	}
	specName := typeStem(typeName) + "Spec"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.SkipObjectResolution)
	if err != nil {
		return schema{}, err
	}
	names := declarations(file)
	spec, ok := names[specName].(*ast.TypeSpec)
	if !ok {
		return schema{}, fmt.Errorf("%s: expected a declaration for type %s", filename, specName)
	}
	iface, ok := spec.Type.(*ast.InterfaceType)
	if !ok || spec.TypeParams != nil || spec.Assign.IsValid() {
		return schema{}, fmt.Errorf("%s: %s must be a non-generic interface declaration", fset.Position(spec.Pos()), specName)
	}
	result := schema{Package: file.Name.Name, Type: typeName}
	variants := make(map[string]bool)
	selectors := make(map[string]token.Pos)
	generated := map[string]token.Pos{
		typeName: spec.Pos(), "Match" + typeName: spec.Pos(), "adt" + typeName + "Variant": spec.Pos(),
	}
	for _, member := range iface.Methods.List {
		entry, err := readVariant(fset, member, selectors)
		if err != nil {
			return schema{}, err
		}
		if variants[entry.Name] {
			return schema{}, fmt.Errorf("%s: duplicate variant %s", fset.Position(member.Pos()), entry.Name)
		}
		variants[entry.Name] = true
		generated["New"+typeName+entry.Name] = member.Pos()
		generated["adt"+typeName+"Case"+entry.Name] = member.Pos()
		result.Variants = append(result.Variants, entry)
		if entry.Name == zero || (zero == "" && len(result.Variants) == 1) {
			result.Zero = entry
		}
	}
	if len(result.Variants) == 0 {
		return schema{}, fmt.Errorf("%s: %s must declare at least one variant", fset.Position(spec.Pos()), specName)
	}
	if result.Zero.Name == "" {
		return schema{}, fmt.Errorf("%s: zero variant %q is not declared in %s", fset.Position(spec.Pos()), zero, specName)
	}
	for name, pos := range generated {
		if declaration, exists := names[name]; exists {
			return schema{}, fmt.Errorf("%s: generated name %s conflicts with declaration at %s", fset.Position(pos), name, fset.Position(declaration.Pos()))
		}
	}
	result.Imports, err = usedImports(fset, file, selectors, generated)
	if err != nil {
		return schema{}, err
	}
	used := make(map[string]bool)
	ast.Inspect(spec, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok {
			used[ident.Name] = true
		}
		return true
	})
	result.ResultType = "adtResult"
	for used[result.ResultType] {
		result.ResultType += "_"
	}
	return result, nil
}

func readVariant(fset *token.FileSet, member *ast.Field, selectors map[string]token.Pos) (variant, error) {
	signature, ok := member.Type.(*ast.FuncType)
	if !ok || len(member.Names) != 1 || !member.Names[0].IsExported() {
		return variant{}, fmt.Errorf("%s: each variant must be an exported method signature", fset.Position(member.Pos()))
	}
	result := variant{Name: member.Names[0].Name}
	if signature.Results != nil && len(signature.Results.List) != 0 {
		return variant{}, fmt.Errorf("%s: variant %s must not return values", fset.Position(member.Pos()), result.Name)
	}
	fieldNames := make(map[string]bool)
	for _, parameter := range signature.Params.List {
		_, variadic := parameter.Type.(*ast.Ellipsis)
		if variadic || len(parameter.Names) == 0 {
			return variant{}, fmt.Errorf("%s: variant fields must be named and non-variadic", fset.Position(parameter.Pos()))
		}
		var fieldType bytes.Buffer
		if err := format.Node(&fieldType, fset, parameter.Type); err != nil {
			return variant{}, fmt.Errorf("%s: format field type: %w", fset.Position(parameter.Pos()), err)
		}
		ast.Inspect(parameter.Type, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok {
				if qualifier, ok := selector.X.(*ast.Ident); ok {
					selectors[qualifier.Name] = qualifier.Pos()
				}
			}
			return true
		})
		for _, name := range parameter.Names {
			if name.Name == "_" || fieldNames[name.Name] {
				return variant{}, fmt.Errorf("%s: invalid or duplicate field name %q", fset.Position(name.Pos()), name.Name)
			}
			fieldNames[name.Name] = true
			result.Fields = append(result.Fields, field{Name: name.Name, Type: fieldType.String()})
		}
	}
	return result, nil
}

func usedImports(fset *token.FileSet, file *ast.File, selectors, generated map[string]token.Pos) ([]importSpec, error) {
	var imports []importSpec
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid import path: %w", fset.Position(spec.Pos()), err)
		}
		name := path.Base(importPath)
		var alias string
		if spec.Name != nil {
			alias = spec.Name.Name
			name = alias
		}
		if name == "." {
			return nil, fmt.Errorf("%s: dot imports are unsupported in a schema file", fset.Position(spec.Pos()))
		}
		if _, needed := selectors[name]; !needed || name == "_" {
			continue
		}
		if _, conflict := generated[name]; conflict {
			return nil, fmt.Errorf("%s: import name %q conflicts with a generated name", fset.Position(spec.Pos()), name)
		}
		imports = append(imports, importSpec{Alias: alias, Path: importPath})
		delete(selectors, name)
	}
	var missing string
	for name, pos := range selectors {
		if missing == "" || pos < selectors[missing] {
			missing = name
		}
	}
	if missing != "" {
		return nil, fmt.Errorf("%s: qualifier %q has no matching import; add an explicit alias if the package name differs from its path", fset.Position(selectors[missing]), missing)
	}
	return imports, nil
}

func declarations(file *ast.File) map[string]ast.Node {
	names := make(map[string]ast.Node)
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Recv == nil {
				names[declaration.Name.Name] = declaration
			}
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					names[spec.Name.Name] = spec
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						names[name.Name] = name
					}
				}
			}
		}
	}
	return names
}
