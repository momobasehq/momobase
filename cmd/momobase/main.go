package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/signal"
	"runtime"
	"syscall"

	_ "github.com/joho/godotenv/autoload"
	"github.com/spf13/cobra"

	"github.com/momobasehq/momobase"
	"github.com/momobasehq/momobase/providers/dummy"
)

// Build information, replaced at link time by the release build.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// @title Momobase API
// @version 1.0
// @description Payment orchestration API for application payments and administrative operations.
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter a bearer token using the format: Bearer {token}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := newRootCommand().ExecuteContext(ctx); err != nil {
		log.Fatal(err)
	}
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "momobase",
		Short:         "Run and manage the Momobase payment service",
		Version:       version,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          runServe,
	}
	cmd.AddCommand(newServeCommand(), newMigrateCommand(), newSeedAdminCommand(), newVersionCommand())
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"momobase %s\ncommit: %s\nbuilt:  %s\ngo:     %s %s/%s\n",
				version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH,
			)
			return err
		},
	}
}

func newServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server and background workers",
		Args:  cobra.NoArgs,
		RunE:  runServe,
	}
}

// providerFactories returns the payment providers this binary ships with.
// Momobase registers none of its own, so a build chooses them explicitly;
// applications embedding Momobase call momobase.New directly and register
// whichever providers they need.
//
// The dummy provider simulates payments and moves no money. It is registered so
// that a fresh deployment can be exercised end to end before real credentials
// exist; creating and activating an account for it remains an explicit
// administrative action.
func providerFactories() map[string]momobase.ProviderFactory {
	return map[string]momobase.ProviderFactory{
		"dummy": dummy.New,
	}
}

// loadInstance builds the instance served by this command.
func loadInstance() (*momobase.Instance, error) {
	return momobase.New(momobase.WithProviders(providerFactories()))
}

func closeInstance(instance *momobase.Instance, err *error) {
	if closeErr := instance.Close(); closeErr != nil {
		*err = errors.Join(*err, closeErr)
	}
}

func runServe(cmd *cobra.Command, _ []string) (err error) {
	instance, err := loadInstance()
	if err != nil {
		return err
	}
	defer closeInstance(instance, &err)
	return instance.Serve(cmd.Context())
}

func newMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database schema migrations",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			instance, err := loadInstance()
			if err != nil {
				return err
			}
			defer closeInstance(instance, &err)
			if err = instance.Migrate(); err == nil {
				fmt.Println("migrations applied")
			}
			return err
		},
	}
}

func newSeedAdminCommand() *cobra.Command {
	var email, password, name string
	cmd := &cobra.Command{
		Use:   "seed-admin",
		Short: "Create a super administrator",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (err error) {
			instance, err := loadInstance()
			if err != nil {
				return err
			}
			defer closeInstance(instance, &err)
			if err = instance.SeedAdmin(command.Context(), email, password, name); err == nil {
				fmt.Printf("admin created: %s\n", email)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&email, "email", "admin@momobase.local", "admin email")
	cmd.Flags().StringVar(&password, "password", "", "admin password")
	cmd.Flags().StringVar(&name, "name", "Super Admin", "admin name")
	if err := cmd.MarkFlagRequired("password"); err != nil {
		panic(fmt.Errorf("mark password flag as required: %w", err))
	}
	return cmd
}
