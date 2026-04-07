package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

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
	tmux("set-option", "-g", "@claude-watcher", fmt.Sprintf("#[fg=%s]⦿ ", d.fmt.actColor))

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
	tmux("set-option", "-gu", "@claude-watcher")
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
