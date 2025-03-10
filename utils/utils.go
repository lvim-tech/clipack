package utils

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"strings"
	"time"
)

func AskForConfirmation(s string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [y/n]: ", s)

		response, err := reader.ReadString('\n')
		if err != nil {
			return false
		}

		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		}
	}
}

func EnsureDirectoryExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}

func DownloadContent(url string) ([]byte, error) {
	url = strings.Replace(url, "github.com", "raw.githubusercontent.com", 1)
	url = strings.Replace(url, "/blob/", "/", 1)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download content: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download content: status code %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %v", err)
	}

	return content, nil
}

func GetCurrentUser() string {
	currentUser, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return currentUser.Username
}

func FormatDateTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

func GetCurrentDateTime() time.Time {
	return time.Now().UTC()
}

func CompareVersions(currentVersion, newVersion string) bool {
	return currentVersion != newVersion
}

func IsLatestInstallation(method string) bool {
	return method == "latest"
}
