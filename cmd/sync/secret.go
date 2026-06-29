package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kelsos/rotki-sync/internal/config"
	"github.com/kelsos/rotki-sync/internal/logger"
	"github.com/kelsos/rotki-sync/internal/process"
	"github.com/kelsos/rotki-sync/internal/secrets"
	"github.com/kelsos/rotki-sync/internal/services"
)

// secretCmd builds the `secret` command tree for managing the age-encrypted
// store that holds per-user login passwords.
func secretCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage the age-encrypted secret store (user login passwords)",
		Long: "Manage the age-encrypted secret store. Passwords are encrypted at rest;\n" +
			"the age identity is kept in the OS keyring with an env/key-file fallback.",
	}
	cmd.AddCommand(
		secretInitCmd(),
		secretSetCmd(),
		secretRmCmd(),
		secretListCmd(),
		secretCheckCmd(cfg),
	)
	return cmd
}

func secretInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the encrypted secret store and its age identity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store := secrets.Default()
			if store.Exists() {
				return fmt.Errorf("secret store already exists at %s", store.Path())
			}
			recipient, mode, err := store.Init()
			if err != nil {
				return err
			}
			fmt.Printf("✓ created secret store: %s\n", store.Path())
			fmt.Printf("  identity stored in: %s\n", mode)
			fmt.Printf("  recipient:          %s\n", recipient)
			if mode == "file" {
				fmt.Println("  (no OS keyring available; private key written to a 0600 key file)")
			}
			fmt.Println("  next: rotki-sync secret set <username>")
			return nil
		},
	}
}

func secretSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <username> [password]",
		Short: "Store or update a user's login password",
		Long: "Store a user's login password. With no password argument it is read from\n" +
			"the terminal without echo (preferred, so it stays out of shell history).",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := secrets.Default()
			username := args[0]

			var password string
			if len(args) == 2 {
				password = args[1]
			} else {
				pw, err := promptPassword(fmt.Sprintf("Password for %s: ", username))
				if err != nil {
					return err
				}
				password = pw
			}
			if password == "" {
				return fmt.Errorf("password must not be empty")
			}

			if err := store.Set(secrets.ScopeUsers, username, password); err != nil {
				return err
			}
			fmt.Printf("✓ stored password for %s\n", username)
			return nil
		},
	}
}

func secretRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <username>",
		Short: "Remove a user's stored password",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := secrets.Default()
			if err := store.Rm(secrets.ScopeUsers, args[0]); err != nil {
				return err
			}
			fmt.Printf("✓ removed password for %s\n", args[0])
			return nil
		},
	}
}

func secretListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List users with a stored password (names only)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store := secrets.Default()
			names, err := store.Keys(secrets.ScopeUsers)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Println("no stored users; add one with: rotki-sync secret set <username>")
				return nil
			}
			for _, n := range names {
				fmt.Println(n)
			}
			return nil
		},
	}
}

func secretCheckCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Verify stored passwords authenticate against a live rotki-core",
		Long: "Boot rotki-core and, for each user, log in then immediately log out using the\n" +
			"stored password. No sync is performed. Exits non-zero if any user fails.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			os.Exit(runSecretCheck(cfg))
			return nil
		},
	}
}

// runSecretCheck boots rotki-core, verifies every user's stored password via a
// login/logout round-trip, prints a per-user report, and returns an exit code.
func runSecretCheck(cfg *config.Config) int {
	logger.Init()
	cfg.SetBaseURL()

	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration: %v", err)
	}

	rotki, err := process.StartRotkiCore(cfg.BinPath, cfg.Port, cfg.APIReadyTimeout, cfg.DataDir)
	if err != nil {
		logger.Fatal("Failed to start rotki-core: %v", err)
	}

	syncService := services.NewSyncService(cfg)
	defer syncService.Cleanup()

	if !syncService.WaitForAPIReady() {
		logger.Fatal("API failed to become ready")
	}

	results, checkErr := syncService.CheckCredentials()
	stopRotki(rotki)

	if checkErr != nil {
		logger.Error("Credential check could not run: %v", checkErr)
		return exitStepFailure
	}

	failed := 0
	for _, r := range results {
		if r.OK {
			fmt.Printf("✓ %s\n", r.Username)
			continue
		}
		failed++
		fmt.Printf("✗ %s: %v\n", r.Username, r.Err)
	}

	if failed > 0 {
		logger.Error("%d of %d users failed the credential check", failed, len(results))
		return exitStepFailure
	}
	logger.Info("All %d users authenticated successfully", len(results))
	return exitOK
}

// promptPassword reads a password from the terminal without echo. When stdin is
// not a terminal (piped input) it reads a single line instead.
func promptPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	// #nosec G115 - a standard fd (stdin) is always a small, non-overflowing value
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimRight(scanner.Text(), "\r\n"), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}
