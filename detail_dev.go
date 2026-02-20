package main

import (
	"fmt"
	"reflect"
	"strings"
)

// methodResult holds a reflected method name and its formatted return value.
type methodResult struct {
	name  string
	value string
}

// skipMethods lists method names to exclude from reflection output.
var skipMethods = map[string]bool{
	// Circular / backlinks
	"Parent": true,
	"Node":   true,

	// Redundant with dedicated sections (Node entity accessors)
	"Object":       true,
	"Notification": true,
	"Group":        true,
	"Compliance":   true,
	"Capability":   true,

	// Redundant with Name + OID
	"String": true,

	// Iterator type, cannot be reflected over
	"Subtree": true,
}

// nameOnlyMethods are methods that return entity pointers; we show just .Name().
var nameOnlyMethods = map[string]bool{
	"Module":   true,
	"Augments": true,
	"Table":    true,
	"Row":      true,
	"Entry":    true,
}

func (d *detailModel) buildDevContent() string {
	node := d.node
	if node == nil {
		return ""
	}

	var b strings.Builder

	// Header
	writeHeader(&b, nodeLabel(node)+" (inspect)")

	// Node section
	writeDevSection(&b, "Node", node, d.width)

	// Object section
	if obj := node.Object(); obj != nil {
		writeDevSection(&b, "Object [OBJECT-TYPE]", obj, d.width)
		if obj.Type() != nil {
			writeDevSection(&b, "Object.Type", obj.Type(), d.width)
		}
	}

	// Notification section
	if notif := node.Notification(); notif != nil {
		writeDevSection(&b, "Notification [NOTIFICATION-TYPE]", notif, d.width)
	}

	// Group section
	if grp := node.Group(); grp != nil {
		label := "Group [OBJECT-GROUP]"
		if grp.IsNotificationGroup() {
			label = "Group [NOTIFICATION-GROUP]"
		}
		writeDevSection(&b, label, grp, d.width)
	}

	// Compliance section
	if comp := node.Compliance(); comp != nil {
		writeDevSection(&b, "Compliance [MODULE-COMPLIANCE]", comp, d.width)
	}

	// Capability section
	if cap := node.Capability(); cap != nil {
		writeDevSection(&b, "Capability [AGENT-CAPABILITIES]", cap, d.width)
	}

	return b.String()
}

func writeDevSection(b *strings.Builder, label string, obj any, width int) {
	b.WriteByte('\n')
	b.WriteString(styles.Label.Bold(true).Render("  " + label))
	b.WriteByte('\n')

	const labelWidth = 28 // 4 indent + 24 padded name
	results := reflectMethods(obj)
	for _, r := range results {
		val := r.value
		// Word-wrap long single-line values; multi-line values (from
		// formatNameList/formatSlice) already have embedded newlines.
		if !strings.Contains(val, "\n") && width > labelWidth && len(val) > width-labelWidth {
			val = wrapValue(val, width-labelWidth, strings.Repeat(" ", labelWidth))
		}
		b.WriteString(styles.Label.Render(fmt.Sprintf("    %-24s", r.name)))
		b.WriteString(styles.Value.Render(val))
		b.WriteByte('\n')
	}
}

func reflectMethods(obj any) []methodResult {
	if obj == nil {
		return nil
	}

	v := reflect.ValueOf(obj)
	t := v.Type()
	var results []methodResult

	for i := range t.NumMethod() {
		method := t.Method(i)

		// Skip unexported
		if !method.IsExported() {
			continue
		}

		// Only zero-arg methods (receiver only)
		if method.Type.NumIn() != 1 {
			continue
		}

		// Must return at least one value
		if method.Type.NumOut() < 1 {
			continue
		}

		name := method.Name

		if skipMethods[name] {
			continue
		}

		// Call the method
		retVals := v.Method(i).Call(nil)
		ret := retVals[0]

		// Name-only methods: show just .Name() for cross-references
		if nameOnlyMethods[name] {
			val := formatNameOnly(ret)
			results = append(results, methodResult{name: name, value: val})
			continue
		}

		// Slice of named entities: show as a list of names
		if ret.Kind() == reflect.Slice && !ret.IsNil() && ret.Len() > 0 {
			if ret.Index(0).MethodByName("Name").IsValid() {
				val := formatNameList(ret)
				results = append(results, methodResult{name: name, value: val})
				continue
			}
		}

		val := formatReflectValue(ret)
		results = append(results, methodResult{name: name, value: val})
	}

	return results
}

