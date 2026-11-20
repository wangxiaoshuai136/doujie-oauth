//go:build !windows

package main

// browserLoginSupported reports whether this platform build implements the
// isolated-browser sign-in flow. Only the Windows build has the CDP-based
// browser implementation; other platforms use QR sign-in.
const browserLoginSupported = false
