package shared

import "github.com/charmbracelet/lipgloss"

func Colorize(text, color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(text)
}
