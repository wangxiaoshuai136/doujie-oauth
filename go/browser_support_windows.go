//go:build windows

package main

// browserLoginSupported reports whether this platform build implements the
// isolated-browser sign-in flow. The Windows build has the CDP-based browser
// implementation, so browser sign-in stays available there.
const browserLoginSupported = true
