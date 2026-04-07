package main

import (
	"sync"
	"time"
)

type request struct {
	Type       string `json:"type"`
	Session    string `json:"session"`
	Window     string `json:"window"`
	PaneID     string `json:"pane_id"`
	PaneIdx    string `json:"pane_idx"`
	Workspace  string `json:"workspace"`
	WindowAddr string `json:"window_addr"`
	TmuxSocket string `json:"tmux_socket"`
}

func (r *request) target() string {
	return r.Session + ":" + r.Window
}

func (r *request) validate() bool {
	if !validTypes[r.Type] {
		return false
	}
	if r.Type == "status" {
		return true // no target fields needed
	}
	if r.Session == "" || !validName.MatchString(r.Session) {
		return false
	}
	if r.Window == "" || !validDigits.MatchString(r.Window) {
		return false
	}
	if r.PaneID != "" && !validPaneID.MatchString(r.PaneID) {
		return false
	}
	if r.TmuxSocket != "" && !validTmuxSocket.MatchString(r.TmuxSocket) {
		return false
	}
	return true
}

// ── Window state ──

type windowState int

const (
	stateNone windowState = iota
	stateActive
	stateAttention
	stateStopped
)

func (s windowState) String() string {
	switch s {
	case stateActive:
		return "active"
	case stateAttention:
		return "attention"
	case stateStopped:
		return "stopped"
	default:
		return "none"
	}
}

func parseState(s string) windowState {
	switch s {
	case "active":
		return stateActive
	case "attention":
		return stateAttention
	case "stopped":
		return stateStopped
	default:
		return stateNone
	}
}

// ── Tracked window ──

type trackedWindow struct {
	state       windowState
	paneID      string
	tmuxSocket  string
	activeSince *time.Time    // when current active interval started
	attSince    *time.Time    // when current attention interval started
	totalActive time.Duration // accumulated active time
	totalAtt    time.Duration // accumulated attention time
}

// finalizeInterval closes any open timer interval. Caller must hold d.mu.
func (tw *trackedWindow) finalizeInterval(now time.Time) {
	if tw.activeSince != nil {
		tw.totalActive += now.Sub(*tw.activeSince)
		tw.activeSince = nil
	}
	if tw.attSince != nil {
		tw.totalAtt += now.Sub(*tw.attSince)
		tw.attSince = nil
	}
}

func (tw *trackedWindow) currentActiveDur(now time.Time) time.Duration {
	d := tw.totalActive
	if tw.activeSince != nil {
		d += now.Sub(*tw.activeSince)
	}
	return d
}

func (tw *trackedWindow) currentAttDur(now time.Time) time.Duration {
	d := tw.totalAtt
	if tw.attSince != nil {
		d += now.Sub(*tw.attSince)
	}
	return d
}

// ── Notification record ──

type notifyRecord struct {
	Time        time.Time  `json:"time"`
	Target      string     `json:"target"`
	Session     string     `json:"session"`
	Window      string     `json:"window"`
	Responded   bool       `json:"responded"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
}

// ── Persistence types ──

type persistedState struct {
	Version       int                        `json:"version"`
	SavedAt       time.Time                  `json:"saved_at"`
	Windows       map[string]persistedWindow `json:"windows"`
	Notifications []notifyRecord             `json:"notifications"`
}

type persistedWindow struct {
	State          string     `json:"state"`
	PaneID         string     `json:"pane_id,omitempty"`
	TmuxSocket     string     `json:"tmux_socket,omitempty"`
	ActiveSince    *time.Time `json:"active_since,omitempty"`
	AttentionSince *time.Time `json:"attention_since,omitempty"`
	TotalActiveMs  int64      `json:"total_active_ms"`
	TotalAttMs     int64      `json:"total_attention_ms"`
}

// ── Status API types ──

type statusResponse struct {
	OK            bool                    `json:"ok"`
	Uptime        string                  `json:"uptime"`
	WindowCount   int                     `json:"window_count"`
	Windows       map[string]windowStatus `json:"windows"`
	Notifications []notifyRecord          `json:"notifications"`
	Counts        statusCounts            `json:"counts"`
}

type windowStatus struct {
	State             string     `json:"state"`
	PaneID            string     `json:"pane_id,omitempty"`
	ActiveDuration    string     `json:"active_duration"`
	AttentionDuration string     `json:"attention_duration"`
	ActiveSince       *time.Time `json:"active_since,omitempty"`
	AttentionSince    *time.Time `json:"attention_since,omitempty"`
}

type statusCounts struct {
	Active    int `json:"active"`
	Attention int `json:"attention"`
	Stopped   int `json:"stopped"`
}

// ── Formats ──

type formats struct {
	once                          sync.Once
	active, activeCur             string
	attention, attentionCur       string
	stopped, stoppedCur           string
	attColor, actColor, stopColor string
}

func (f *formats) load() {
	f.once.Do(func() {
		f.active = tmuxGetOption("@claude-fmt-active")
		f.activeCur = tmuxGetOption("@claude-fmt-active-cur")
		f.attention = tmuxGetOption("@claude-fmt-attention")
		f.attentionCur = tmuxGetOption("@claude-fmt-attention-cur")
		f.stopped = tmuxGetOption("@claude-fmt-stopped")
		f.stoppedCur = tmuxGetOption("@claude-fmt-stopped-cur")
		f.attColor = optionOr("@claude-attention-color", "#c4746e")
		f.actColor = optionOr("@claude-active-color", "#87a987")
		f.stopColor = optionOr("@claude-stopped-color", "#8ba4b0")
	})
}
