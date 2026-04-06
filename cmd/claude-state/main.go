// claude-state: centralized state daemon for the tmux Claude Code attention plugin.
//
// Usage:
//
//	claude-state serve          Start the daemon (auto-exits after 10min idle)
//	claude-state active         Mark current window as active (green)
//	claude-state attention      Mark current window as needs-input (red)
//	claude-state stopped        Mark current window as idle/done (blue)
//	claude-state clear          Clear attention/stopped on current window
//	claude-state notify         Send desktop notification + mark attention
//	claude-state status         Query daemon state as JSON
//
// Client commands read $TMUX and $TMUX_PANE, resolve the session:window target,
// and send a single message over the unix socket. If the daemon isn't running,
// it's started automatically. Stdin is drained so Claude Code hooks don't block.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	replaceID        = "99999"
	notifyTimeout    = "30000"
	idleTimeout      = 10 * time.Minute
	readTimeout      = 2 * time.Second
	donePopupSec     = 5
	maxWindows       = 500
	maxConnections   = 50
	maxNotifyHistory = 200
	saveDebounce     = time.Second
)

var (
	socketPath = runtimePath("claude-state.sock")
	pidFile    = runtimePath("claude-state.pid")
	statePath  = runtimePath("claude-state.json")

	// SECURITY: validName must never allow single-quote (') — PowerShell injection prevention.
	validName       = regexp.MustCompile(`^[a-zA-Z0-9_./-]{1,128}$`)
	validDigits     = regexp.MustCompile(`^[0-9]{1,10}$`)
	validPaneID     = regexp.MustCompile(`^%[0-9]{1,10}$`)
	validTmuxSocket = regexp.MustCompile(`^/[a-zA-Z0-9/_.-]+,\d+,\d+$`)
	validTypes      = map[string]bool{
		"active": true, "attention": true, "stopped": true,
		"clear": true, "notify": true, "status": true,
	}
)

func runtimePath(name string) string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir + "/claude-state/" + name
	}
	return fmt.Sprintf("/tmp/claude-state-%d/%s", os.Getuid(), name)
}

func ensureRuntimeDir() {
	os.MkdirAll(filepath.Dir(socketPath), 0700)
}

// ── Client mode ──

func clientMain(msgType string) {
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := os.Stdin.Read(buf); err != nil {
				return
			}
		}
	}()

	tmuxSocket := os.Getenv("TMUX")
	paneID := os.Getenv("TMUX_PANE")
	if tmuxSocket == "" {
		os.Exit(0)
	}
	if paneID == "" && msgType != "clear" {
		os.Exit(0)
	}

	var info string
	if msgType == "clear" {
		info = tmuxGetFormat("#{session_name}|#{window_index}|#{pane_index}")
	} else {
		info = tmuxDisplayPane(paneID, "#{session_name}|#{window_index}|#{pane_index}")
	}
	parts := strings.SplitN(info, "|", 3)
	if len(parts) < 2 {
		os.Exit(1)
	}

	req := request{
		Type:       msgType,
		Session:    parts[0],
		Window:     parts[1],
		TmuxSocket: tmuxSocket,
		PaneID:     paneID,
	}
	if len(parts) >= 3 {
		req.PaneIdx = parts[2]
	}

	if msgType == "notify" {
		if skip := resolveHyprland(&req, paneID); skip {
			os.Exit(0)
		}
	}

	ensureDaemon()
	sendRequest(&req)
}

func statusMain() {
	ensureDaemon()
	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	data, _ := json.Marshal(&request{Type: "status"})
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	conn.Write(data)
	conn.(*net.UnixConn).CloseWrite()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	io.Copy(os.Stdout, conn)
}

