package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	serviceUnitName = "rotki-sync.service"
	timerUnitName   = "rotki-sync.timer"
	// defaultSchedule is a systemd OnCalendar expression. Daily at 09:30; with
	// Persistent=true a run missed while logged out fires right after next login.
	defaultSchedule = "*-*-* 09:30:00"
)

// serviceCmd builds the `service` command tree for the systemd --user timer that
// runs rotki-sync on a schedule. A --user timer (no lingering) only runs while
// you are logged in, so the OS keyring is unlocked and reachable.
func serviceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the systemd --user timer that runs rotki-sync",
	}
	cmd.AddCommand(serviceInstallCmd(), serviceUninstallCmd())
	return cmd
}

func serviceInstallCmd() *cobra.Command {
	var schedule string
	var execPath string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the systemd --user timer",
		Long: "Write rotki-sync.service/.timer to the systemd --user directory and enable\n" +
			"the timer. Run from the installed binary so the unit points at it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if execPath == "" {
				resolved, err := currentExecutable()
				if err != nil {
					return fmt.Errorf("could not resolve the rotki-sync binary path: %w (pass --exec-path)", err)
				}
				execPath = resolved
			}

			dir := userUnitDir()
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			servicePath := filepath.Join(dir, serviceUnitName)
			timerPath := filepath.Join(dir, timerUnitName)

			if err := os.WriteFile(servicePath, []byte(renderServiceUnit(execPath)), 0o644); err != nil { // #nosec G306 - unit files are not secret
				return err
			}
			if err := os.WriteFile(timerPath, []byte(renderTimerUnit(schedule)), 0o644); err != nil { // #nosec G306 - unit files are not secret
				return err
			}
			fmt.Printf("✓ wrote %s\n✓ wrote %s\n", servicePath, timerPath)
			fmt.Printf("  ExecStart: %s --no-tui --yes\n  schedule:  %s\n", execPath, schedule)

			if !hasSystemctl() {
				fmt.Println("  systemctl not found; enable manually once available:")
				fmt.Printf("    systemctl --user daemon-reload && systemctl --user enable --now %s\n", timerUnitName)
				return nil
			}
			if err := runSystemctl("daemon-reload"); err != nil {
				return err
			}
			if err := runSystemctl("enable", "--now", timerUnitName); err != nil {
				return err
			}
			fmt.Printf("✓ enabled %s\n", timerUnitName)
			fmt.Printf("  check: systemctl --user list-timers %s\n", timerUnitName)
			return nil
		},
	}
	cmd.Flags().StringVar(&schedule, "schedule", defaultSchedule, "systemd OnCalendar expression for the timer")
	cmd.Flags().StringVar(&execPath, "exec-path", "", "path to the rotki-sync binary (defaults to the running executable)")
	return cmd
}

func serviceUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Disable and remove the systemd --user timer",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if hasSystemctl() {
				// Best-effort: the units may already be disabled/absent.
				_ = runSystemctl("disable", "--now", timerUnitName)
			}

			dir := userUnitDir()
			for _, name := range []string{timerUnitName, serviceUnitName} {
				path := filepath.Join(dir, name)
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
				fmt.Printf("✓ removed %s\n", path)
			}

			if hasSystemctl() {
				if err := runSystemctl("daemon-reload"); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// userUnitDir is the systemd --user unit directory ($XDG_CONFIG_HOME/systemd/user).
func userUnitDir() string {
	return filepath.Join(configHome(), "systemd", "user")
}

// currentExecutable resolves the absolute, symlink-free path of the running binary.
func currentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return exe, nil
}

func hasSystemctl() bool {
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func runSystemctl(args ...string) error {
	full := append([]string{"--user"}, args...)
	// #nosec G204 - fixed systemctl subcommands, no user-controlled arguments
	c := exec.Command("systemctl", full...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("systemctl --user %v failed: %w", args, err)
	}
	return nil
}

func renderServiceUnit(execPath string) string {
	return fmt.Sprintf(`[Unit]
Description=rotki-sync data sync
After=graphical-session.target

[Service]
Type=oneshot
ExecStart=%s --no-tui --yes
`, execPath)
}

func renderTimerUnit(schedule string) string {
	return fmt.Sprintf(`[Unit]
Description=Run rotki-sync on a schedule

[Timer]
OnCalendar=%s
Persistent=true

[Install]
WantedBy=timers.target
`, schedule)
}
