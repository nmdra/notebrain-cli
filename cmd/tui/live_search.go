package tui

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nmdra/notebrain-cli/internal/store"
)

// ─── Dependencies ─────────────────────────────────────────────────

// SearchFunc is the function signature used to perform a vector search.
// It is decoupled from concrete types so tests can inject a stub.
type SearchFunc func(ctx context.Context, query string) ([]store.Result, error)

// ─── Messages ─────────────────────────────────────────────────────

// searchMsg is dispatched after the debounce timer fires.
type searchMsg struct {
	id    int64
	query string
}

// resultsMsg carries results (or an error) back to the Update loop.
type resultsMsg struct {
	id      int64
	results []store.Result
	err     error
}

// ─── Model ────────────────────────────────────────────────────────

// LiveSearchModel is a Bubble Tea model for interactive live semantic search.
// The user types a query; results update automatically after a short debounce.
//
// inflight is a *int64 pointer (heap-allocated) so that value-copies of the
// model (as Bubble Tea requires) do not violate sync/atomic noCopy rules.
type LiveSearchModel struct {
	input     textinput.Model
	list      list.Model
	searchFn  SearchFunc
	vaultName string

	// inflight is a pointer to an int64 counter used for debounce ID tracking.
	// Using a pointer keeps the model copyable (Bubble Tea's interface requirement)
	// while still allowing safe atomic operations.
	inflight *int64
	debounce time.Duration
	loading  bool
	status   string

	quitting bool
	width    int
	height   int
}

// styles
var (
	lsInputBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#5DCAA5")).
				Padding(0, 1)

	lsStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B4B2A9")).
			Italic(true)

	lsHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888780"))

	lsLoadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5DCAA5"))
)

// NewLiveSearch creates a LiveSearchModel.
// searchFn wraps embed+query so the TUI has no direct dependency on the
// store/embedder packages.
func NewLiveSearch(searchFn SearchFunc, vaultName string, limit int, initialQuery string) LiveSearchModel {
	ti := textinput.New()
	ti.Placeholder = "type to search your vault…"
	ti.Focus()
	if initialQuery != "" {
		ti.SetValue(initialQuery)
	}

	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "🔍  NoteBrain Live Search"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // we own the search; list filtering would interfere
	l.SetShowHelp(false)

	var inflightCounter int64
	return LiveSearchModel{
		input:     ti,
		list:      l,
		searchFn:  searchFn,
		vaultName: vaultName,
		inflight:  &inflightCounter,
		debounce:  300 * time.Millisecond,
		status:    "start typing to search…",
	}
}

// ─── Init ─────────────────────────────────────────────────────────

func (m LiveSearchModel) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	// If a pre-populated query was provided, trigger an immediate search.
	if q := m.input.Value(); q != "" {
		cmds = append(cmds, m.triggerSearch(q, 0))
	}
	return tea.Batch(cmds...)
}

// ─── Update ───────────────────────────────────────────────────────

func (m LiveSearchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputH := 3 // border + 1 line input
		helpH := 2
		statusH := 1
		listH := m.height - inputH - helpH - statusH - 2
		if listH < 1 {
			listH = 1
		}
		m.input.SetWidth(m.width - 4) // account for border padding
		m.list.SetSize(m.width, listH)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "down":
			var listCmd tea.Cmd
			m.list, listCmd = m.list.Update(msg)
			return m, listCmd

		case "enter", "o":
			// Open the selected note in Obsidian.
			if item, ok := m.list.SelectedItem().(resultItem); ok {
				_ = openInObsidian(m.vaultName, item.FilePath)
			}
			return m, nil

		default:
			// Let textinput handle the key.
			prevVal := m.input.Value()
			var tiCmd tea.Cmd
			m.input, tiCmd = m.input.Update(msg)
			newVal := m.input.Value()

			var debounceCmd tea.Cmd
			if newVal != prevVal {
				id := atomic.AddInt64(m.inflight, 1)
				debounceCmd = m.triggerSearch(newVal, id)
				m.loading = true
				m.status = ""
			}
			return m, tea.Batch(tiCmd, debounceCmd)
		}

	case searchMsg:
		// Only act if this ID is still current (guards against stale fires).
		if msg.id != atomic.LoadInt64(m.inflight) {
			return m, nil
		}
		if msg.query == "" {
			m.loading = false
			m.status = "start typing to search…"
			m.list.SetItems(nil)
			return m, nil
		}
		// Capture the current inflight pointer for the closure.
		searchID := msg.id
		searchFn := m.searchFn
		return m, func() tea.Msg {
			results, err := searchFn(context.Background(), msg.query)
			return resultsMsg{id: searchID, results: results, err: err}
		}

	case resultsMsg:
		// Discard results from superseded searches.
		if msg.id != atomic.LoadInt64(m.inflight) {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.status = lsStatusStyle.Render("⚠  " + msg.err.Error())
			return m, nil
		}
		items := make([]list.Item, len(msg.results))
		for i, r := range msg.results {
			items[i] = resultItem{r}
		}
		m.list.SetItems(items)
		switch len(msg.results) {
		case 0:
			m.status = lsStatusStyle.Render("no results")
		case 1:
			m.status = lsStatusStyle.Render("1 result")
		default:
			m.status = lsStatusStyle.Render(fmt.Sprintf("%d results", len(msg.results)))
		}
	}

	// Default: pass through to textinput (e.g. cursor blink).
	var tiCmd tea.Cmd
	m.input, tiCmd = m.input.Update(msg)
	return m, tiCmd
}

// triggerSearch returns a Cmd that waits debounce duration then sends a searchMsg.
func (m LiveSearchModel) triggerSearch(query string, id int64) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(m.debounce)
		return searchMsg{id: id, query: query}
	}
}

// ─── View ─────────────────────────────────────────────────────────

func (m LiveSearchModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	inputRendered := lsInputBorderStyle.Render(m.input.View())

	var statusLine string
	if m.loading {
		statusLine = lsLoadingStyle.Render("  searching…")
	} else {
		statusLine = "  " + m.status
	}

	help := lsHelpStyle.Render("  ↑/↓ navigate · enter open in Obsidian · esc quit")

	v := tea.NewView(inputRendered + "\n" + statusLine + "\n" + m.list.View() + "\n" + help)
	v.AltScreen = true
	return v
}