func resolveHyprland(req *request, paneID string) bool {
	if _, err := exec.LookPath("hyprctl"); err != nil {
		return false
	}

	termPID := tmuxDisplayPane(paneID, "#{client_pid}")
	if termPID == "" {
		return false
	}

	chain := termPID
	for range 5 {
		parent := strings.TrimSpace(cmdOutput("ps", "-o", "ppid=", "-p", chain))
		if parent == "" || parent == "1" {
			break
		}
		clients := cmdOutput("hyprctl", "clients", "-j")
		if clients == "" {
			break
		}
		match := cmdOutputStdin(clients, "jq", "-r", "--argjson", "pid", parent,
			`.[] | select(.pid == $pid) | "\(.workspace.id) \(.address)"`)
		if match != "" {
			fields := strings.Fields(match)
			if len(fields) >= 2 {
				req.Workspace = fields[0]
				req.WindowAddr = fields[1]
			}
			break
		}
		chain = parent
	}

	if req.WindowAddr != "" {
		activeJSON := cmdOutput("hyprctl", "activewindow", "-j")
		if activeJSON != "" {
			var win struct {
				Address string `json:"address"`
			}
			if json.Unmarshal([]byte(activeJSON), &win) == nil {
				activeTmuxWin := tmuxGetFormat("#{window_index}")
				activeTmuxPane := tmuxGetFormat("#{pane_id}")
				if win.Address == req.WindowAddr && activeTmuxWin == req.Window && activeTmuxPane == req.PaneID {
					return true
				}
			}
		}
	}

	return false
}

func ensureDaemon() {
	ensureRuntimeDir()
	if _, err := os.Stat(socketPath); err == nil {
		return
	}

	self, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(self, "serve")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Start()

	for range 5 {
		time.Sleep(50 * time.Millisecond)
		if _, err := os.Stat(socketPath); err == nil {
			return
		}
	}
}

func sendRequest(req *request) {
	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		log.Printf("dial daemon: %v", err)
		return
	}
	defer conn.Close()

	data, _ := json.Marshal(req)
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write(data); err != nil {
		log.Printf("write: %v", err)
		return
	}
	conn.(*net.UnixConn).CloseWrite()

	conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 256)
	conn.Read(buf)
}

// ── Shared types ──

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

// ── Daemon types ──

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
	State          string         `json:"state"`
	PaneID         string         `json:"pane_id,omitempty"`
	TmuxSocket     string         `json:"tmux_socket,omitempty"`
	ActiveSince    *time.Time     `json:"active_since,omitempty"`
	AttentionSince *time.Time     `json:"attention_since,omitempty"`
	TotalActiveMs  int64          `json:"total_active_ms"`
	TotalAttMs     int64          `json:"total_attention_ms"`
}

// ── Status API types ──

type statusResponse struct {
	OK            bool                     `json:"ok"`
	Uptime        string                   `json:"uptime"`
	WindowCount   int                      `json:"window_count"`
	Windows       map[string]windowStatus  `json:"windows"`
	Notifications []notifyRecord           `json:"notifications"`
	Counts        statusCounts             `json:"counts"`
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
	once                            sync.Once
	active, activeCur               string
	attention, attentionCur         string
	stopped, stoppedCur             string
	attColor, actColor, stopColor   string
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

func optionOr(name, fallback string) string {
	if v := tmuxGetOption(name); v != "" {
		return v
	}
	return fallback
}

// ── Daemon ──

type daemon struct {
	mu            sync.Mutex
	windows       map[string]*trackedWindow
	notifications []notifyRecord
	fmt           formats
	notifyCmd     *exec.Cmd
	doneTimer     *time.Timer
	saveTimer     *time.Timer
	startTime     time.Time
	lastActivity  time.Time
	listener      net.Listener
	done          chan struct{}
}

func (d *daemon) getOrCreate(target string) *trackedWindow {
	tw, ok := d.windows[target]
	if !ok {
		if len(d.windows) >= maxWindows {
			return nil
		}
		tw = &trackedWindow{}
		d.windows[target] = tw
	}
	return tw
}

func (d *daemon) handleConn(conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(readTimeout))

	data, err := io.ReadAll(io.LimitReader(conn, 4096))
	if err != nil || len(data) == 0 {
		conn.Write([]byte("{\"ok\":false}\n"))
		return
	}

	var req request
	if err := json.Unmarshal(data, &req); err != nil {
		conn.Write([]byte("{\"ok\":false}\n"))
		return
	}

	if !req.validate() {
		conn.Write([]byte("{\"ok\":false}\n"))
		return
	}

	d.mu.Lock()
	d.lastActivity = time.Now()
	d.mu.Unlock()

	d.fmt.load()

	// Status returns a larger response
	if req.Type == "status" {
		resp := d.buildStatusResponse()
		out, _ := json.MarshalIndent(resp, "", "  ")
		conn.Write(out)
		conn.Write([]byte("\n"))
		return
	}

	d.dispatch(&req)
	conn.Write([]byte("{\"ok\":true}\n"))
}

