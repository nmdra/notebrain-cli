// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"log/slog"
	"os"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

var (
	hasDarkBG = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	lightDark = lipgloss.LightDark(hasDarkBG)

	colorAccent = lightDark(lipgloss.Color("#534AB7"), lipgloss.Color("#AFA9EC")) // purple
)

type spinnerModel struct {
	spinner  spinner.Model
	message  string
	done     bool
	doneChan <-chan struct{}
}

func NewSpinner(message string, doneChan <-chan struct{}) spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(colorAccent)
	return spinnerModel{spinner: s, message: message, doneChan: doneChan}
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForDone(m.doneChan))
}

type doneMsg struct{}

func waitForDone(ch <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-ch
		return doneMsg{}
	}
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case doneMsg:
		m.done = true
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m spinnerModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView(fmt.Sprintf("%s %s\n", m.spinner.View(), m.message))
}

func RunSpinner(message string, doneChan <-chan struct{}) error {
	if !term.IsTerminal(os.Stderr.Fd()) || os.Getenv("TERM") == "dumb" {
		slog.Info(message)
		<-doneChan
		return nil
	}
	p := tea.NewProgram(
		NewSpinner(message, doneChan),
		tea.WithOutput(os.Stderr),
	)
	_, err := p.Run()
	return err
}
