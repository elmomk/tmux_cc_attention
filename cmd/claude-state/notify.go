package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// windowsNotifyScript shows a Windows toast and auto-focuses the terminal.
// Placeholders: %s = title, %s = body, %s = appID
var windowsNotifyScript = `
[void][Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]
$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent(1)
$x = $t.GetElementsByTagName('text')
[void]$x.Item(0).AppendChild($t.CreateTextNode('%s'))
[void]$x.Item(1).AppendChild($t.CreateTextNode('%s'))
$n = [Windows.UI.Notifications.ToastNotification]::new($t)
$n.ExpirationTime = [DateTimeOffset]::Now.AddSeconds(10)
$n.Tag = 'claude'
$n.Group = 'claude'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('%s').Show($n)
`

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

	winName := strings.TrimSpace(cmdOutput("tmux", "display-message", "-t", req.target(), "-p", "#{window_name}"))
	if winName == "" {
		winName = req.target()
	}
	body := fmt.Sprintf("%s (%s:%s)", winName, req.Session, req.Window)
	platform := tmuxGetOption("@claude-notify-platform")

	switch platform {
	case "linux":
		d.notifyLinux(req, body)
	case "windows":
		d.notifyWindows(req, body)
	default:
		if _, err := exec.LookPath("notify-send"); err == nil {
			d.notifyLinux(req, body)
		} else if _, err := exec.LookPath("powershell.exe"); err == nil {
			d.notifyWindows(req, body)
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

func (d *daemon) notifyWindows(req *request, body string) {
	appID := tmuxGetOption("@claude-notify-appid")
	if appID == "" {
		appID = "Microsoft.WindowsTerminal_8wekyb3d8bbwe!App"
	}

	title := strings.ReplaceAll("Claude Code needs attention", "'", "''")
	bodyEsc := strings.ReplaceAll(body, "'", "''")
	appIDEsc := strings.ReplaceAll(appID, "'", "''")

	script := fmt.Sprintf(windowsNotifyScript, title, bodyEsc, appIDEsc)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)

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