// ── State transitions ──

func (d *daemon) dispatch(req *request) {
	switch req.Type {
	case "active":
		d.setActive(req)
	case "attention":
		d.setAttention(req)
	case "stopped":
		d.setStopped(req)
	case "clear":
		d.clearWindow(req)
	case "notify":
		d.setAttention(req)
		go d.sendDesktopNotification(req)
	}
}

func (d *daemon) setActive(req *request) {
	target := req.target()
	now := time.Now()

	d.mu.Lock()
	tw := d.getOrCreate(target)
	if tw == nil || tw.state == stateActive {
		d.mu.Unlock()
		return
	}

	tw.finalizeInterval(now)
	tw.state = stateActive
	tw.activeSince = &now
	tw.paneID = req.PaneID
	tw.tmuxSocket = req.TmuxSocket
	d.mu.Unlock()

	tmux(
		"set-window-option", "-t", target, "window-status-format", d.fmt.active, ";",
		"set-window-option", "-t", target, "window-status-current-format", d.fmt.activeCur, ";",
		"set-window-option", "-t", target, "@claude-active", "1", ";",
		"set-window-option", "-t", target, "-u", "@claude-stopped", ";",
		"set-window-option", "-t", target, "-u", "@claude-attention",
	)

	// Race guard: attention may have been set concurrently
	d.mu.Lock()
	if tw.state == stateAttention {
		d.mu.Unlock()
		tmux(
			"set-window-option", "-t", target, "window-status-format", d.fmt.attention, ";",
			"set-window-option", "-t", target, "window-status-current-format", d.fmt.attentionCur, ";",
			"set-window-option", "-t", target, "-u", "@claude-active",
		)
	} else {
		d.mu.Unlock()
	}

	d.refreshCounts()
	d.scheduleSave()
}

func (d *daemon) setAttention(req *request) {
	target := req.target()
	now := time.Now()

	d.mu.Lock()
	tw := d.getOrCreate(target)
	if tw == nil {
		d.mu.Unlock()
		return
	}

	tw.finalizeInterval(now)
	tw.state = stateAttention
	tw.attSince = &now
	tw.paneID = req.PaneID
	tw.tmuxSocket = req.TmuxSocket
	d.mu.Unlock()

	tmux(
		"set-window-option", "-t", target, "window-status-format", d.fmt.attention, ";",
		"set-window-option", "-t", target, "window-status-current-format", d.fmt.attentionCur, ";",
		"set-window-option", "-t", target, "@claude-attention", "1", ";",
		"set-window-option", "-t", target, "-u", "@claude-active", ";",
		"set-window-option", "-t", target, "-u", "@claude-stopped",
	)

	if tmuxGetOption("@claude-attention-bell") == "on" {
		tmux("run-shell", "-t", target, `printf "\a"`)
	}

	d.refreshCounts()
	d.scheduleSave()
}

