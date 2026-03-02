package osutils

import (
	"path/filepath"
	"strings"
)

// DangerousExtensions is a list of file extensions that are commonly associated
// with malware, scripts, or executables that could harm a user's machine if opened.
// This list is not exhaustive but covers the most common attack vectors across OSes.
var DangerousExtensions = map[string]bool{
	// Windows Executables & Scripts
	".exe": true, ".dll": true, ".bat": true, ".cmd": true, ".com": true,
	".msi": true, ".msp": true, ".scr": true, ".pif": true, ".vbs": true,
	".vbe": true, ".js": true, ".jse": true, ".ws": true, ".wsf": true,
	".wsh": true, ".ps1": true, ".ps1xml": true, ".ps2": true, ".ps2xml": true,
	".psc1": true, ".psc2": true, ".msc": true, ".hta": true, ".cpl": true,

	// macOS Executables & Packages
	".app": true, ".dmg": true, ".pkg": true, ".command": true, ".scpt": true,
	".applescript": true,

	// Linux / Unix Scripts
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".elf": true,
	".bin": true, ".run": true, ".AppImage": true,

	// Macros / Rich Documents (often used for droppers)
	".docm": true, ".xlsm": true, ".pptm": true,

	// Archives (can be dangerous if auto-extracted, though JEND handles zip safely)
	// We'll leave out .zip, .tar, .gz for normal transfers but flag weird ones
	".jar": true, ".apk": true,

	// Web / Local execution
	".htm": true, ".html": true, ".svg": true,
}

// IsDangerousExtension returns true if the given filename has an extension
// known to be potentially dangerous (executable, script, etc).
func IsDangerousExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return DangerousExtensions[ext]
}
