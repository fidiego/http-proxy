package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorGreen  = lipgloss.Color("2")
	colorYellow = lipgloss.Color("3")
	colorRed    = lipgloss.Color("1")
	colorCyan   = lipgloss.Color("6")
	colorGray   = lipgloss.Color("8")
	colorWhite  = lipgloss.Color("15")
	colorBlue   = lipgloss.Color("4")

	styleStatus = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorGray)

	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorGray).
			Italic(true)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan)

	styleSectionTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorBlue).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240"))

	styleKeyword = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorYellow)

	styleError = lipgloss.NewStyle().
			Foreground(colorRed)

	styleTag = lipgloss.NewStyle().
			Foreground(colorCyan).
			Background(lipgloss.Color("17")).
			Padding(0, 1)

	styleDivider = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	tableSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("238")).
				Bold(true)
)

// statusColor returns a lipgloss color for an HTTP status code.
func statusColor(code int) lipgloss.Color {
	switch {
	case code >= 500:
		return colorRed
	case code >= 400:
		return colorYellow
	case code >= 300:
		return colorCyan
	case code >= 200:
		return colorGreen
	default:
		return colorGray
	}
}
