package ui

import "github.com/charmbracelet/lipgloss"

// JEND ASCII art in Rebel-inspired font style
const jendBannerRaw = `
     ██╗███████╗███╗   ██╗██████╗ 
     ██║██╔════╝████╗  ██║██╔══██╗
     ██║█████╗  ██╔██╗ ██║██║  ██║
██   ██║██╔══╝  ██║╚██╗██║██║  ██║
╚█████╔╝███████╗██║ ╚████║██████╔╝
 ╚════╝ ╚══════╝╚═╝  ╚═══╝╚═════╝`

var (
	BannerStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Align(lipgloss.Center)

	TaglineStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext).
			Italic(true).
			Align(lipgloss.Center).
			MarginTop(1)

	SectionHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				Padding(1, 0, 0, 0).
				MarginBottom(1).
				Align(lipgloss.Center)
)

// RenderBanner returns the full styled JEND ASCII art banner.
func RenderBanner() string {
	return BannerStyle.Render(jendBannerRaw)
}

// RenderBannerWithTagline returns the banner + tagline.
func RenderBannerWithTagline() string {
	return lipgloss.JoinVertical(lipgloss.Center,
		RenderBanner(),
		TaglineStyle.Render("Secure, direct file transfer"),
	)
}
