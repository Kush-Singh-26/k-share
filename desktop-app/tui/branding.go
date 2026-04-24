package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	// ASCII Art Logo for K-SHARE - Original Version
	KShareLogo = `⠀⠀⠀⢀⣤⣤⣤⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⢀⣿⣿⣿⣿⡇⠀⠀⠀⠀⣀⣀⣠⣤⣴⣶⣶⡟⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⢸⣿⣿⣿⣿⠁⠀⠀⠘⠿⠿⠿⢛⣻⣿⣿⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⣼⣿⣿⣿⡿⠀⣠⣴⣾⣿⣿⣿⣿⣿⠟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⣿⣿⠟⢋⣴⣿⣿⣿⣿⣿⠟⠉⠻⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⢸⡿⢁⣴⣿⣿⣿⣿⡿⢋⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠈⣴⣿⣿⣿⣿⡿⢋⣴⣿⣿⣦⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⣼⣿⣿⣿⣿⠋⠀⠻⣿⣿⣿⣿⣷⣄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⢸⣿⣿⣿⣿⠇⠀⠀⠀⠙⢿⣿⣿⣿⣿⣷⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠘⠿⠿⠿⠋⠀⠀⠀⠀⠀⠈⠛⠿⢿⠿⠟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀`

	// Standard Emojis for high compatibility
	IconHistory   = "📜 "
	IconFiles     = "📁 "
	IconClipboard = "📋 "
	IconSettings  = "⚙️ "
	IconServer    = "📱 "
	IconLoading   = "⏳ "
	IconSuccess   = "✅ " 
	IconError     = "❌ "
	IconDelete    = "🗑️ "
)

type BrandingConfig struct {
	UseIcons bool
}

var Branding = BrandingConfig{
	UseIcons: true, // Default to true, but can be toggled via settings
}

func (b BrandingConfig) GetIcon(icon string, fallback string) string {
	return icon
}

func RenderLogo(maxWidth int) string {
	if maxWidth < 40 {
		return lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true).Render("K-SHARE")
	}
	
	lines := strings.Split(KShareLogo, "\n")
	var styledLines []string
	style := lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if len(trimmed) == 0 {
			continue
		}
		// Use lipgloss.Width() for display width instead of len() (byte count)
		// This correctly handles multi-byte characters like braille and emoji
		if lipgloss.Width(trimmed) > maxWidth {
			trimmed = trimToWidth(trimmed, maxWidth)
		}
		styledLines = append(styledLines, style.Render(trimmed))
	}
	return strings.Join(styledLines, "\n")
}

func trimToWidth(s string, maxWidth int) string {
	var result []rune
	width := 0
	for _, r := range s {
		charWidth := 1
		if r > 0x1F000 {
			charWidth = 2 // Rough emoji/wide char check
		}
		if width+charWidth > maxWidth {
			break
		}
		result = append(result, r)
		width += charWidth
	}
	return string(result)
}
