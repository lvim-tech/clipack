package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withStdin replaces os.Stdin with a file holding the given input.
func withStdin(t *testing.T, input string) {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = old
		f.Close()
	})
}

func TestReadLine(t *testing.T) {
	withStdin(t, "first\nsecond\n")

	first, err := ReadLine()
	if err != nil {
		t.Fatalf("ReadLine() error = %v", err)
	}
	if first != "first" {
		t.Errorf("first line = %q, want %q", first, "first")
	}

	second, err := ReadLine()
	if err != nil {
		t.Fatalf("second ReadLine() error = %v", err)
	}
	if second != "second" {
		t.Errorf("second line = %q, want %q", second, "second")
	}
}

func TestReadLineWithoutTrailingNewline(t *testing.T) {
	withStdin(t, "no newline at the end")

	line, err := ReadLine()
	// io.EOF is expected, but the content read before it must not be lost.
	if line != "no newline at the end" {
		t.Errorf("line = %q, want the content despite the EOF (err = %v)", line, err)
	}
}

func TestReadLineStripsCarriageReturn(t *testing.T) {
	withStdin(t, "windows line\r\n")

	line, _ := ReadLine()
	if line != "windows line" {
		t.Errorf("line = %q, want the CR stripped", line)
	}
}

func TestStdinIsSharedAcrossPrompts(t *testing.T) {
	// The regression this guards: each prompt used to build its own
	// bufio.Reader. bufio reads ahead, so with piped input the first reader
	// swallowed every later answer and the following prompts saw EOF.
	withStdin(t, "/opt/clipack\ny\n")

	first, err := ReadLine()
	if err != nil {
		t.Fatalf("first read error = %v", err)
	}
	if first != "/opt/clipack" {
		t.Fatalf("first line = %q", first)
	}

	// A confirmation right after must still find its own answer.
	if !AskForConfirmation("proceed?") {
		t.Error("AskForConfirmation() = false; the piped answer was swallowed by the first read")
	}
}

func TestStdinRebuildsWhenStdinIsReplaced(t *testing.T) {
	withStdin(t, "one\n")
	if line, _ := ReadLine(); line != "one" {
		t.Fatalf("line = %q, want one", line)
	}

	// A fresh os.Stdin has to produce a fresh reader rather than serving the
	// previous one's leftovers.
	withStdin(t, "two\n")
	if line, _ := ReadLine(); line != "two" {
		t.Errorf("line = %q, want two after os.Stdin was replaced", line)
	}
}

func TestAskForConfirmation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"y", "y\n", true},
		{"yes", "yes\n", true},
		{"uppercase", "YES\n", true},
		{"padded", "  y  \n", true},
		{"n", "n\n", false},
		{"no", "no\n", false},
		{"empty defaults to no", "\n", false},
		{"eof defaults to no", "", false},
		{"retries until valid", "maybe\nperhaps\ny\n", true},
		{"invalid then eof", "maybe\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStdin(t, tt.input)
			if got := AskForConfirmation("proceed?"); got != tt.want {
				t.Errorf("AskForConfirmation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"short", 20, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"truncate me please", 8, "truncat…"},
		{"anything", 1, "…"},
		{"anything", 0, "anything"}, // a non-positive width is left alone
		{"anything", -5, "anything"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		if got := Truncate(tt.in, tt.width); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
		}
	}
}

func TestTruncateCountsRunesNotBytes(t *testing.T) {
	// Multi-byte input must be cut on a character boundary, not mid-rune.
	got := Truncate("българският текст", 6)
	if runes := []rune(got); len(runes) != 6 {
		t.Errorf("Truncate() = %q (%d runes), want 6 runes", got, len(runes))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("Truncate() = %q, want an ellipsis", got)
	}
}

func TestEnsureDirectoryExists(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")

	if err := EnsureDirectoryExists(nested); err != nil {
		t.Fatalf("EnsureDirectoryExists() error = %v", err)
	}
	if info, err := os.Stat(nested); err != nil || !info.IsDir() {
		t.Fatalf("the directory was not created: %v", err)
	}

	// Calling it again on an existing directory is not an error.
	if err := EnsureDirectoryExists(nested); err != nil {
		t.Errorf("EnsureDirectoryExists() on an existing directory error = %v, want nil", err)
	}
}

func TestEnsureDirectoryExistsFailsOnAFile(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDirectoryExists(file); err == nil {
		t.Error("EnsureDirectoryExists() error = nil for an existing file, want an error")
	}
}

func TestDownloadContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "theme contents")
	}))
	defer server.Close()

	got, err := DownloadContent(server.URL + "/theme.tmTheme")
	if err != nil {
		t.Fatalf("DownloadContent() error = %v", err)
	}
	if string(got) != "theme contents" {
		t.Errorf("DownloadContent() = %q, want %q", got, "theme contents")
	}
}

func TestDownloadContentReportsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := DownloadContent(server.URL + "/missing")
	if err == nil {
		t.Fatal("DownloadContent() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %v, want it to include the status code", err)
	}
}

func TestDownloadContentUnreachable(t *testing.T) {
	if _, err := DownloadContent("http://127.0.0.1:1/nothing"); err == nil {
		t.Error("DownloadContent() error = nil for an unreachable host, want an error")
	}
}

func TestDownloadContentRewritesGitHubBlobURLs(t *testing.T) {
	var requested string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		fmt.Fprint(w, "raw contents")
	}))
	defer server.Close()

	// Registry authors paste blob links; the /blob/ segment has to be dropped
	// so the raw file is fetched rather than the HTML page.
	if _, err := DownloadContent(server.URL + "/owner/repo/blob/main/theme.yml"); err != nil {
		t.Fatalf("DownloadContent() error = %v", err)
	}
	if requested != "/owner/repo/main/theme.yml" {
		t.Errorf("requested %q, want the /blob/ segment removed", requested)
	}
}
