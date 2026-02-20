package main

import (
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/golangsnmp/gomib/mib"
)

const tooltipDelay = 300 * time.Millisecond

type showTooltipMsg struct {
	seq uint64 // matches tooltipModel.seq to discard stale timer firings
}

// tooltipModel manages hover tooltip popups for tree nodes.
type tooltipModel struct {
	node    *mib.Node
	x, y    int // screen position where tooltip should appear
	visible bool
	pending bool   // waiting for delay timer
	seq     uint64 // incremented on each startDelay call
}

func (t *tooltipModel) hide() {
	t.visible = false
	t.pending = false
	t.node = nil
}

func (t *tooltipModel) startDelay(node *mib.Node, x, y int) tea.Cmd {
	t.node = node
	t.x = x
	t.y = y
	t.visible = false
	t.pending = true
	t.seq++
	seq := t.seq
	return tea.Tick(tooltipDelay, func(time.Time) tea.Msg {
		return showTooltipMsg{seq: seq}
	})
}

func (t *tooltipModel) show(seq uint64) {
	if t.pending && t.node != nil && t.seq == seq {
		t.visible = true
		t.pending = false
	}
}

func (t *tooltipModel) draw(canvas uv.ScreenBuffer, area image.Rectangle) {
	if !t.visible || t.node == nil {
		return
	}

	content := t.buildContent()
	if content == "" {
		return
	}

	content = padContentBg(content, palette.BgLighter)
	box := styles.Tooltip.Box.Render(content)
	w := lipgloss.Width(box)
	h := lipgloss.Height(box)

	// Position near the mouse, clamped to screen
	x := t.x + 2
	y := t.y

	if x+w > area.Max.X {
		x = area.Max.X - w
	}
	if x < area.Min.X {
		x = area.Min.X
	}
	if y+h > area.Max.Y {
		y = area.Max.Y - h
	}
	if y < area.Min.Y {
		y = area.Min.Y
	}

	rect := image.Rect(x, y, x+w, y+h)
	uv.NewStyledString(box).Draw(canvas, rect)
}

func (t *tooltipModel) buildContent() string {
	if t.node == nil {
		return ""
	}

	var b strings.Builder
	node := t.node
	bg := palette.BgLighter

	// Name with kind dot
	dot := kindStyle(node.Kind()).Background(bg).Render(IconPending)
	space := lipgloss.NewStyle().Background(bg).Render(" ")
	name := styles.Value.Background(bg).Render(node.Name())
	b.WriteString(dot + space + name)
	b.WriteByte('\n')

	// OID
	if oid := node.OID(); oid != nil {
		b.WriteString(styles.Label.Background(bg).Render(oid.String()))
		b.WriteByte('\n')
	}

	// Type + kind
	var typeLine string
	if obj := node.Object(); obj != nil {
		if typ := obj.Type(); typ != nil {
			name := typ.Name()
			if name == "" {
				name = typ.Base().String()
			}
			typeLine = name
		}
	}
	kindStr := node.Kind().String()
	if typeLine != "" {
		b.WriteString(styles.Tooltip.Label.Background(bg).Render(typeLine + " (" + kindStr + ")"))
	} else {
		b.WriteString(styles.Tooltip.Label.Background(bg).Render(kindStr))
	}
	b.WriteByte('\n')

	// Description, word-wrapped to tooltip width
	desc := ""
	if obj := node.Object(); obj != nil && obj.Description() != "" {
		desc = normalizeDescription(obj.Description())
	} else if notif := node.Notification(); notif != nil && notif.Description() != "" {
		desc = normalizeDescription(notif.Description())
	}
	if desc != "" {
		const tooltipWidth = 50
		if len(desc) > 250 {
			desc = desc[:247] + "..."
		}
		wrapped := wrapText(desc, tooltipWidth, "")
		b.WriteString(styles.Tooltip.Value.Background(bg).Render(wrapped))
	}

	return b.String()
}