func (d *daemon) setStopped(req *request) {
	target := req.target()
	now := time.Now()

	d.mu.Lock()
	tw := d.getOrCreate(target)
	if tw == nil {
		d.mu.Unlock()
		return
	}
	wasActive := tw.state == stateActive

	tw.finalizeInterval(now)
	tw.state = stateStopped
	d.mu.Unlock()

	tmux(
		"set-window-option", "-t", target, "window-status-format", d.fmt.stopped, ";",
		"set-window-option", "-t", target, "window-status-current-format", d.fmt.stoppedCur, ";",
		"set-window-option", "-t", target, "@claude-stopped", "1", ";",
		"set-window-option", "-t", target, "-u", "@claude-active", ";",
		"set-window-option", "-t", target, "-u", "@claude-attention",
	)

	if wasActive && tmuxGetOption("@claude-done-popup") == "on" {
		msg := fmt.Sprintf("#[fg=%s]done:%s ", d.fmt.stopColor, target)
		tmux("set-option", "-g", "@claude-done-msg", msg)

		d.mu.Lock()
		if d.doneTimer != nil {
			d.doneTimer.Stop()
		}
		d.doneTimer = time.AfterFunc(donePopupSec*time.Second, func() {
			tmux("set-option", "-gu", "@claude-done-msg")
		})
		d.mu.Unlock()
	}

	if timeoutStr := tmuxGetOption("@claude-stopped-timeout"); timeoutStr != "" {
		if timeout, err := strconv.Atoi(timeoutStr); err == nil && timeout > 0 {
			go func() {
				time.Sleep(time.Duration(timeout) * time.Second)
				d.mu.Lock()
				current := d.windows[target]
				if current != nil && current.state == stateStopped {
					current.state = stateNone
					d.mu.Unlock()
					tmuxUnsetWindow(target)
					d.refreshCounts()
					d.scheduleSave()
				} else {
					d.mu.Unlock()
				}
			}()
		}
	}

	d.refreshCounts()
	d.scheduleSave()
}

func (d *daemon) clearWindow(req *request) {
	target := req.target()
	now := time.Now()

	d.mu.Lock()
	tw := d.windows[target]
	if tw == nil || (tw.state != stateAttention && tw.state != stateStopped) {
		d.mu.Unlock()
		return
	}

	tw.finalizeInterval(now)
	// Stay as stopped (not stateNone) — Claude is still running, user just focused the window.
	// This keeps the window in the status API so the popup always shows it.
	tw.state = stateStopped

	// Mark most recent unresponded notification for this target as responded
	for i := len(d.notifications) - 1; i >= 0; i-- {
		if d.notifications[i].Target == target && !d.notifications[i].Responded {
			d.notifications[i].Responded = true
			d.notifications[i].RespondedAt = &now
			break
		}
	}
	d.mu.Unlock()

	tmux(
		"set-window-option", "-t", target, "-u", "window-status-format", ";",
		"set-window-option", "-t", target, "-u", "window-status-current-format", ";",
		"set-window-option", "-t", target, "-u", "@claude-attention", ";",
		"set-window-option", "-t", target, "-u", "@claude-stopped",
	)

	d.refreshCounts()
	d.scheduleSave()
}

// ── Cross-session counts ──

func (d *daemon) refreshCounts() {
	currentSession := tmuxGetFormat("#{session_name}")

	d.mu.Lock()
	att, act, stop := 0, 0, 0
	for target, tw := range d.windows {
		if tw.state == stateNone {
			continue
		}
		if idx := strings.Index(target, ":"); idx >= 0 && target[:idx] == currentSession {
			continue
		}
		switch tw.state {
		case stateAttention:
			att++
		case stateActive:
			act++
		case stateStopped:
			stop++
		}
	}
	d.mu.Unlock()

	var b strings.Builder
	if att > 0 {
		fmt.Fprintf(&b, "#[fg=%s]!%d ", d.fmt.attColor, att)
	}
	if act > 0 {
		fmt.Fprintf(&b, "#[fg=%s]*%d ", d.fmt.actColor, act)
	}
	if stop > 0 {
		fmt.Fprintf(&b, "#[fg=%s]-%d ", d.fmt.stopColor, stop)
	}
	tmux("set-option", "-g", "@claude-cross-counts", b.String())
}

