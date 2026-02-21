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
