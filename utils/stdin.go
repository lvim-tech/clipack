package utils

import (
	"bufio"
	"io"
	"os"
	"sync"
)

// Every interactive prompt has to share one reader.
//
// bufio reads ahead: a reader created over os.Stdin pulls in as much as is
// available, not just the line it was asked for. A second, independently
// created reader therefore starts after whatever the first one buffered. With a
// terminal this is invisible, because the tty hands over one line at a time,
// but with piped input the later prompts see EOF and silently answer "no":
//
//	printf 'y\ny\n' | clipack install bat   # the second y was swallowed
var (
	stdinMu     sync.Mutex
	stdinReader *bufio.Reader
	stdinSource io.Reader
)

// Stdin returns the shared buffered reader over os.Stdin. The reader is rebuilt
// if os.Stdin has been replaced, which is what tests do.
func Stdin() *bufio.Reader {
	stdinMu.Lock()
	defer stdinMu.Unlock()

	if stdinReader == nil || stdinSource != io.Reader(os.Stdin) {
		stdinSource = os.Stdin
		stdinReader = bufio.NewReader(os.Stdin)
	}
	return stdinReader
}

// ReadLine reads one line from the shared reader, without the trailing newline.
// The line is returned even when it is followed by EOF rather than a newline.
func ReadLine() (string, error) {
	line, err := Stdin().ReadString('\n')
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, err
}
