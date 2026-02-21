package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/gomib/mib"
)

// Icon constants (crush-inspired).
const (
	IconSuccess = "\u2713" // checkmark
	IconError   = "\u00d7" // multiplication sign
	IconPending = "\u25cf" // filled circle
	IconArrow   = "\u2192" // right arrow
	IconLoading = "\u22ef" // midline horizontal ellipsis
	IconWarn    = "\u25b2" // small up triangle

	BorderThick = "\u258c" // left half block

	DiagFill = "\u2571" // box drawings light diagonal

	ScrollThumbChar = "\u2503" // heavy vertical line
	ScrollTrackChar = "\u2502" // light vertical line

	ExpandIcon   = "\u25bc " // down triangle + space
	CollapseIcon = "\u25b6 " // right triangle + space
	LeafIcon     = "  "
)

// palette defines all colors in one place. Changing values here
// recolors the entire UI.
var palette = struct {
	// Grays / text hierarchy
	Fg       color.Color // normal foreground
	Muted    color.Color // labels, deprecated text
	Faint    color.Color // descriptions, status text
	Subtle   color.Color // separators, breadcrumbs
	Dim      color.Color // very dim separators
	Node     color.Color // regular node foreground
	Internal color.Color // internal nodes, info severity

	// UI chrome
	Primary    color.Color // charple purple, accent borders, selection
	PrimaryFg  color.Color // text on primary bg
	Secondary  color.Color // dolly pink
	Tertiary   color.Color // bok teal, prompts
	HeaderBlue color.Color // section headers

	// Backgrounds
	BgBase    color.Color // base background
	BgLighter color.Color // lighter panels, overlays
	BgSubtle  color.Color // subtle highlights, borders

	// Borders
	Border      color.Color // unfocused borders
	BorderFocus color.Color // focused borders

	// Kind colors
	Green  color.Color // scalar, index, TC badge
	Cyan   color.Color // table kind, links
	Teal   color.Color // row kind
	Blue   color.Color // column kind, minor severity
	Yellow color.Color // notification kind
	Pink   color.Color // group kind, warning
	Purple color.Color // compliance/capability kind
	Mauve  color.Color // style severity

	// Severity
	Fatal  color.Color
	Severe color.Color
	Error  color.Color

	// SNMP-specific
	Connected    color.Color
	Disconnected color.Color
}{
	Fg:       lipgloss.Color("#DFDBDD"),
	Muted:    lipgloss.Color("#858392"),
	Faint:    lipgloss.Color("#BFBCC8"),
	Subtle:   lipgloss.Color("#605F6B"),
	Dim:      lipgloss.Color("#4D4C57"),
	Node:     lipgloss.Color("#DFDBDD"),
	Internal: lipgloss.Color("#858392"),

	Primary:    lipgloss.Color("#6B50FF"),
	PrimaryFg:  lipgloss.Color("#FFFAF1"),
	Secondary:  lipgloss.Color("#FF60FF"),
	Tertiary:   lipgloss.Color("#68FFD6"),
	HeaderBlue: lipgloss.Color("#00A4FF"),

	BgBase:    lipgloss.Color("#201F26"),
	BgLighter: lipgloss.Color("#2D2C35"),
	BgSubtle:  lipgloss.Color("#3A3943"),

	Border:      lipgloss.Color("#3A3943"),
	BorderFocus: lipgloss.Color("#6B50FF"),

	Green:  lipgloss.Color("#00FFB2"),
	Cyan:   lipgloss.Color("#00A4FF"),
	Teal:   lipgloss.Color("#12C78F"),
	Blue:   lipgloss.Color("#4FBEFE"),
	Yellow: lipgloss.Color("#E8FE96"),
	Pink:   lipgloss.Color("#FF60FF"),
	Purple: lipgloss.Color("#AD6EFF"),
	Mauve:  lipgloss.Color("#D46EFF"),

	Fatal:  lipgloss.Color("#FF577D"),
	Severe: lipgloss.Color("#EB4268"),
	Error:  lipgloss.Color("#FF985A"),

	Connected:    lipgloss.Color("#00FFB2"),
	Disconnected: lipgloss.Color("#858392"),
}

// appStyles groups all visual styles by semantic role.
type appStyles struct {
	// Shared styles used across multiple components
	Pane       lipgloss.Style // padded container for panes
	Label      lipgloss.Style // muted field labels
	Value      lipgloss.Style // normal field values
	Prompt     lipgloss.Style // text input prompts
	StatusText lipgloss.Style // faint info text
	EmptyText  lipgloss.Style // faint empty-state text
	Separator  lipgloss.Style // vertical/horizontal separators
	Subtle     lipgloss.Style // subtle text for underlines etc

	// Header bar (crush-style)
	Header headerStyles

	// Breadcrumb
	Breadcrumb    lipgloss.Style
	BreadcrumbSep lipgloss.Style

	// Tree
	Tree treeStyles

	// Status bar (typed messages)
	Status statusStyles

	// Dialog (overlays)
	Dialog dialogStyles

	// Pills (inline badges)
	Pill pillStyles

	// Tooltip (hover popup)
	Tooltip tooltipStyles

	// Help overlay
	Help helpStyles

	// Table schema
	Table tableStyles

	// Scrollbar
	ScrollThumb lipgloss.Style
	ScrollTrack lipgloss.Style
}