// ── Stale cleanup ──

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
			tw.finalizeInterval(now)
			tw.state = detected
			switch detected {
			case stateActive:
				tw.activeSince = &now
			case stateAttention:
				tw.attSince = &now
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

// ── Desktop notifications ──

func (d *daemon) sendDesktopNotification(req *request) {
	// Record notification
	now := time.Now()
	d.mu.Lock()
	d.notifications = append(d.notifications, notifyRecord{
		Time:    now,
		Target:  req.target(),
		Session: req.Session,
		Window:  req.Window,
	})
	if len(d.notifications) > maxNotifyHistory {
		d.notifications = d.notifications[len(d.notifications)-maxNotifyHistory:]
	}
	d.mu.Unlock()

	d.killNotify()

	body := fmt.Sprintf("Session: %s, Window: %s, Pane: %s", req.Session, req.Window, req.PaneIdx)
	platform := tmuxGetOption("@claude-notify-platform")

	switch platform {
	case "linux":
		d.notifyLinux(req, body)
	case "windows":
		d.notifyWindows(body)
	default:
		if _, err := exec.LookPath("notify-send"); err == nil {
			d.notifyLinux(req, body)
		} else if _, err := exec.LookPath("powershell.exe"); err == nil {
			d.notifyWindows(body)
		}
	}
}

func (d *daemon) runNotifyCmd(cmd *exec.Cmd) ([]byte, error) {
	d.mu.Lock()
	d.notifyCmd = cmd
	d.mu.Unlock()

	out, err := cmd.Output()

	d.mu.Lock()
	if d.notifyCmd == cmd {
		d.notifyCmd = nil
	}
	d.mu.Unlock()

	return out, err
}

func (d *daemon) notifyLinux(req *request, body string) {
	out, err := d.runNotifyCmd(exec.Command("notify-send",
		"-u", "normal",
		"-a", "Claude Code",
		"-A", "focus=Focus",
		"-r", replaceID,
		"-t", notifyTimeout,
		"Claude Code needs attention",
		body,
	))

	if err == nil && strings.TrimSpace(string(out)) == "focus" {
		d.focusWindow(req)
	}
}

func (d *daemon) notifyWindows(body string) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", "-")
	cmd.Stdin = strings.NewReader(fmt.Sprintf(`
$title = 'Claude Code needs attention'
$body = '%s'
$ErrorActionPreference = 'SilentlyContinue'
if (Get-Command New-BurntToastNotification -EA 0) {
    New-BurntToastNotification -Text $title, $body -AppLogo $null
} else {
    [void][Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]
    $t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent(1)
    $x = $t.GetElementsByTagName('text')
    [void]$x.Item(0).AppendChild($t.CreateTextNode($title))
    [void]$x.Item(1).AppendChild($t.CreateTextNode($body))
    [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Claude Code').Show(
        [Windows.UI.Notifications.ToastNotification]::new($t))
}
`, strings.ReplaceAll(body, "'", "''")))

	if _, err := d.runNotifyCmd(cmd); err != nil {
		log.Printf("windows notification: %v", err)
	}
}

func (d *daemon) killNotify() {
	d.mu.Lock()
	cmd := d.notifyCmd
	d.notifyCmd = nil
	d.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}
	cmd.Process.Signal(syscall.SIGTERM)
	ch := make(chan struct{})
	go func() { cmd.Wait(); close(ch) }()
	select {
	case <-ch:
	case <-time.After(500 * time.Millisecond):
		cmd.Process.Kill()
	}
}

