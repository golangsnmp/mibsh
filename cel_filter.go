package main

import (
	"fmt"

	"github.com/golangsnmp/gomib/mib"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// celFilter holds the compiled CEL program for tree filtering.
type celFilter struct {
	env        *cel.Env
	program    cel.Program    // nil when no valid expression
	expr       string         // current expression text
	err        string         // compilation error, "" if ok
	evalErr    string         // first runtime eval error, "" if ok
	envErr     string         // CEL environment init error, "" if ok
	matchCount int            // direct matches from last evaluation
	activation map[string]any // reusable activation map for eval calls
}

func newCelFilter() *celFilter {
	env, err := cel.NewEnv(
		cel.Variable("name", cel.StringType),
		cel.Variable("oid", cel.StringType),
		cel.Variable("kind", cel.StringType),
		cel.Variable("module", cel.StringType),
		cel.Variable("status", cel.StringType),
		cel.Variable("access", cel.StringType),
		cel.Variable("type_name", cel.StringType),
		cel.Variable("base_type", cel.StringType),
		cel.Variable("description", cel.StringType),
		cel.Variable("units", cel.StringType),
		cel.Variable("display_hint", cel.StringType),
		cel.Variable("language", cel.StringType),
		cel.Variable("is_tc", cel.BoolType),
		cel.Variable("is_table", cel.BoolType),
		cel.Variable("is_row", cel.BoolType),
		cel.Variable("is_column", cel.BoolType),
		cel.Variable("is_scalar", cel.BoolType),
		cel.Variable("is_counter", cel.BoolType),
		cel.Variable("is_gauge", cel.BoolType),
		cel.Variable("is_string", cel.BoolType),
		cel.Variable("is_enum", cel.BoolType),
		cel.Variable("is_bits", cel.BoolType),
		cel.Variable("arc", cel.UintType),
		cel.Variable("depth", cel.IntType),
		ext.Strings(),
	)
	if err != nil {
		return &celFilter{envErr: fmt.Sprintf("cel env init: %v", err)}
	}
	return &celFilter{env: env}
}

func (f *celFilter) compile(expr string) {
	f.expr = expr
	f.program = nil
	f.err = ""
	f.evalErr = ""
	f.matchCount = 0

	if expr == "" {
		return
	}

	if f.envErr != "" {
		f.err = f.envErr
		return
	}

	ast, issues := f.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		f.err = issues.Err().Error()
		return
	}

	// Verify output type is bool
	if ast.OutputType() != cel.BoolType {
		f.err = "expression must return bool"
		return
	}

	prg, err := f.env.Program(ast)
	if err != nil {
		f.err = err.Error()
		return
	}
	f.program = prg
}

func (f *celFilter) buildActivation(node *mib.Node) map[string]any {
	vars := f.activation
	if vars == nil {
		vars = make(map[string]any, 25)
		f.activation = vars
	}

	// Always-present fields
	vars["name"] = node.Name()
	vars["oid"] = node.OID().String()
	vars["kind"] = node.Kind().String()
	vars["arc"] = uint64(node.Arc())
	vars["depth"] = int64(len(node.OID()))

	// Reset optional fields to defaults
	vars["module"] = ""
	vars["status"] = ""
	vars["access"] = ""
	vars["type_name"] = ""
	vars["base_type"] = ""
	vars["description"] = ""
	vars["units"] = ""
	vars["display_hint"] = ""
	vars["language"] = ""
	vars["is_tc"] = false
	vars["is_table"] = false
	vars["is_row"] = false
	vars["is_column"] = false
	vars["is_scalar"] = false
	vars["is_counter"] = false
	vars["is_gauge"] = false
	vars["is_string"] = false
	vars["is_enum"] = false
	vars["is_bits"] = false

	if mod := node.Module(); mod != nil {
		vars["module"] = mod.Name()
		vars["language"] = mod.Language().String()
	}

	if obj := node.Object(); obj != nil {
		vars["status"] = obj.Status().String()
		vars["access"] = obj.Access().String()
		vars["description"] = obj.Description()
		vars["units"] = obj.Units()
		vars["display_hint"] = obj.EffectiveDisplayHint()
		vars["is_table"] = obj.IsTable()
		vars["is_row"] = obj.IsRow()
		vars["is_column"] = obj.IsColumn()
		vars["is_scalar"] = obj.IsScalar()
		if t := obj.Type(); t != nil {
			vars["type_name"] = t.Name()
			vars["base_type"] = t.EffectiveBase().String()
			vars["is_tc"] = t.IsTextualConvention()
			vars["is_counter"] = t.IsCounter()
			vars["is_gauge"] = t.IsGauge()
			vars["is_string"] = t.IsString()
			vars["is_enum"] = t.IsEnumeration()
			vars["is_bits"] = t.IsBits()
		}
	} else if status, desc, ok := nodeEntityProps(node); ok {
		vars["status"] = status.String()
		vars["description"] = desc
	}

	return vars
}

func (f *celFilter) eval(node *mib.Node) bool {
	if f.program == nil {
		return true
	}
	activation := f.buildActivation(node)
	out, _, err := f.program.Eval(activation)
	if err != nil {
		if f.evalErr == "" {
			f.evalErr = err.Error()
		}
		return false
	}
	result, ok := out.Value().(bool)
	return ok && result
}
