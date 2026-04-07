package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

func (d *daemon) saveState() {
	d.mu.Lock()
	ps := persistedState{
		Version:       1,
		SavedAt:       time.Now(),
		Windows:       make(map[string]persistedWindow, len(d.windows)),
		Notifications: d.notifications,
	}
	for target, tw := range d.windows {
		if tw.state == stateNone {
			continue
		}
		ps.Windows[target] = persistedWindow{
			State:          tw.state.String(),
			PaneID:         tw.paneID,
			TmuxSocket:     tw.tmuxSocket,
			ActiveSince:    tw.activeSince,
			AttentionSince: tw.attSince,
			TotalActiveMs:  tw.totalActive.Milliseconds(),
			TotalAttMs:     tw.totalAtt.Milliseconds(),
		}
	}
	d.mu.Unlock()

	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		log.Printf("marshal state: %v", err)
		return
	}

	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		log.Printf("write state: %v", err)
		return
	}
	if err := os.Rename(tmp, statePath); err != nil {
		log.Printf("rename state: %v", err)
	}
}

func (d *daemon) loadState() {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return // no state file, fresh start
	}

	var ps persistedState
	if err := json.Unmarshal(data, &ps); err != nil {
		log.Printf("load state: %v", err)
		return
	}

	if ps.Version != 1 {
		log.Printf("unknown state version %d, ignoring", ps.Version)
		return
	}

	d.mu.Lock()
	for target, pw := range ps.Windows {
		state := parseState(pw.State)
		if state == stateNone {
			continue
		}
		tw := &trackedWindow{
			state:       state,
			paneID:      pw.PaneID,
			tmuxSocket:  pw.TmuxSocket,
			activeSince: pw.ActiveSince,
			attSince:    pw.AttentionSince,
			totalActive: time.Duration(pw.TotalActiveMs) * time.Millisecond,
			totalAtt:    time.Duration(pw.TotalAttMs) * time.Millisecond,
		}
		d.windows[target] = tw
	}
	d.notifications = ps.Notifications
	if len(d.notifications) > maxNotifyHistory {
		d.notifications = d.notifications[len(d.notifications)-maxNotifyHistory:]
	}
	d.mu.Unlock()

	log.Printf("loaded state: %d windows, %d notifications", len(ps.Windows), len(ps.Notifications))
}

func (d *daemon) buildStatusResponse() statusResponse {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	resp := statusResponse{
		OK:          true,
		Uptime:      now.Sub(d.startTime).Truncate(time.Second).String(),
		WindowCount: len(d.windows),
		Windows:     make(map[string]windowStatus, len(d.windows)),
	}

	for target, tw := range d.windows {
		if tw.state == stateNone {
			continue
		}
		ws := windowStatus{
			State:             tw.state.String(),
			PaneID:            tw.paneID,
			ActiveDuration:    tw.currentActiveDur(now).Truncate(time.Second).String(),
			AttentionDuration: tw.currentAttDur(now).Truncate(time.Second).String(),
			ActiveSince:       tw.activeSince,
			AttentionSince:    tw.attSince,
		}
		resp.Windows[target] = ws

		switch tw.state {
		case stateActive:
			resp.Counts.Active++
		case stateAttention:
			resp.Counts.Attention++
		case stateStopped:
			resp.Counts.Stopped++
		}
	}

	// Return last 50 notifications in the status response
	if len(d.notifications) > 50 {
		resp.Notifications = d.notifications[len(d.notifications)-50:]
	} else {
		resp.Notifications = d.notifications
	}

	return resp
}
