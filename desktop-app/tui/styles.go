package tui

import (
	"github.com/charmbracelet/lipgloss"
)

const (
	sidebarWidth = 22
)

const LogoHeight = 10 // Number of lines in the logo

var (
	// Non-AI-slop color palette: Grounded slate + warm amber accent
	// Avoids vivid purples, teals, neons used by generic AI tools
	PrimaryColor   = lipgloss.AdaptiveColor{Light: "#1E293B", Dark: "#F1F5F9"} // Slate: main text, headers
	SecondaryColor = lipgloss.AdaptiveColor{Light: "#D97706", Dark: "#FBBF24"} // Amber: accents, active states
	BackgroundColor = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#0F172A"} // App background
	SurfaceColor   = lipgloss.AdaptiveColor{Light: "#F8FAFC", Dark: "#1E293B"} // Panels, cards
	BorderColor    = lipgloss.AdaptiveColor{Light: "#E2E8F0", Dark: "#334155"} // Borders, dividers
	SuccessColor   = lipgloss.AdaptiveColor{Light: "#059669", Dark: "#34D399"} // Success states
	ErrorColor     = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"} // Errors, delete actions
	DimTextColor   = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"} // Secondary text
	TextColor      = lipgloss.AdaptiveColor{Light: "#1E293B", Dark: "#F1F5F9"} // Main text

	// Logo & Header
	LogoStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true)

	TitleStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true).
			Padding(0, 1)

	// Sidebar Tabs
	SidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(BorderColor).
			PaddingRight(1)

	SidebarItemStyle = lipgloss.NewStyle().
			Foreground(DimTextColor).
			Padding(0, 1).
			PaddingLeft(2).
			MarginBottom(1).
			Width(sidebarWidth - 2) // 20 chars

	ActiveSidebarItemStyle = SidebarItemStyle.Copy().
			Background(SurfaceColor).
			Foreground(SecondaryColor).
			Bold(true).
			Width(sidebarWidth - 2). // Keep consistent 20 char width
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(SecondaryColor).
			PaddingLeft(1) // 1 (border) + 1 (padding) = 2

	// Content Panels
	PanelStyle = lipgloss.NewStyle().
			Background(SurfaceColor).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BorderColor).
			Padding(1, 2).
			MarginBottom(1)

	// List Items
	ListItemStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(BorderColor)

	SelectedListItemStyle = ListItemStyle.Copy().
			Background(SurfaceColor).
			Foreground(PrimaryColor).
			Bold(true)

	// Text Styles
	HeaderStyle = lipgloss.NewStyle().
			Foreground(TextColor).
			Bold(true).
			Underline(true).
			MarginBottom(1)

	LabelStyle = lipgloss.NewStyle().
			Foreground(SecondaryColor).
			Bold(true)

	FocusLabelStyle = LabelStyle.Copy().
			Foreground(SecondaryColor).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(SecondaryColor)

	DimTextStyle = lipgloss.NewStyle().
			Foreground(DimTextColor)

	// Status & Feedback
	StatusStyle = lipgloss.NewStyle().
			Foreground(DimTextColor).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(BorderColor).
			Padding(0, 1)

	ToastStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1E293B"}).
			Background(SuccessColor).
			Padding(0, 1).
			Bold(true).
			Align(lipgloss.Right).
			MarginTop(1)

	ErrorToastStyle = ToastStyle.Copy().
			Background(ErrorColor)

	// Help
	HelpStyle = lipgloss.NewStyle().
			Foreground(DimTextColor).
			Padding(0, 1)

	// Progress Bars
	ProgressStyle = lipgloss.NewStyle().
			Foreground(SecondaryColor)

	// Settings Inputs
	InputStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(BorderColor).
			Padding(0, 1)

	FocusedInputStyle = InputStyle.Copy().
			BorderForeground(SecondaryColor)

	// Confirmation Dialog
	DialogStyle = lipgloss.NewStyle().
			Background(SurfaceColor).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(SecondaryColor).
			Padding(1, 2)
)
