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
	replaceID      = "99999"
	notifyTimeout  = "30000"
	idleTimeout    = 10 * time.Minute
	readTimeout    = 2 * time.Second
	donePopupSec   = 5
	maxWindows     = 500  // cap tracked windows to prevent memory exhaustion
	maxConnections = 50   // concurrent socket connections
	maxFieldLen    = 128  // max length for session/window/pane fields
)

var (
	// Per-user socket/pid in XDG_RUNTIME_DIR (falls back to /tmp/claude-state-$UID)
	socketPath = runtimePath("claude-state.sock")
	pidFile    = runtimePath("claude-state.pid")

	// Input validation: session names, window indices, pane IDs.
	// SECURITY: validName must never allow single-quote (') — the PowerShell
	// notification path relies on this for injection prevention.
	validName       = regexp.MustCompile(`^[a-zA-Z0-9_./-]{1,128}$`)
	validDigits     = regexp.MustCompile(`^[0-9]{1,10}$`)
	validPaneID     = regexp.MustCompile(`^%[0-9]{1,10}$`)
	validTmuxSocket = regexp.MustCompile(`^/[a-zA-Z0-9/_.-]+,\d+,\d+$`)
	validTypes      = map[string]bool{
		"active": true, "attention": true, "stopped": true,
		"clear": true, "notify": true,
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
	// Drain stdin so Claude Code hook doesn't block
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
	// TMUX_PANE may be empty for clear (tmux run-shell context)
	if paneID == "" && msgType != "clear" {
		os.Exit(0)
	}

	var info string
	if msgType == "clear" {
		// Clear operates on the currently focused window (no TMUX_PANE needed)
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

	// For notify: resolve Hyprland context and skip if already focused
	if msgType == "notify" {
		if skip := resolveHyprland(&req, paneID); skip {
			os.Exit(0)
		}
	}

	ensureDaemon()
	sendRequest(&req)
}

// resolveHyprland gathers Hyprland workspace/window info for click-to-focus.
// Returns true if the user is already focused on this pane (skip notification).
func resolveHyprland(req *request, paneID string) bool {
	if _, err := exec.LookPath("hyprctl"); err != nil {
		return false
	}

	termPID := tmuxDisplayPane(paneID, "#{client_pid}")
	if termPID == "" {
		return false
	}

	// Walk parent PIDs to find the Hyprland window
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

	// Check if user is already focused on this exact pane
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
		return // socket exists, daemon likely running
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

// ── Daemon ──

type windowState int

const (
	stateNone windowState = iota
	stateActive
	stateAttention
	stateStopped
)

type trackedWindow struct {
	state windowState
}

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

type daemon struct {
	mu           sync.Mutex
	windows      map[string]*trackedWindow
	fmt          formats
	notifyCmd    *exec.Cmd
	doneTimer    *time.Timer
	lastActivity time.Time
	listener     net.Listener
	done         chan struct{}
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

	d.mu.Lock()
	tw := d.getOrCreate(target)
	if tw == nil || tw.state == stateActive {
		d.mu.Unlock()
		return
	}

	tw.state = stateActive
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
}

func (d *daemon) setAttention(req *request) {
	target := req.target()

	d.mu.Lock()
	tw := d.getOrCreate(target)
	if tw == nil {
		d.mu.Unlock()
		return
	}

	tw.state = stateAttention
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
}

func (d *daemon) setStopped(req *request) {
	target := req.target()

	d.mu.Lock()
	tw := d.getOrCreate(target)
	if tw == nil {
		d.mu.Unlock()
		return
	}
	wasActive := tw.state == stateActive

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
				tw := d.windows[target]
				if tw != nil && tw.state == stateStopped {
					tw.state = stateNone
					d.mu.Unlock()
					tmuxUnsetWindow(target)
					d.refreshCounts()
				} else {
					d.mu.Unlock()
				}
			}()
		}
	}

	d.refreshCounts()
}

func (d *daemon) clearWindow(req *request) {
	target := req.target()

	d.mu.Lock()
	tw := d.windows[target]
	if tw == nil || (tw.state != stateAttention && tw.state != stateStopped) {
		d.mu.Unlock()
		return
	}

	tw.state = stateNone
	d.mu.Unlock()

	tmux(
		"set-window-option", "-t", target, "-u", "window-status-format", ";",
		"set-window-option", "-t", target, "-u", "window-status-current-format", ";",
		"set-window-option", "-t", target, "-u", "@claude-attention", ";",
		"set-window-option", "-t", target, "-u", "@claude-stopped",
	)

	d.refreshCounts()
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
		"-F", "#{session_name}:#{window_index}|#{pane_current_command}").Output()
	if err != nil {
		return
	}

	claudeWindows := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if k, cmd, ok := strings.Cut(line, "|"); ok {
			lower := strings.ToLower(cmd)
			if lower == "claude" || lower == "claude-code" {
				claudeWindows[k] = true
			}
		}
	}

	d.mu.Lock()
	var stale []string
	for target, tw := range d.windows {
		if tw.state != stateNone && !claudeWindows[target] {
			stale = append(stale, target)
			tw.state = stateNone
		}
	}
	d.mu.Unlock()

	for _, target := range stale {
		log.Printf("stale: %s", target)
		tmuxUnsetWindow(target)
	}
	if len(stale) > 0 {
		d.refreshCounts()
	}
}

// ── Desktop notifications ──

func (d *daemon) sendDesktopNotification(req *request) {
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

// runNotifyCmd registers cmd as the active notification, runs it, and clears it.
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
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive",
		"-File", "-",
	)
	// Pass script via stdin to avoid argument injection
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
	d.mu.Lock()
	if d.doneTimer != nil {
		d.doneTimer.Stop()
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
	// Set restrictive umask before creating socket (no window of wider access)
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

	log.Printf("listening on %s (pid %d)", socketPath, os.Getpid())

	go d.idleWatchdog()
	go d.staleCleanupLoop()

	// Semaphore limits concurrent connection handlers
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
		fmt.Fprintf(os.Stderr, "usage: claude-state <serve|active|attention|stopped|clear|notify>\n")
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

		// Redirect stdin to /dev/null (daemon never reads it)
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

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
