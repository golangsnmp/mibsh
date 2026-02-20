package main

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// filterFields is the sorted list of all CEL filter field names.
var filterFields = func() []string {
	f := []string{
		"name", "oid", "kind", "module", "status", "access",
		"type_name", "base_type", "description", "units",
		"display_hint", "language",
		"is_tc", "is_table", "is_row", "is_column", "is_scalar",
		"is_counter", "is_gauge", "is_string", "is_enum", "is_bits",
		"arc", "depth",
	}
	sort.Strings(f)
	return f
}()

// filterEnumValues maps field names to their valid string values.
var filterEnumValues = map[string][]string{
	"kind":      {"scalar", "table", "row", "column", "node", "internal", "notification", "group", "compliance", "capabilities"},
	"status":    {"current", "deprecated", "obsolete", "mandatory", "optional"},
	"access":    {"not-accessible", "read-only", "read-write", "read-create", "accessible-for-notify", "write-only"},
	"base_type": {"Integer32", "Unsigned32", "Counter32", "Counter64", "Gauge32", "TimeTicks", "IpAddress", "OCTET STRING", "OBJECT IDENTIFIER", "BITS", "Opaque", "SEQUENCE"},
	"language":  {"SMIv1", "SMIv2", "SPPI"},
}

// filterMethods is the list of string methods available in CEL expressions.
var filterMethods = []string{"contains", "startsWith", "endsWith"}

// filterBarModel is the filter bar UI component wrapping a textinput and celFilter.
type filterBarModel struct {
	input  textinput.Model
	filter *celFilter
	active bool
	width  int

	tc tabCompleter // tab completion state
}

func newFilterBar() filterBarModel {
	ti := newStyledInput("f ", 256)
	ti.Placeholder = `name.contains("Error") or kind == "scalar"  [? help]`
	s := ti.Styles()
	s.Focused.Placeholder = styles.Label
	s.Blurred.Placeholder = styles.Label
	ti.SetStyles(s)

	return filterBarModel{
		input:  ti,
		filter: newCelFilter(),
	}
}

func (f *filterBarModel) activate() {
	f.active = true
	f.input.SetWidth(max(20, f.width-4))
	// Keep existing text so user can edit previous filter
	f.input.Focus()
}

func (f *filterBarModel) deactivate() {
	f.active = false
	f.input.Blur()
}

func (f *filterBarModel) clear() {
	f.input.SetValue("")
	f.filter.compile("")
}

func (f *filterBarModel) recompile() {
	f.filter.compile(f.input.Value())
}

func (f *filterBarModel) isFiltering() bool {
	return f.filter.program != nil
}

func (f *filterBarModel) resetCompletion() {
	f.tc.reset()
}

// tabComplete performs context-aware tab completion in the filter bar.
func (f *filterBarModel) tabComplete() {
	value := f.input.Value()
	cursor := f.input.Position()
	if cursor > len(value) {
		cursor = len(value)
	}

	// Scan backwards from cursor to find token start.
	// Word boundaries: space, parens, operators, comma, quote.
	tokenStart := cursor
	for tokenStart > 0 {
		ch := value[tokenStart-1]
		if ch == ' ' || ch == '(' || ch == ')' || ch == '!' ||
			ch == '<' || ch == '>' || ch == '=' || ch == ',' || ch == '"' {
			break
		}
		tokenStart--
	}

	token := value[tokenStart:cursor]

	// Determine context by looking at what precedes the token.
	var candidates []string

	// Check for method completion: preceding char is '.'
	if tokenStart > 0 && value[tokenStart-1] == '.' {
		candidates = filterMethods
	} else if isEnumValueContext(value, tokenStart) {
		// After FIELD == ", complete enum values for that field
		field := extractPrecedingField(value, tokenStart)
		if vals, ok := filterEnumValues[field]; ok {
			candidates = vals
		}
	} else {
		candidates = filterFields
	}

	if len(candidates) == 0 {
		return
	}

	completion, ok := f.tc.complete(token, candidates)
	if !ok {
		return
	}

	newValue := value[:tokenStart] + completion + value[cursor:]
	f.input.SetValue(newValue)
	f.input.SetCursor(tokenStart + len(completion))
}

// isEnumValueContext checks if the cursor is inside a quoted enum value context,
// i.e. after FIELD == " or FIELD != ".
func isEnumValueContext(value string, tokenStart int) bool {
	// We need to find a quote just before the token start
	if tokenStart == 0 {
		return false
	}
	if value[tokenStart-1] != '"' {
		return false
	}
	// Scan back from the quote to find == or !=
	pos := tokenStart - 2
	// Skip spaces
	for pos >= 0 && value[pos] == ' ' {
		pos--
	}
	// Check for == or !=
	if pos >= 1 && (value[pos-1:pos+1] == "==" || value[pos-1:pos+1] == "!=") {
		return true
	}
	return false
}

