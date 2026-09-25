//go:build !windows

package main

// Non-Windows builds (Docker, Linux) keep console semantics: the server
// runs in the foreground and the UI is reached by URL directly.
func hideConsoleWindow() {}

func openWebUI(url string) {}
