package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

type statusType int

const (
	statusInfo statusType = iota
	statusSuccess
	statusWarn
	statusError
)

type statusMsg struct {
	typ  statusType
	text string
}

type clearStatusMsg struct{}

// statusModel manages typed status messages with auto-clear.
type statusModel struct {
	current *statusMsg
}

func (s *statusModel) view() string {
	if s.current == nil {
		return ""
	}

	var icon, msg string
	switch s.current.typ {
	case statusSuccess:
		icon = styles.Status.SuccessIcon.Render(IconSuccess)
		msg = styles.Status.SuccessMsg.Render(" " + s.current.text)
	case statusError:
		icon = styles.Status.ErrorIcon.Render(IconError)
		msg = styles.Status.ErrorMsg.Render(" " + s.current.text)
	case statusWarn:
		icon = styles.Status.WarnIcon.Render(IconWarn)
		msg = styles.Status.WarnMsg.Render(" " + s.current.text)
	default:
		icon = styles.Status.InfoIcon.Render(IconArrow)
		msg = styles.Status.InfoMsg.Render(" " + s.current.text)
	}
	return icon + msg
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}