// extractPrecedingField extracts the field name before an operator in
// FIELD == " or FIELD != " context.
func extractPrecedingField(value string, tokenStart int) string {
	// tokenStart-1 is the quote, scan back past it and the operator
	pos := tokenStart - 2
	// Skip spaces
	for pos >= 0 && value[pos] == ' ' {
		pos--
	}
	// Skip past == or !=
	if pos >= 1 && (value[pos-1:pos+1] == "==" || value[pos-1:pos+1] == "!=") {
		pos -= 2
	} else {
		return ""
	}
	// Skip spaces
	for pos >= 0 && value[pos] == ' ' {
		pos--
	}
	if pos < 0 {
		return ""
	}
	// Scan back to find the field name
	end := pos + 1
	for pos >= 0 {
		ch := value[pos]
		if ch == ' ' || ch == '(' || ch == ')' || ch == '!' ||
			ch == '<' || ch == '>' || ch == '=' || ch == ',' {
			break
		}
		pos--
	}
	return value[pos+1 : end]
}

func (f *filterBarModel) view() string {
	if !f.active {
		return ""
	}

	var b strings.Builder
	b.WriteString(f.input.View())

	if f.filter.err != "" {
		errText := styles.Status.ErrorMsg.Render("  " + f.filter.err)
		b.WriteString(errText)
	} else if f.filter.program != nil {
		b.WriteString(styles.Status.SuccessMsg.Render("  " + matchBadge(f.filter.matchCount)))
		if f.filter.evalErr != "" {
			b.WriteString(styles.Status.WarnMsg.Render("  eval: " + f.filter.evalErr))
		}
	}

	return b.String()
}

// styledList renders items separated by a plain gap, with a 2-char indent.
func styledList(items []string, style lipgloss.Style, sep string) string {
	rendered := make([]string, len(items))
	for i, item := range items {
		rendered[i] = style.Render(item)
	}
	return "  " + strings.Join(rendered, sep)
}

// renderFilterHelp builds the styled filter reference overlay content.
func renderFilterHelp() string {
	hdr := styles.Header.Info
	val := styles.Value
	lbl := styles.Label

	var b strings.Builder

	b.WriteString(hdr.Render("Filter Reference"))
	b.WriteString("\n\n")

	// String fields
	b.WriteString(hdr.Render("Fields (string)"))
	b.WriteString("\n")
	b.WriteString(styledList(
		[]string{"name", "oid", "kind", "module", "status", "access", "type_name"},
		val, "  ",
	))
	b.WriteString("\n")
	b.WriteString(styledList(
		[]string{"base_type", "description", "units", "display_hint", "language"},
		val, "  ",
	))
	b.WriteString("\n\n")

	// Bool fields
	b.WriteString(hdr.Render("Fields (bool)"))
	b.WriteString("\n")
	b.WriteString(styledList(
		[]string{"is_tc", "is_table", "is_row", "is_column", "is_scalar"},
		val, "  ",
	))
	b.WriteString("\n")
	b.WriteString(styledList(
		[]string{"is_counter", "is_gauge", "is_string", "is_enum", "is_bits"},
		val, "  ",
	))
	b.WriteString("\n\n")

	// Numeric fields
	b.WriteString(hdr.Render("Fields (numeric)"))
	b.WriteString("\n")
	b.WriteString("  " + val.Render("arc") + " " + lbl.Render("(uint)") + "    " + val.Render("depth") + " " + lbl.Render("(int)"))
	b.WriteString("\n\n")

	// Enum values
	writeValues := func(label string, values []string) {
		b.WriteString(hdr.Render("Values: " + label))
		b.WriteString("\n")
		b.WriteString(styledList(values, val, "  "))
		b.WriteString("\n\n")
	}

	// Build help values from filterEnumValues (single source of truth).
	// Values containing spaces are shown with surrounding quotes to
	// indicate the user must quote them in CEL expressions.
	for _, field := range []string{"kind", "status", "access", "base_type", "language"} {
		vals := filterEnumValues[field]
		display := make([]string, len(vals))
		for i, v := range vals {
			if strings.Contains(v, " ") {
				display[i] = `"` + v + `"`
			} else {
				display[i] = v
			}
		}
		writeValues(field, display)
	}

	// String methods
	b.WriteString(hdr.Render("String methods"))
	b.WriteString("\n")
	b.WriteString(styledList(
		[]string{`.contains("x")`, `.startsWith("x")`, `.endsWith("x")`},
		val, "  ",
	))
	b.WriteString("\n\n")

	// Operators
	b.WriteString(hdr.Render("Operators"))
	b.WriteString("\n")
	b.WriteString(styledList([]string{"==", "!=", "<", ">", "<=", ">="}, val, "  "))
	b.WriteString("  ")
	b.WriteString(styledList([]string{"and", "or", "!", "(", ")"}, lbl, "  "))
	b.WriteString("\n\n")

	// Examples
	b.WriteString(hdr.Render("Examples"))
	b.WriteString("\n")
	b.WriteString(val.Render(`  kind == "scalar"`))
	b.WriteString("\n")
	b.WriteString(val.Render(`  name.contains("Error") and status == "current"`))
	b.WriteString("\n")
	b.WriteString(val.Render("  is_counter or is_gauge"))
	b.WriteString("\n")
	b.WriteString(val.Render(`  module == "IF-MIB" and depth < 10`))

	return b.String()
}
