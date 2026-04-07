package main

import (
	"os/exec"
	"strings"
)

func tmux(args ...string) {
	exec.Command("tmux", args...).Run()
}

func tmuxGetOption(name string) string {
	out, _ := exec.Command("tmux", "show-option", "-gqv", name).Output()
	return strings.TrimSpace(string(out))
}

func optionOr(name, fallback string) string {
	if v := tmuxGetOption(name); v != "" {
		return v
	}
	return fallback
}

func tmuxGetFormat(format string) string {
	out, _ := exec.Command("tmux", "display-message", "-p", format).Output()
	return strings.TrimSpace(string(out))
}

func tmuxDisplayPane(paneID, format string) string {
	out, _ := exec.Command("tmux", "display-message", "-t", paneID, "-p", format).Output()
	return strings.TrimSpace(string(out))
}

// detectPaneState inspects pane content to determine Claude's state.
//
// Indicators:
//   - "esc to interrupt"  → active (Claude is streaming/working)
//   - "Esc to cancel"     → attention (permission/elicitation prompt)
//   - otherwise           → stopped (idle at input prompt)
func detectPaneState(paneContent string) windowState {
	hasActive := false
	hasAttention := false

	for _, line := range strings.Split(paneContent, "\n") {
		// Active indicators (Claude is working)
		if strings.Contains(line, "esc to interrupt") ||
			strings.Contains(line, "stop agents") ||
			strings.Contains(line, "Thinking") ||
			strings.Contains(line, "Flowing") {
			hasActive = true
		}
		// Attention indicators (needs user input)
		if strings.Contains(line, "Esc to cancel") {
			hasAttention = true
		}
	}

	// Attention takes priority — if both present, user input is blocking
	if hasAttention {
		return stateAttention
	}
	if hasActive {
		return stateActive
	}
	return stateStopped
}

// paneFullContent captures the full pane content for multi-line analysis.
func paneFullContent(target string) string {
	out, err := exec.Command("tmux", "capture-pane", "-t", target, "-p").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func tmuxUnsetWindow(target string) {
	tmux(
		"set-window-option", "-t", target, "-u", "window-status-format", ";",
		"set-window-option", "-t", target, "-u", "window-status-current-format", ";",
		"set-window-option", "-t", target, "-u", "@claude-attention", ";",
		"set-window-option", "-t", target, "-u", "@claude-active", ";",
		"set-window-option", "-t", target, "-u", "@claude-stopped",
	)
}

// applyWindowMarker sets tmux visual markers for a single window.
func (d *daemon) applyWindowMarker(target string, state windowState) {
	switch state {
	case stateActive:
		tmux(
			"set-window-option", "-t", target, "window-status-format", d.fmt.active, ";",
			"set-window-option", "-t", target, "window-status-current-format", d.fmt.activeCur, ";",
			"set-window-option", "-t", target, "@claude-active", "1", ";",
			"set-window-option", "-t", target, "-u", "@claude-stopped", ";",
			"set-window-option", "-t", target, "-u", "@claude-attention",
		)
	case stateAttention:
		tmux(
			"set-window-option", "-t", target, "window-status-format", d.fmt.attention, ";",
			"set-window-option", "-t", target, "window-status-current-format", d.fmt.attentionCur, ";",
			"set-window-option", "-t", target, "@claude-attention", "1", ";",
			"set-window-option", "-t", target, "-u", "@claude-active", ";",
			"set-window-option", "-t", target, "-u", "@claude-stopped",
		)
	case stateStopped:
		tmux(
			"set-window-option", "-t", target, "window-status-format", d.fmt.stopped, ";",
			"set-window-option", "-t", target, "window-status-current-format", d.fmt.stoppedCur, ";",
			"set-window-option", "-t", target, "@claude-stopped", "1", ";",
			"set-window-option", "-t", target, "-u", "@claude-active", ";",
			"set-window-option", "-t", target, "-u", "@claude-attention",
		)
	}
}

// applyTmuxMarkers re-applies visual markers for all tracked windows.
func (d *daemon) applyTmuxMarkers() {
	d.mu.Lock()
	targets := make(map[string]windowState, len(d.windows))
	for target, tw := range d.windows {
		if tw.state != stateNone {
			targets[target] = tw.state
		}
	}
	d.mu.Unlock()

	for target, state := range targets {
		d.applyWindowMarker(target, state)
	}
}

func run(name string, args ...string) {
	exec.Command(name, args...).Run()
}

func runEnv(env []string, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Run()
}

func cmdOutput(name string, args ...string) string {
	out, _ := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out))
}

func cmdOutputStdin(stdin string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}
