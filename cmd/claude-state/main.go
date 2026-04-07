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
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
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
