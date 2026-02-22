package web

import "embed"

// Content holds the embedded web assets for the QR download page.
//
//go:embed download.html
var Content embed.FS
