// Package clipboard provides a cross-platform abstraction for writing text
// to the system clipboard.
//
// Supported environments:
//   - macOS: pbcopy
//   - Linux X11: xclip or xsel
//   - Linux Wayland: wl-copy
//   - WSL: clip.exe
//
// The underlying implementation is handled by github.com/atotto/clipboard,
// which selects the appropriate mechanism at runtime.
package clipboard

import (
	"fmt"

	"github.com/atotto/clipboard"
)

// Copy writes text to the system clipboard.
// Returns an error if the clipboard is unavailable (e.g. headless server
// without xclip/wl-copy, or running inside a Docker container).
func Copy(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("clipboard: write failed: %w", err)
	}
	return nil
}

// Available reports whether the clipboard is usable in the current environment.
// Call this before showing clipboard-related UI hints to avoid misleading the user.
func Available() bool {
	return !clipboard.Unsupported
}
