// Package utils holds small helpers shared across clipack that do not belong to
// any one subsystem: stdin confirmation, directory creation, downloading a file
// and truncating a string for display.
package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// downloadClient is reused so repeated additional-config downloads share
// connections instead of opening a new one per file.
var downloadClient = &http.Client{Timeout: 60 * time.Second}

// AskForConfirmation prompts on stdin until it gets a yes/no answer.
// EOF (piped input, closed stdin) is treated as "no" rather than looping.
func AskForConfirmation(s string) bool {
	for {
		fmt.Printf("%s [y/N]: ", s)

		response, err := ReadLine()
		if err != nil && response == "" {
			fmt.Println()
			return false
		}

		switch strings.ToLower(strings.TrimSpace(response)) {
		case "y", "yes":
			return true
		case "n", "no", "":
			return false
		}

		if err != nil {
			return false
		}
	}
}

// EnsureDirectoryExists creates path if it is missing.
func EnsureDirectoryExists(path string) error {
	return os.MkdirAll(path, 0o755)
}

// DownloadContent fetches a URL, rewriting GitHub blob links to raw links.
func DownloadContent(url string) ([]byte, error) {
	url = strings.Replace(url, "github.com", "raw.githubusercontent.com", 1)
	url = strings.Replace(url, "/blob/", "/", 1)

	resp, err := downloadClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download content: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download %s: status %d", url, resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	return content, nil
}

// Truncate shortens s to at most width cells, adding an ellipsis.
func Truncate(s string, width int) string {
	runes := []rune(s)
	if width <= 0 || len(runes) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
