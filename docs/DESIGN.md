# JEND Design System & Guidelines

This document outlines the core design language, color palettes, and component patterns used across the JEND ecosystem. JEND strives to provide a premium, modern, and highly polished user experience—whether the user is interacting via the Command Line Interface (CLI), the Terminal User Interface (TUI), or the Mobile Web Interface.

---

## 1. Core Principles

1. **Vibrant & Premium Aesthetics**: JEND uses rich, dark-themed backgrounds contrasted with highly vibrant, saturated gradients (specifically our signature purple) to create a striking first impression.
2. **Platform Consistency**: The UI components, terminology, and visual hierarchy must remain consistent between the local terminal (using `lipgloss` and `bubbletea`) and the remote web interfaces (using CSS).
3. **Clarity Over Clutter**: Progress bars, transfer states, and settings should be intuitive. Avoid printing raw data unless explicitly requested. Use iconography (✓, ✗, ⬆, ⬇) and color to convey state.
4. **Resilience & Accessibility**: The terminal UI gracefully degrades if basic colors are unsupported, and web interfaces must be fully responsive down to the narrowest mobile screens.

---

## 2. Color Palette (Dark Theme Default)

JEND automatically detects if the user's terminal has a dark or light background and adjusts accordingly. However, the default and "hero" aesthetic is the **Dark Theme**.

The colors below map to our `lipgloss.Color` variables in `internal/ui/styles.go` and directly correspond to the CSS hex codes in our web interfaces (e.g., `qrupload_page.go`).

* **Bg** `#16161A` — The absolute background color of the terminal or web page.
* **Panel** `#242629` — Slightly elevated background used for cards, code blocks, or active input fields.
* **Primary** `#7F5AF0` (Soft Purple) — The signature brand color. Used for ASCII logos, primary titles, and the base of gradients `#7F5AF0` → `#6B3FD4`.
* **Secondary / Success** `#2CB67D` (Muted Green) — Used for "Done", "Connected", green checkmarks (✓), and positive actions.
* **Accent** `#00F0FF` (Cyan) — Used for highlighting specific terms, variables, URLs, and active download arrows (⬇).
* **Warning** `#F9C74E` (Yellow) — Used for warnings (⚠️), rate limits reached, or impending expiration notices.
* **Error** `#EF4565` (Red) — Used for failures, disconnected peers, red crosses (✗), or quarantine notices.
* **Text** `#FFFFFE` (Off-White) — Primary text color for high contrast against dark backgrounds.
* **Subtext** `#94A1B2` (Blue-Gray) — Used for muted hints, helper text, timestamps, or secondary data formatting.

---

## 3. Terminal Interface (CLI / TUI)

We use `github.com/charmbracelet/bubbletea` for interactive prompts and `github.com/charmbracelet/lipgloss` for styling standard log lines.

### Styling Rules

* **No Raw `fmt.Printf`**: Avoid printing raw text without style wrappers. Always wrap dynamic states and file names in `ui.ColorAccent` or logs in `ui.ColorSuccess`.
* **Clear Lines**: When updating progress bars in a loop, always use the ANSI clear line escape sequence `\033[K` to prevent ghost characters and graphical tearing in older terminals.
* **ASCII Banners**: Every major entry point (e.g., `jend send`, `jend receive`) must print the `ui.RenderBanner()` to establish branding.

### Interactive Widgets (Wizard)

* **Checklists/Radios**: Active selections should use the `›` character and be styled with `RadioActiveStyle` (Accent/Bold). Hidden/unselected elements use `RadioInactiveStyle` (Subtext).
* **Buttons/Prompts**: When prompting the user for Yes/No, highlight the default choice in brackets `[Yes]`. Keybinds (`esc`, `enter`, `j/k`) should be indicated at the bottom of the screen using the `WizardHelpStyle` (faint subtext).

---

## 4. Mobile Web Interface (QR Uploads)

The Web interface is served locally when users initiate a QR code scan (`jend receive --qr`). The HTML/CSS is bundled entirely within Go to avoid external footprint dependencies.

### CSS Guidelines

* **Zero Dependencies**: There is no TailwindCSS, Bootstrap, or external JavaScript. pure CSS3 and Vanilla JS.
* **Buttons**: All buttons must share the exact `.btn` utility class. They must have substantial padding (`16px`), rounded corners (`12px`), heavy font-weight, and slightly elevated drop-shadows `box-shadow: 0 4px 24px rgba(...)`.
* **Gradients**:
  * Primary Button (Upload): `linear-gradient(135deg, #7F5AF0, #6B3FD4)`
  * Camera Button: `linear-gradient(135deg, #2CB67D, #1A9D6C)`
  * Text Button: `linear-gradient(135deg, #FF8906, #F25F4C)`
* **Hover Micro-animations**: Interactive elements (drop zones, buttons) should lift slightly on hover `transform: translateY(-2px)` and expand their drop-shadow to create a tactile feel.

### Responsiveness

* Use viewport meta tags preventing scaling.
* Use `max-width: 420px; width: 100%` on container cards to ensure the interface isn't awkwardly stretched horizontally on wide tablets, but fits perfectly on narrow iOS or Android devices.

---

## 5. File Formats & Fallbacks

* **HEIC Support**: Because this is a mobile-first upload client, `<input type="file">` fields must explicitly handle `image/heic` and `image/heif` to prevent Apple iOS Safari Webviews from dropping the file picker payload before it even hits Javascript.