type headerStyles struct {
	Brand     lipgloss.Style // "gomib" in bold primary
	Diagonal  lipgloss.Style // diagonal fill chars
	Bar       lipgloss.Style // header bar background
	Info      lipgloss.Style // section header text in content areas
	Underline lipgloss.Style // subtle underline for section headers
}

type treeStyles struct {
	Pane          lipgloss.Style
	Row           lipgloss.Style
	FocusBorder   lipgloss.Style // thick left border for focused selected row
	UnfocusBorder lipgloss.Style // thick left border for unfocused selected row
	SelectedBg    lipgloss.Style // background-only style for selected row highlight
	Deprecated    lipgloss.Style
	Obsolete      lipgloss.Style
}

type statusStyles struct {
	SuccessIcon lipgloss.Style
	SuccessMsg  lipgloss.Style
	ErrorIcon   lipgloss.Style
	ErrorMsg    lipgloss.Style
	WarnIcon    lipgloss.Style
	WarnMsg     lipgloss.Style
	InfoIcon    lipgloss.Style
	InfoMsg     lipgloss.Style
}

type dialogStyles struct {
	Box     lipgloss.Style // rounded border, primary colored
	Title   lipgloss.Style
	Content lipgloss.Style
}

type pillStyles struct {
	Connected    lipgloss.Style
	Disconnected lipgloss.Style
	Version      lipgloss.Style
}

type tooltipStyles struct {
	Box   lipgloss.Style
	Label lipgloss.Style
	Value lipgloss.Style
}

type helpStyles struct {
	Sep     lipgloss.Style
	Overlay lipgloss.Style
}

type tableStyles struct {
	Sep        lipgloss.Style
	Index      lipgloss.Style
	CurrentCol lipgloss.Style
}

var styles = defaultStyles()

func defaultStyles() appStyles {
	p := palette
	return appStyles{
		Pane: lipgloss.NewStyle().
			Padding(0, 1),
		Label: lipgloss.NewStyle().
			Foreground(p.Muted),
		Value: lipgloss.NewStyle().
			Foreground(p.Fg),
		Prompt: lipgloss.NewStyle().
			Foreground(p.Tertiary),
		StatusText: lipgloss.NewStyle().
			Foreground(p.Faint),
		EmptyText: lipgloss.NewStyle().
			Foreground(p.Faint).
			Faint(true),
		Separator: lipgloss.NewStyle().
			Foreground(p.Border),
		Subtle: lipgloss.NewStyle().
			Foreground(p.Subtle),

		Header: headerStyles{
			Brand: lipgloss.NewStyle().
				Bold(true).
				Foreground(p.Primary),
			Diagonal: lipgloss.NewStyle().
				Foreground(p.Subtle),
			Bar: lipgloss.NewStyle().
				Background(p.BgLighter).
				Foreground(p.Fg),
			Info: lipgloss.NewStyle().
				Bold(true).
				Foreground(p.HeaderBlue),
			Underline: lipgloss.NewStyle().
				Foreground(p.Subtle),
		},

		Breadcrumb: lipgloss.NewStyle().
			Foreground(p.Subtle),
		BreadcrumbSep: lipgloss.NewStyle().
			Foreground(p.Dim),

		Tree: treeStyles{
			Pane: lipgloss.NewStyle().
				Padding(0, 1),
			Row: lipgloss.NewStyle(),
			FocusBorder: lipgloss.NewStyle().
				Foreground(p.Primary),
			UnfocusBorder: lipgloss.NewStyle().
				Foreground(p.Subtle),
			SelectedBg: lipgloss.NewStyle().
				Background(p.BgSubtle),
			Deprecated: lipgloss.NewStyle().Foreground(p.Muted).Faint(true),
			Obsolete:   lipgloss.NewStyle().Foreground(p.Muted).Strikethrough(true),
		},

		Status: statusStyles{
			SuccessIcon: lipgloss.NewStyle().Foreground(p.Green),
			SuccessMsg:  lipgloss.NewStyle().Foreground(p.Green),
			ErrorIcon:   lipgloss.NewStyle().Foreground(p.Fatal),
			ErrorMsg:    lipgloss.NewStyle().Foreground(p.Fatal),
			WarnIcon:    lipgloss.NewStyle().Foreground(p.Yellow),
			WarnMsg:     lipgloss.NewStyle().Foreground(p.Yellow),
			InfoIcon:    lipgloss.NewStyle().Foreground(p.Cyan),
			InfoMsg:     lipgloss.NewStyle().Foreground(p.Faint),
		},

		Dialog: dialogStyles{
			Box: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(p.Primary).
				Background(p.BgLighter).
				Padding(1, 2),
			Title: lipgloss.NewStyle().
				Bold(true).
				Foreground(p.Primary),
			Content: lipgloss.NewStyle().
				Foreground(p.Fg),
		},

		Pill: pillStyles{
			Connected: lipgloss.NewStyle().
				Foreground(p.Connected),
			Disconnected: lipgloss.NewStyle().
				Foreground(p.Disconnected),
			Version: lipgloss.NewStyle().
				Foreground(p.Muted),
		},

		Tooltip: tooltipStyles{
			Box: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(p.Primary).
				Background(p.BgLighter).
				Padding(0, 1),
			Label: lipgloss.NewStyle().
				Foreground(p.Muted),
			Value: lipgloss.NewStyle().
				Foreground(p.Fg),
		},

		Help: helpStyles{
			Sep: lipgloss.NewStyle().
				Foreground(p.Dim),
			Overlay: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(p.Primary).
				Background(p.BgLighter).
				Padding(1, 2),
		},

		Table: tableStyles{
			Sep: lipgloss.NewStyle().
				Foreground(p.Subtle),
			Index: lipgloss.NewStyle().
				Foreground(p.Green),
			CurrentCol: lipgloss.NewStyle().
				Background(p.BgSubtle).
				Foreground(p.Fg),
		},

		ScrollThumb: lipgloss.NewStyle().
			Foreground(p.Primary),
		ScrollTrack: lipgloss.NewStyle().
			Foreground(p.Dim),
	}
}