// formatNameOnly formats an entity pointer by calling its Name() method.
func formatNameOnly(v reflect.Value) string {
	if !v.IsValid() {
		return "(nil)"
	}
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return "(nil)"
	}
	nameMethod := v.MethodByName("Name")
	if !nameMethod.IsValid() {
		return fmt.Sprintf("%v", v.Interface())
	}
	ret := nameMethod.Call(nil)
	if len(ret) > 0 {
		return fmt.Sprintf("%v", ret[0].Interface())
	}
	return "(nil)"
}

// formatNameList formats a slice of values that have a Name() method as a compact list.
func formatNameList(v reflect.Value) string {
	n := v.Len()
	limit := min(n, 10)

	names := make([]string, 0, limit)
	for i := range limit {
		elem := v.Index(i)
		ret := elem.MethodByName("Name").Call(nil)
		if len(ret) > 0 {
			names = append(names, fmt.Sprintf("%v", ret[0].Interface()))
		}
	}

	if n > limit {
		return fmt.Sprintf("[%d] %s, ...", n, strings.Join(names, ", "))
	}

	joined := "[" + strings.Join(names, ", ") + "]"
	if len(joined) < 80 {
		return joined
	}

	// Multi-line for long lists
	var b strings.Builder
	fmt.Fprintf(&b, "[%d]", n)
	for _, name := range names {
		b.WriteString("\n      " + name)
	}
	return b.String()
}

func formatReflectValue(v reflect.Value) string {
	if !v.IsValid() {
		return "(nil)"
	}

	iface := v.Interface()

	// Check for nil pointer/interface
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return "(nil)"
		}
	}

	// Check for fmt.Stringer
	if s, ok := iface.(fmt.Stringer); ok {
		return s.String()
	}

	switch v.Kind() {
	case reflect.String:
		s := v.String()
		if s == "" {
			return `""`
		}
		return s

	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())

	case reflect.Slice:
		if v.IsNil() || v.Len() == 0 {
			return "[]"
		}
		return formatSlice(v)

	case reflect.Struct:
		return formatStruct(v)

	case reflect.Ptr:
		return formatReflectValue(v.Elem())

	default:
		return fmt.Sprintf("%v", iface)
	}
}

func formatSlice(v reflect.Value) string {
	n := v.Len()
	if n == 0 {
		return "[]"
	}

	// For large slices, just show count
	if n > 10 {
		return fmt.Sprintf("[%d items]", n)
	}

	var parts []string
	for i := range n {
		elem := v.Index(i)
		parts = append(parts, formatReflectValue(elem))
	}

	// Try compact single-line first
	joined := "[" + strings.Join(parts, ", ") + "]"
	if len(joined) < 80 {
		return joined
	}

	// Multi-line
	var b strings.Builder
	fmt.Fprintf(&b, "[%d]", n)
	for _, p := range parts {
		b.WriteString("\n      " + p)
	}
	return b.String()
}

// wrapValue word-wraps s to the given line width, indenting continuation
// lines with prefix. Unlike wrapText, the first line has no prefix (it
// follows a label on the same line).
func wrapValue(s string, lineWidth int, prefix string) string {
	if lineWidth <= 0 {
		return s
	}
	words := strings.Fields(s)
	var b strings.Builder
	lineLen := 0
	for _, word := range words {
		if lineLen > 0 && lineLen+1+len(word) > lineWidth {
			b.WriteByte('\n')
			b.WriteString(prefix)
			lineLen = 0
		}
		if lineLen > 0 {
			b.WriteByte(' ')
			lineLen++
		}
		b.WriteString(word)
		lineLen += len(word)
	}
	return b.String()
}

func formatStruct(v reflect.Value) string {
	t := v.Type()
	n := t.NumField()
	var parts []string
	for i := range n {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fv := v.Field(i)
		val := formatReflectValue(fv)
		parts = append(parts, field.Name+": "+val)
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
