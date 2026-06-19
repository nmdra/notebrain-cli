package cmd

import (
	"os"

	"charm.land/lipgloss/v2"
)

var (
	hasDarkBG = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	lightDark = lipgloss.LightDark(hasDarkBG)

	colorAccent = lightDark(lipgloss.Color("#534AB7"), lipgloss.Color("#AFA9EC")) // purple
	colorMuted  = lightDark(lipgloss.Color("#888780"), lipgloss.Color("#B4B2A9")) // gray
	colorGood   = lightDark(lipgloss.Color("#0F6E56"), lipgloss.Color("#5DCAA5")) // teal
)

var (
	headerStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colorAccent).
			Foreground(colorAccent).
			Bold(true).
			PaddingBottom(1)

	scoreStyle = lipgloss.NewStyle().
			Foreground(colorGood)

	extraStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	rankStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(3).
			Align(lipgloss.Right)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)
)