// Kind-based foreground colors for tree rows.
var kindStyles = func() map[mib.Kind]lipgloss.Style {
	p := palette
	return map[mib.Kind]lipgloss.Style{
		mib.KindUnknown:      lipgloss.NewStyle().Foreground(p.Subtle),
		mib.KindInternal:     lipgloss.NewStyle().Foreground(p.Internal),
		mib.KindNode:         lipgloss.NewStyle().Foreground(p.Node),
		mib.KindScalar:       lipgloss.NewStyle().Foreground(p.Green),
		mib.KindTable:        lipgloss.NewStyle().Foreground(p.Cyan),
		mib.KindRow:          lipgloss.NewStyle().Foreground(p.Teal),
		mib.KindColumn:       lipgloss.NewStyle().Foreground(p.Blue),
		mib.KindNotification: lipgloss.NewStyle().Foreground(p.Yellow),
		mib.KindGroup:        lipgloss.NewStyle().Foreground(p.Pink),
		mib.KindCompliance:   lipgloss.NewStyle().Foreground(p.Purple),
		mib.KindCapability:   lipgloss.NewStyle().Foreground(p.Purple),
	}
}()

var severityStyles = func() map[mib.Severity]lipgloss.Style {
	p := palette
	return map[mib.Severity]lipgloss.Style{
		mib.SeverityFatal:   lipgloss.NewStyle().Foreground(p.Fatal),
		mib.SeveritySevere:  lipgloss.NewStyle().Foreground(p.Severe),
		mib.SeverityError:   lipgloss.NewStyle().Foreground(p.Error),
		mib.SeverityMinor:   lipgloss.NewStyle().Foreground(p.Blue),
		mib.SeverityStyle:   lipgloss.NewStyle().Foreground(p.Mauve),
		mib.SeverityWarning: lipgloss.NewStyle().Foreground(p.Pink),
		mib.SeverityInfo:    lipgloss.NewStyle().Foreground(p.Internal),
	}
}()

func diagSeverityStyle(s mib.Severity) lipgloss.Style {
	if style, ok := severityStyles[s]; ok {
		return style
	}
	return styles.Value
}

func kindStyle(k mib.Kind) lipgloss.Style {
	if s, ok := kindStyles[k]; ok {
		return s
	}
	return styles.Tree.Row
}

// nodeStatus returns the status of a node by checking its object,
// notification, or group attachment. Returns zero value if none.
func nodeStatus(node *mib.Node) mib.Status {
	status, _, _ := nodeEntityProps(node)
	return status
}

// padContentBg pads each line of content to the widest line's visual width
// using background-colored spaces. This fixes background fill inside bordered
// containers where lipgloss's ANSI resets prevent background inheritance.
func padContentBg(content string, bg color.Color) string {
	lines := strings.Split(content, "\n")
	maxW := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxW {
			maxW = w
		}
	}
	if maxW == 0 {
		return content
	}
	bgStyle := lipgloss.NewStyle().Background(bg)
	for i, line := range lines {
		if w := lipgloss.Width(line); w < maxW {
			lines[i] = line + bgStyle.Render(strings.Repeat(" ", maxW-w))
		}
	}
	return strings.Join(lines, "\n")
}
