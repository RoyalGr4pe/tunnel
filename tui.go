package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maxVisibleLogs = 8

type tunnelLogMsg struct {
	line string
}

type tunnelDoneMsg struct {
	err error
}

type tunnelTickMsg time.Time

type terminationMsg struct {
	signal os.Signal
}

type tunnelModel struct {
	publicURL    string
	localTarget  string
	status       string
	lastActivity string
	logs         []string
	showLogs     bool
	completed    bool
	processErr   error
	spinner      int
	events       <-chan tunnelLogMsg
	done         <-chan error
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	goodStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	warnStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	badStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 2)
)

func newTunnelModel(publicURL, localTarget string, events <-chan tunnelLogMsg, done <-chan error) tunnelModel {
	return tunnelModel{
		publicURL:    publicURL,
		localTarget:  localTarget,
		status:       "CONNECTING",
		lastActivity: "Waiting for Cloudflare to register the tunnel...",
		events:       events,
		done:         done,
	}
}

func (m tunnelModel) Init() tea.Cmd {
	return tea.Batch(nextTunnelLog(m.events), waitForTunnelDone(m.done), nextTunnelTick())
}

func (m tunnelModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "l":
			m.showLogs = !m.showLogs
			return m, nil
		}
	case tea.WindowSizeMsg:
		return m, nil
	case tunnelLogMsg:
		m.recordLog(msg.line)
		return m, nextTunnelLog(m.events)
	case tunnelDoneMsg:
		m.completed = true
		m.processErr = msg.err
		if msg.err != nil {
			m.status = "FAILED"
			m.lastActivity = "cloudflared exited with an error. Press l to inspect recent logs."
		} else {
			m.status = "STOPPED"
			m.lastActivity = "cloudflared exited."
		}
		return m, nil
	case tunnelTickMsg:
		if m.status == "CONNECTING" {
			m.spinner = (m.spinner + 1) % 4
			return m, nextTunnelTick()
		}
	case terminationMsg:
		m.lastActivity = fmt.Sprintf("Received %s; disconnecting...", msg.signal)
		return m, tea.Quit
	}
	return m, nil
}

func (m *tunnelModel) recordLog(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	m.logs = append(m.logs, line)
	if len(m.logs) > maxVisibleLogs {
		m.logs = m.logs[len(m.logs)-maxVisibleLogs:]
	}
	m.lastActivity = line
	if strings.Contains(strings.ToLower(line), "registered tunnel connection") {
		m.status = "CONNECTED"
		m.lastActivity = "Cloudflare edge connection registered."
	}
}

func (m tunnelModel) View() string {
	status := warnStyle.Render("o " + m.status)
	if m.status == "CONNECTED" {
		status = goodStyle.Render("o CONNECTED")
	}
	if m.status == "STOPPED" {
		status = badStyle.Render("o STOPPED")
	}
	if m.status == "FAILED" {
		status = badStyle.Render("o FAILED")
	}
	if m.status == "CONNECTING" {
		frames := []string{"|", "/", "-", "\\"}
		status = warnStyle.Render(frames[m.spinner] + " CONNECTING")
	}

	activity := dimStyle.Render(m.lastActivity)
	if m.showLogs {
		activity = strings.Join(m.logs, "\n")
		if activity == "" {
			activity = dimStyle.Render("No Cloudflare output yet.")
		}
	}
	logHint := "l show recent logs"
	if m.showLogs {
		logHint = "l hide logs"
	}

	body := strings.Join([]string{
		titleStyle.Render("KARO  /  PERSONAL TUNNEL"),
		"",
		status,
		"",
		dimStyle.Render("PUBLIC URL"),
		m.publicURL,
		"",
		dimStyle.Render("LOCAL TARGET"),
		m.localTarget,
		"",
		dimStyle.Render("ACTIVITY"),
		activity,
		"",
		dimStyle.Render("q disconnect  |  ctrl+c disconnect  |  " + logHint),
	}, "\n")
	return panelStyle.Render(body) + "\n"
}

func nextTunnelLog(events <-chan tunnelLogMsg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-events
		if !ok {
			return nil
		}
		return message
	}
}

func waitForTunnelDone(done <-chan error) tea.Cmd {
	return func() tea.Msg {
		return tunnelDoneMsg{err: <-done}
	}
}

func nextTunnelTick() tea.Cmd {
	return tea.Tick(180*time.Millisecond, func(at time.Time) tea.Msg {
		return tunnelTickMsg(at)
	})
}

func runTunnelUI(cmd *exec.Cmd, stdout, stderr io.Reader, publicURL, localTarget string) error {
	events := make(chan tunnelLogMsg, 64)
	var output sync.WaitGroup
	output.Add(2)
	go func() {
		defer output.Done()
		readTunnelOutput(stdout, events)
	}()
	go func() {
		defer output.Done()
		readTunnelOutput(stderr, events)
	}()

	done := make(chan error, 1)
	go func() {
		output.Wait()
		done <- cmd.Wait()
		close(events)
	}()

	program := tea.NewProgram(newTunnelModel(publicURL, localTarget, events, done), tea.WithAltScreen())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)

	uiDone := make(chan struct{})
	go func() {
		select {
		case sig := <-signals:
			program.Send(terminationMsg{signal: sig})
		case <-uiDone:
		}
	}()

	final, err := program.Run()
	close(uiDone)
	if err != nil {
		stopTunnel(cmd, done)
		return errors.New("could not run terminal interface")
	}

	model, ok := final.(tunnelModel)
	if !ok {
		stopTunnel(cmd, done)
		return errors.New("terminal interface stopped unexpectedly")
	}
	if !model.completed {
		stopTunnel(cmd, done)
		return nil
	}
	if model.processErr != nil {
		return errors.New("cloudflared stopped with an error")
	}
	return nil
}

func readTunnelOutput(source io.Reader, events chan<- tunnelLogMsg) {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case events <- tunnelLogMsg{line: scanner.Text()}:
		default:
			// The dashboard only needs recent activity; never block tunnel shutdown on logs.
		}
	}
}

func stopTunnel(cmd *exec.Cmd, done <-chan error) {
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		<-done
		return
	}
	<-done
}