func (d *daemon) focusWindow(req *request) {
	if req.WindowAddr != "" {
		run("hyprctl", "dispatch", "focuswindow", "address:"+req.WindowAddr)
	} else if req.Workspace != "" {
		run("hyprctl", "dispatch", "workspace", req.Workspace)
	}

	if req.TmuxSocket != "" && req.PaneID != "" {
		env := append(os.Environ(), "TMUX="+req.TmuxSocket)
		runEnv(env, "tmux", "switch-client", "-t", req.Session)
		runEnv(env, "tmux", "select-window", "-t", req.target())
		runEnv(env, "tmux", "select-pane", "-t", req.PaneID)
	}
}

// ── Status API ──

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

// ── Persistence ──

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

func (d *daemon) scheduleSave() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.saveTimer != nil {
		d.saveTimer.Stop()
	}
	d.saveTimer = time.AfterFunc(saveDebounce, func() {
		d.saveState()
	})
}

// ── Lifecycle ──

func (d *daemon) idleWatchdog() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			idle := time.Since(d.lastActivity)
			d.mu.Unlock()
			if idle >= idleTimeout {
				log.Println("idle timeout, shutting down")
				d.shutdown()
				return
			}
		case <-d.done:
			return
		}
	}
}

func (d *daemon) shutdown() {
	d.killNotify()
	d.saveState() // persist before exit
	d.mu.Lock()
	if d.doneTimer != nil {
		d.doneTimer.Stop()
	}
	if d.saveTimer != nil {
		d.saveTimer.Stop()
	}
	d.mu.Unlock()
	if d.listener != nil {
		d.listener.Close()
	}
	os.Remove(socketPath)
	os.Remove(pidFile)
	select {
	case <-d.done:
	default:
		close(d.done)
	}
}

func (d *daemon) serve() error {
	oldMask := syscall.Umask(0077)
	var err error
	d.listener, err = net.Listen("unix", socketPath)
	syscall.Umask(oldMask)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		d.listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0600)

	d.startTime = time.Now()
	d.fmt.load()
	d.loadState()
	d.applyTmuxMarkers()
	d.cleanupStale() // immediate stale check on startup
	d.refreshCounts()

	log.Printf("listening on %s (pid %d)", socketPath, os.Getpid())

	go d.idleWatchdog()
	go d.staleCleanupLoop()

	sem := make(chan struct{}, maxConnections)

	for {
		conn, err := d.listener.Accept()
		if err != nil {
			select {
			case <-d.done:
				return nil
			default:
				continue
			}
		}
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			d.handleConn(conn)
		}()
	}
}

// ── tmux helpers ──

func tmux(args ...string) {
	exec.Command("tmux", args...).Run()
}

func tmuxGetOption(name string) string {
	out, _ := exec.Command("tmux", "show-option", "-gqv", name).Output()
	return strings.TrimSpace(string(out))
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
	for _, line := range strings.Split(paneContent, "\n") {
		if strings.Contains(line, "esc to interrupt") {
			return stateActive
		}
		if strings.Contains(line, "Esc to cancel") {
			return stateAttention
		}
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

// ── Entry point ──

func main() {
	log.SetFlags(log.Ltime)
	log.SetPrefix("claude-state: ")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: claude-state <serve|active|attention|stopped|clear|notify|status>\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		ensureRuntimeDir()
		conn, err := net.DialTimeout("unix", socketPath, time.Second)
		if err == nil {
			conn.Close()
			fmt.Fprintln(os.Stderr, "daemon already running")
			os.Exit(0)
		}
		os.Remove(socketPath)

		if null, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0); err == nil {
			os.Stdin = null
		}

		d := &daemon{
			windows:      make(map[string]*trackedWindow),
			lastActivity: time.Now(),
			done:         make(chan struct{}),
		}

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
		go func() { <-sigs; d.shutdown() }()

		if err := d.serve(); err != nil {
			log.Fatal(err)
		}

	case "active", "attention", "stopped", "clear", "notify":
		clientMain(os.Args[1])

	case "status":
		statusMain()

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
