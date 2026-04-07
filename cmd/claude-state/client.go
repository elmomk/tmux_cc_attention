package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

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
