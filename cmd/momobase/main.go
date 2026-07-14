package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	_ "github.com/joho/godotenv/autoload"
	"github.com/spf13/cobra"

	"github.com/momobasehq/momobase/internal/bootstrap"
)

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
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          runServe,
	}
	cmd.AddCommand(newServeCommand(), newMigrateCommand(), newSeedAdminCommand())
	return cmd
}

func newServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server and background workers",
		Args:  cobra.NoArgs,
		RunE:  runServe,
	}
}

func runServe(cmd *cobra.Command, _ []string) error {
	app, err := bootstrap.NewApp(bootstrap.LoadConfig())
	if err != nil {
		return err
	}
	err = app.Serve(cmd.Context())
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func newMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database schema migrations",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			app, err := bootstrap.NewApp(bootstrap.LoadConfig())
			if err != nil {
				return err
			}
			if err = bootstrap.AutoMigrate(app.DB); err == nil {
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
		RunE: func(_ *cobra.Command, _ []string) error {
			app, err := bootstrap.NewApp(bootstrap.LoadConfig())
			if err != nil {
				return err
			}
			if err = app.SeedAdmin(email, password, name); err == nil {
				fmt.Printf("admin created: %s\n", email)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&email, "email", "admin@example.com", "admin email")
	cmd.Flags().StringVar(&password, "password", "password123", "admin password")
	cmd.Flags().StringVar(&name, "name", "Super Admin", "admin name")
	return cmd
}
