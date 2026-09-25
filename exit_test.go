package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if args, ok := os.LookupEnv("TERMINAL_CLI_ARGS"); ok {
		os.Args = append([]string{"terminal-cli"}, strings.Fields(args)...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// None of these cases reach the network.
func TestSilentExitStatus(t *testing.T) {
	for _, tc := range []struct {
		args string
		want int
	}{
		{"--silent --mode check --type ticker --start-date 2026-09-01", 0},
		{"--silent --mode check --type ticker --start-date 2000-01-01", exitNoMatch},
		{"--silent --api-key x --type ticker --exchanges nope --tokens btc_usd --start-date 2026-09-01", exitNoMatch},
		{"--silent --bogus", exitUsage},
		{"--silent", exitUsage},
		{"--silent --start-date 2026-13-01", exitUsage},
		{"--silent --type nope --start-date 2026-09-01", exitUsage},
		{"--silent --type ticker --exchanges redstonelive --tokens btc_usd --start-date 2026-09-01", exitUsage},
	} {
		t.Run(tc.args, func(t *testing.T) {
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(self)
			// Set but empty, so godotenv cannot fill it in from .env.
			cmd.Env = append(os.Environ(), "TERMINAL_CLI_ARGS="+tc.args, "REDSTONE_TERMINAL_API_KEY=")
			out, err := cmd.CombinedOutput()

			got := 0
			if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
				got = exitErr.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("exit status %d, want %d", got, tc.want)
			}
			if len(out) != 0 {
				t.Errorf("printed %q, want nothing", out)
			}
		})
	}
}
