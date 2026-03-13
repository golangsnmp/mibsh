package snmp

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gosnmp/gosnmp"
)

// WatchPollMsg carries the results of a watch poll.
type WatchPollMsg struct {
	Seq  uint64 // poll sequence number for staleness detection
	PDUs []gosnmp.SnmpPDU
	Err  error
}

// WatchTickMsg signals that the next poll should start.
type WatchTickMsg struct {
	Seq uint64 // sequence number, ignored if stale
}

// WatchPollCmd performs a synchronous walk in a goroutine and returns all
// PDUs at once via WatchPollMsg. Unlike the streaming walk, this collects
// everything before sending a single message.
func WatchPollCmd(sess *Session, rootOID string, seq uint64) tea.Cmd {
	return func() tea.Msg {
		if !sess.IsConnected() {
			return WatchPollMsg{Seq: seq, Err: errors.New("not connected")}
		}

		var pdus []gosnmp.SnmpPDU
		walkFn := func(pdu gosnmp.SnmpPDU) error {
			pdus = append(pdus, pdu)
			return nil
		}

		err := doWalk(sess.client, rootOID, walkFn)
		return WatchPollMsg{Seq: seq, PDUs: pdus, Err: err}
	}
}

// WatchTickCmd returns a command that sends a WatchTickMsg after the given
// interval. The seq parameter is checked against the current watch sequence
// to discard stale ticks.
func WatchTickCmd(interval time.Duration, seq uint64) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return WatchTickMsg{Seq: seq}
	})
}
