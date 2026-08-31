// Package tui is the keyboard-only terminal interface. It uses Bubble Tea but
// exposes a very small set of screens: home, status, downloads, library, and
// search. Every command also exists as a --json CLI path.
package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/therealmangoosey/TAB-IGNORE/internal/app"
	"github.com/therealmangoosey/TAB-IGNORE/internal/scrub"
	"github.com/therealmangoosey/TAB-IGNORE/pkg/hermit"
)

type screen int

const (
	screenHome screen = iota
	screenStatus
	screenDownloads
	screenLibrary
	screenSearch
)

type menuItem struct {
	Key  string
	Text string
}

var menuItems = []menuItem{
	{"1", "Status"},
	{"2", "Downloads"},
	{"3", "Library"},
	{"4", "Search"},
	{"5", "Doctor"},
	{"q", "Quit"},
}

// Model is the Bubble Tea model.
type Model struct {
	app      *app.App
	wide     int
	height   int
	screen   screen
	cursor   int
	status   hermit.Status
	items    []string
	message  string
	search   string
}

// NewModel creates the TUI model.
func NewModel(application *app.App) *Model {
	return &Model{app: application, screen: screenHome, wide: 80, height: 24}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg { return statusMsg{} }
}

type statusMsg struct{ status hermit.Status }

func (m *Model) refreshStatus() tea.Cmd {
	return func() tea.Msg {
		st, err := m.app.Status(context.Background())
		if err != nil {
			return statusMsg{status: hermit.Status{}}
		}
		return statusMsg{status: st}
	}
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.wide = msg.Width
		m.height = msg.Height
		return m, nil
	case statusMsg:
		m.status = msg.status
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "1":
		m.screen = screenStatus
		m.cursor = 0
		m.items = m.statusItems()
		return m, m.refreshStatus()
	case "2":
		m.screen = screenDownloads
		m.cursor = 0
		m.items = m.jobItems()
		return m, m.refreshStatus()
	case "3":
		m.screen = screenLibrary
		m.cursor = 0
		m.items = m.libraryItems()
		return m, m.refreshStatus()
	case "4":
		m.screen = screenSearch
		m.search = ""
		m.message = "type then press enter"
		return m, nil
	case "5":
		m.screen = screenStatus
		m.cursor = 0
		m.items = m.statusItems()
		return m, m.refreshStatus()
	case "esc":
		m.screen = screenHome
		m.cursor = 0
		return m, nil
	case "enter", "right":
		if m.screen == screenSearch {
			m.message = m.runSearch()
		} else if len(m.items) > 0 {
			m.message = m.items[m.cursor%len(m.items)]
		}
		return m, nil
	case "up", "k":
		if len(m.items) > 0 {
			m.cursor--
			if m.cursor < 0 {
				m.cursor = len(m.items) - 1
			}
		}
		return m, nil
	case "down", "j":
		if len(m.items) > 0 {
			m.cursor++
			if m.cursor >= len(m.items) {
				m.cursor = 0
			}
		}
		return m, nil
	}
	if m.screen == screenSearch && len(msg.String()) == 1 {
		m.search += msg.String()
		return m, nil
	}
	return m, nil
}

func (m *Model) statusItems() []string {
	var out []string
	st := m.status
	if st.Version == "" {
		out = append(out, "no daemon status")
	}
	out = append(out, fmt.Sprintf("version: %s", st.Version))
	out = append(out, fmt.Sprintf("profile: %s · %s/%s", st.Profile.ProfileName, st.Profile.OS, st.Profile.Arch))
	out = append(out, fmt.Sprintf("cpu: %d · termux: %v", st.Profile.CPUCount, st.Profile.Termux))
	out = append(out, fmt.Sprintf("library: %d files · %.1f GB", st.LibraryFiles, float64(st.LibrarySize)/1e9))
	out = append(out, fmt.Sprintf("spare: %.1f GB", float64(st.SpareBytes)/1e9))
	out = append(out, fmt.Sprintf("server: %s", st.ServerAddr))
	for _, p := range st.Providers {
		out = append(out, fmt.Sprintf("provider %s enabled=%v ok=%v", p.ID, p.Enabled, p.OK))
	}
	return out
}

func (m *Model) jobItems() []string {
	var out []string
	for _, j := range m.status.Jobs {
		out = append(out, fmt.Sprintf("%d · %s · %s · %d%%", j.ID, j.Provider, j.State, percent(j.BytesDone, j.BytesTotal)))
	}
	if len(out) == 0 {
		out = append(out, "no active jobs")
	}
	return out
}

func (m *Model) libraryItems() []string {
	files, err := m.app.Library.Scan(context.Background())
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	for _, f := range files {
		out = append(out, scrub.SafeName(f.Path))
	}
	sort.Strings(out)
	if len(out) == 0 {
		out = append(out, "library is empty")
	}
	return out
}

func (m *Model) runSearch() string {
	if strings.TrimSpace(m.search) == "" {
		return "enter a search query"
	}
	hits, err := m.app.Search(context.Background(), m.search, hermit.KindTV)
	if err != nil {
		return "search error: " + err.Error()
	}
	if len(hits) == 0 {
		return "no results"
	}
	var out []string
	for _, h := range hits {
		out = append(out, fmt.Sprintf("%s (%d) [tmdb=%d]", h.Title, h.Year, h.Ref.TMDBID))
	}
	m.screen = screenLibrary
	m.items = out
	m.cursor = 0
	return fmt.Sprintf("%d results", len(hits))
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.wide <= 0 {
		m.wide = 80
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("36")).Render("hermit")
	body := title + "\n"
	body += m.renderMenu()
	body += "\n" + m.renderBody()
	return body
}

func (m *Model) renderMenu() string {
	var b strings.Builder
	for i, item := range menuItems {
		col := "37"
		if m.screen == screenStatus && i == 0 || m.screen == screenDownloads && i == 1 || m.screen == screenLibrary && i == 2 || m.screen == screenSearch && i == 3 || m.screen == screenStatus && i == 4 {
			col = "33"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Render(item.Key + " " + item.Text))
		if i < len(menuItems)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func (m *Model) renderBody() string {
	if m.screen == screenHome {
		return " j/k move · enter select · 1-5 screens · q quit\n" +
			" keyboard-only by design; every screen has a --json twin\n"
	}
	var b strings.Builder
	if m.message != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render(m.message) + "\n")
	}
	if m.screen == screenSearch {
		b.WriteString("search: " + m.search + "_\n")
	}
	for i, item := range m.items {
		marker := " "
		if m.screen != screenSearch && i == m.cursor {
			marker = "▸"
		}
		line := marker + " " + item
		if m.screen != screenSearch && i == m.cursor {
			b.WriteString(lipgloss.NewStyle().Bold(true).Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func percent(done, total int64) int {
	if total <= 0 {
		return 0
	}
	if done >= total {
		return 100
	}
	return int(done * 100 / total)
}

// Run starts the TUI.
func Run(application *app.App) error {
	p := tea.NewProgram(NewModel(application), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
