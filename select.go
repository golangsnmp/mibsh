package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// selectModel is a minimal inline cycle selector for small fixed option sets.
// Left/right arrows cycle through options. No free-text input.
type selectModel struct {
	options  []string
	selected int
	focused  bool
}

func newSelect(options []string) selectModel {
	return selectModel{options: options}
}

func (s *selectModel) Focus() tea.Cmd {
	s.focused = true
	return nil
}

func (s *selectModel) Blur() {
	s.focused = false
}

func (s *selectModel) Focused() bool {
	return s.focused
}

func (s *selectModel) Value() string {
	if len(s.options) == 0 {
		return ""
	}
	return s.options[s.selected]
}

// SetValue sets the selected option by string (case-insensitive match).
// If no match is found, the selection is unchanged.
func (s *selectModel) SetValue(v string) {
	for i, opt := range s.options {
		if strings.EqualFold(opt, v) {
			s.selected = i
			return
		}
	}
}

func (s *selectModel) Selected() int {
	return s.selected
}

func (s *selectModel) SetSelected(i int) {
	if i >= 0 && i < len(s.options) {
		s.selected = i
	}
}

func (s *selectModel) Update(msg tea.Msg) (selectModel, tea.Cmd) {
	if !s.focused {
		return *s, nil
	}
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "left":
			s.selected--
			if s.selected < 0 {
				s.selected = len(s.options) - 1
			}
		case "right":
			s.selected++
			if s.selected >= len(s.options) {
				s.selected = 0
			}
		}
	}
	return *s, nil
}

func (s *selectModel) View() string {
	val := s.Value()
	if !s.focused {
		return val
	}
	arrow := lipgloss.NewStyle().Foreground(palette.Muted)
	return arrow.Render("\u25c2") + " " + val + " " + arrow.Render("\u25b8")
}
