package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderScrollbar produces a vertical scrollbar column of the given height.
// It uses ┃ for the thumb and │ for the track. Returns empty string if
// all content is visible (totalLines <= visibleLines).
func renderScrollbar(height, totalLines, visibleLines, offset int) string {
	if height <= 0 || totalLines <= visibleLines {
		return ""
	}

	// Thumb size: proportional to visible/total, minimum 1
	thumbH := height * visibleLines / totalLines
	if thumbH < 1 {
		thumbH = 1
	}

	// Thumb position
	scrollable := totalLines - visibleLines
	if scrollable < 1 {
		scrollable = 1
	}
	thumbPos := (height - thumbH) * offset / scrollable
	if thumbPos > height-thumbH {
		thumbPos = height - thumbH
	}
	if thumbPos < 0 {
		thumbPos = 0
	}

	var b strings.Builder
	for i := range height {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i >= thumbPos && i < thumbPos+thumbH {
			b.WriteString(styles.ScrollThumb.Render(ScrollThumbChar))
		} else {
			b.WriteString(styles.ScrollTrack.Render(ScrollTrackChar))
		}
	}
	return b.String()
}

// joinScrollbar places a scrollbar column to the right of the content.
func joinScrollbar(content, scrollbar string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, content, scrollbar)
}

// attachScrollbar renders a scrollbar and joins it to the content if needed.
// Returns content unchanged when all rows are visible.
func attachScrollbar(content string, height, total, visible, offset int) string {
	sb := renderScrollbar(height, total, visible, offset)
	if sb != "" {
		return joinScrollbar(content, sb)
	}
	return content
}
