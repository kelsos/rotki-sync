package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kelsos/rotki-sync/internal/models"
)

type SyncStage string

const (
	StageIdle              SyncStage = "idle"
	StageLogin             SyncStage = "login"
	StageSnapshot          SyncStage = "snapshot"
	StageTrades            SyncStage = "trades"
	StageEvents            SyncStage = "events"
	StageTransactions      SyncStage = "transactions"
	StageTransactionsFetch SyncStage = "fetch-txs"
	StageDecode            SyncStage = "decode"
	StageDecodeChains      SyncStage = "decode-chains"
	StageLogout            SyncStage = "logout"
	StageComplete          SyncStage = "complete"
)

type SyncStatus struct {
	Stage    SyncStage
	Progress float64
	Message  string
	Error    error
}

type UserSyncStatus struct {
	Username      string
	Status        SyncStatus
	StartTime     time.Time
	CompletedTime time.Time
}

type Model struct {
	users        []string
	userStatuses map[string]*UserSyncStatus
	activeTasks  []models.TaskID
	logs         []string
	spinner      spinner.Model
	progress     progress.Model
	width        int
	height       int
	quit         bool
	errorCount   int
	successCount int
}

type SyncUpdate struct {
	Username string
	Stage    SyncStage
	Progress float64
	Message  string
	Error    error
}

type LogMessage struct {
	Message string
}

type TaskUpdate struct {
	TaskID models.TaskID
	Status string
}

type UsersLoaded struct {
	Users []string
}

func NewModel() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	pr := progress.New(progress.WithDefaultGradient())

	return Model{
		users:        []string{},
		userStatuses: make(map[string]*UserSyncStatus),
		activeTasks:  []models.TaskID{},
		logs:         []string{},
		spinner:      sp,
		progress:     pr,
		width:        80,
		height:       24,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.handleKeyMsg(msg) {
			m.quit = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m = m.handleWindowSizeMsg(msg)

	case UsersLoaded:
		m = m.handleUsersLoaded(msg)

	case SyncUpdate:
		m = m.handleSyncUpdate(msg)

	case LogMessage:
		m = m.handleLogMessage(msg)

	case TaskUpdate:
		m = m.handleTaskUpdate(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		if progressModel, ok := progressModel.(progress.Model); ok {
			m.progress = progressModel
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "q", "ctrl+c":
		return true
	}
	return false
}

func (m Model) handleWindowSizeMsg(msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height
	m.progress.Width = msg.Width - 40
	return m
}

func (m Model) handleUsersLoaded(msg UsersLoaded) Model {
	m.users = msg.Users
	for _, user := range msg.Users {
		m.userStatuses[user] = &UserSyncStatus{
			Username: user,
			Status: SyncStatus{
				Stage: StageIdle,
			},
		}
	}
	return m
}

func (m Model) handleSyncUpdate(msg SyncUpdate) Model {
	if status, exists := m.userStatuses[msg.Username]; exists {
		status.Status.Stage = msg.Stage
		status.Status.Progress = msg.Progress
		status.Status.Message = msg.Message
		status.Status.Error = msg.Error

		if msg.Stage == StageLogin && status.StartTime.IsZero() {
			status.StartTime = time.Now()
		}

		if msg.Stage == StageComplete {
			status.CompletedTime = time.Now()
			if msg.Error != nil {
				m.errorCount++
			} else {
				m.successCount++
			}
		}
	}
	return m
}

func (m Model) handleLogMessage(msg LogMessage) Model {
	m.logs = append(m.logs, fmt.Sprintf("[%s] %s",
		time.Now().Format("15:04:05"), msg.Message))
	if len(m.logs) > 10 {
		m.logs = m.logs[len(m.logs)-10:]
	}
	return m
}

func (m Model) handleTaskUpdate(msg TaskUpdate) Model {
	if msg.Status == "completed" {
		for i, taskID := range m.activeTasks {
			if taskID == msg.TaskID {
				m.activeTasks = append(m.activeTasks[:i], m.activeTasks[i+1:]...)
				break
			}
		}
	} else if msg.Status == "started" {
		m.activeTasks = append(m.activeTasks, msg.TaskID)
	}
	return m
}

func (m Model) View() string {
	if m.quit {
		return "Shutting down...\n"
	}

	var s strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	s.WriteString(headerStyle.Render("🔄 Rotki Sync Monitor"))
	s.WriteString("\n\n")

	// Summary
	summaryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244"))

	summary := fmt.Sprintf("Users: %d | ✅ Success: %d | ❌ Errors: %d | ⏳ Active Tasks: %d",
		len(m.users), m.successCount, m.errorCount, len(m.activeTasks))
	s.WriteString(summaryStyle.Render(summary))
	s.WriteString("\n\n")

	// User sync status
	userSectionStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1).
		Width(m.width - 2)

	var userStatus strings.Builder
	userStatus.WriteString("📊 User Sync Status\n")
	userStatus.WriteString(strings.Repeat("─", 60) + "\n")

	for _, user := range m.users {
		status, exists := m.userStatuses[user]
		if !exists {
			continue
		}

		statusIcon := getStageIcon(status.Status.Stage)
		stageColor := getStageColor(status.Status.Stage)

		userLine := fmt.Sprintf("%s %-15s %s %-12s",
			statusIcon,
			truncate(user, 15),
			m.spinner.View(),
			status.Status.Stage)

		if status.Status.Stage != StageIdle && status.Status.Stage != StageComplete {
			progressBar := m.progress.ViewAs(status.Status.Progress)
			userLine += fmt.Sprintf(" %s", progressBar)
		}

		if status.Status.Error != nil {
			errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
			userLine += " " + errorStyle.Render(fmt.Sprintf("Error: %v", status.Status.Error))
		} else if status.Status.Message != "" {
			messageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
			userLine += " " + messageStyle.Render(status.Status.Message)
		}

		stageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(stageColor))
		userStatus.WriteString(stageStyle.Render(userLine) + "\n")
	}

	s.WriteString(userSectionStyle.Render(userStatus.String()))
	s.WriteString("\n\n")

	// Logs section
	logSectionStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(m.width - 2).
		Height(8)

	var logSection strings.Builder
	logSection.WriteString("📝 Recent Logs\n")
	for _, log := range m.logs {
		logSection.WriteString(log + "\n")
	}

	s.WriteString(logSectionStyle.Render(logSection.String()))
	s.WriteString("\n\n")

	// Footer
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	footer := "Press 'q' to quit | Logs: logs/rotki-sync_*.log"
	s.WriteString(footerStyle.Render(footer))

	return s.String()
}

func getStageIcon(stage SyncStage) string {
	switch stage {
	case StageIdle:
		return "⏸"
	case StageLogin:
		return "🔐"
	case StageSnapshot:
		return "📸"
	case StageTrades:
		return "💱"
	case StageEvents:
		return "📡"
	case StageTransactions:
		return "📊"
	case StageTransactionsFetch:
		return "🔄"
	case StageDecode:
		return "🔍"
	case StageDecodeChains:
		return "⚙️"
	case StageLogout:
		return "🚪"
	case StageComplete:
		return "✅"
	default:
		return "❓"
	}
}

func getStageColor(stage SyncStage) string {
	switch stage {
	case StageIdle:
		return "244"
	case StageComplete:
		return "82"
	default:
		return "39"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
