package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Execute terminates the process, so it is exercised by re-running this test
// binary as a child and inspecting what the child did. The environment variable
// tells the child which half of the test to run.
const (
	executeEnv     = "CLIPACK_TEST_EXECUTE"
	executeFailure = "failure"
	executeSuccess = "success"
)

func TestExecute(t *testing.T) {
	// Child side: run the real Execute and let it decide the exit code.
	switch os.Getenv(executeEnv) {
	case executeFailure:
		rootCmd.SetArgs([]string{"definitely-not-a-command"})
		Execute()
		return
	case executeSuccess:
		rootCmd.SetArgs([]string{"help"})
		Execute()
		return
	}

	tests := []struct {
		name     string
		mode     string
		wantExit int
		wantErr  string
	}{
		{
			name:     "a failing command exits non-zero",
			mode:     executeFailure,
			wantExit: 1,
			// The message is prefixed so it is obvious which tool failed.
			wantErr: "clipack: unknown command",
		},
		{
			name:     "a successful command exits zero",
			mode:     executeSuccess,
			wantExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=TestExecute")
			child.Env = append(os.Environ(), executeEnv+"="+tt.mode)

			output, err := child.CombinedOutput()

			exitCode := 0
			if err != nil {
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("running the child: %v", err)
				}
				exitCode = exitErr.ExitCode()
			}

			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\noutput:\n%s", exitCode, tt.wantExit, output)
			}
			if tt.wantErr != "" && !strings.Contains(string(output), tt.wantErr) {
				t.Errorf("output does not contain %q:\n%s", tt.wantErr, output)
			}
		})
	}
}
