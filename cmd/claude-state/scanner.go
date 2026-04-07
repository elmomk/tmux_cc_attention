package main

import (
	"log"
	"os/exec"
	"strings"
	"time"
)

func (d *daemon) staleCleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.cleanupStale()
		case <-d.done:
			return
		}
	}
}

func (d *daemon) cleanupStale() {
	out, err := exec.Command("tmux", "list-panes", "-a",
		"-F", "#{session_name}:#{window_index}|#{pane_current_command}|#{pane_id}").Output()
	if err != nil {
		return
	}

	type paneInfo struct {
		paneID string // e.g. "%5" — used to capture the correct pane in split windows
	}
	claudeWindows := make(map[string]paneInfo)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		lower := strings.ToLower(parts[1])
		if lower == "claude" || lower == "claude-code" {
			claudeWindows[parts[0]] = paneInfo{paneID: parts[2]}
		}
	}

	now := time.Now()
	d.mu.Lock()

	// Remove stale: tracked but Claude no longer running
	var stale []string
	for target, tw := range d.windows {
		if tw.state != stateNone && claudeWindows[target].paneID == "" {
			stale = append(stale, target)
			tw.finalizeInterval(now)
			tw.state = stateNone
		}
	}

	// Discover untracked Claude windows AND re-check stopped ones
	// (a stopped window may have started working again without hooks firing).
	type discovery struct {
		target   string
		paneID   string
		newState windowState
	}
	// Re-check ALL tracked windows (not just stopped/none). An active window
	// may be showing a permission prompt if the Notification hook was delayed.
	var discovered []discovery
	for target, info := range claudeWindows {
		tw := d.windows[target]
		if tw == nil {
			if len(d.windows) >= maxWindows {
				continue
			}
			tw = &trackedWindow{}
			d.windows[target] = tw
		}
		discovered = append(discovered, discovery{target: target, paneID: info.paneID})
	}
	d.mu.Unlock()

	// Detect state from pane content (target the specific Claude pane, not active pane)
	for i, disc := range discovered {
		content := paneFullContent(disc.paneID)
		detected := detectPaneState(content)
		discovered[i].newState = detected

		d.mu.Lock()
		tw := d.windows[disc.target]
		if tw != nil && tw.state != detected {
			// Don't downgrade active→attention via scan — hooks are authoritative
			// for that transition and fire instantly. The scan could catch a
			// permission prompt that's about to be auto-approved, causing a red blink.
			// Only upgrade: none/stopped→active/attention, or active→stopped.
			skip := tw.state == stateActive && detected == stateAttention
			if !skip {
				tw.finalizeInterval(now)
				tw.state = detected
				switch detected {
				case stateActive:
					tw.activeSince = &now
				case stateAttention:
					tw.attSince = &now
				}
			}
		}
		d.mu.Unlock()
	}

	for _, target := range stale {
		log.Printf("stale: %s", target)
		tmuxUnsetWindow(target)
	}

	for _, disc := range discovered {
		log.Printf("discovered: %s (%s)", disc.target, disc.newState)
		d.applyWindowMarker(disc.target, disc.newState)
	}

	if len(stale) > 0 || len(discovered) > 0 {
		d.refreshCounts()
		d.scheduleSave()
	}
}
