package main

import (
	"image"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/ultraviolet/layout"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlayFilterHelp
	overlayConnect
)

// overlayModel manages modal overlays (help dialog, connect dialog).
type overlayModel struct {
	kind overlayKind
}

func (o *overlayModel) isDialog() bool {
	return o.kind == overlayHelp || o.kind == overlayFilterHelp || o.kind == overlayConnect
}

// drawCentered draws content in a centered dialog box on the canvas.
func (o *overlayModel) drawCentered(canvas uv.ScreenBuffer, area image.Rectangle, content string) {
	// Fill background with dim overlay
	bgFill := styles.Header.Bar.Width(area.Dx()).Height(area.Dy()).Render("")
	uv.NewStyledString(bgFill).Draw(canvas, area)

	box := styles.Dialog.Box.Render(content)
	w := lipgloss.Width(box)
	h := lipgloss.Height(box)
	rect := layout.CenterRect(area, w, h)
	uv.NewStyledString(box).Draw(canvas, rect)
}
