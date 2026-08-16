// Package utils holds small helpers shared across clipack that do not belong to
// any one subsystem: stdin confirmation, directory creation and downloading a
// file.
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

// maxDownloadBytes bounds a downloaded configuration file. These are dotfiles
// and shell snippets — kilobytes — so anything past this is a wrong URL or a
// server that means harm, and without the bound the only way to find out is to
// read the whole body into memory.
const maxDownloadBytes = 8 << 20 // 8 MiB

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

	// One byte past the limit is read so that hitting it is distinguishable
	// from a file that is exactly the maximum size.
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}
	if len(content) > maxDownloadBytes {
		return nil, fmt.Errorf("%s is larger than the %d byte limit for downloaded content",
			url, maxDownloadBytes)
	}

	return content, nil
}
