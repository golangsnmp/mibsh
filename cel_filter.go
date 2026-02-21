package main

import (
	"log"

	"github.com/golangsnmp/gomib/mib"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// celFilter holds the compiled CEL program for tree filtering.
type celFilter struct {
	env        *cel.Env
	program    cel.Program // nil when no valid expression
	expr       string      // current expression text
	err        string      // compilation error, "" if ok
	evalErr    string      // first runtime eval error, "" if ok
	matchCount int         // direct matches from last evaluation
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
		// Should not happen with static variable declarations
		log.Fatal("cel env: ", err)
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
	vars := map[string]any{
		"name":         node.Name(),
		"oid":          node.OID().String(),
		"kind":         node.Kind().String(),
		"module":       "",
		"status":       "",
		"access":       "",
		"type_name":    "",
		"base_type":    "",
		"description":  "",
		"units":        "",
		"display_hint": "",
		"language":     "",
		"is_tc":        false,
		"is_table":     false,
		"is_row":       false,
		"is_column":    false,
		"is_scalar":    false,
		"is_counter":   false,
		"is_gauge":     false,
		"is_string":    false,
		"is_enum":      false,
		"is_bits":      false,
		"arc":          uint64(node.Arc()),
		"depth":        int64(len(node.OID())),
	}

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
