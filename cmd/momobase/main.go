package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	_ "github.com/joho/godotenv/autoload"

	"github.com/momobasehq/momobase"
	"github.com/momobasehq/momobase/providers"
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
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return runServe(ctx)
	}
	command, args := args[0], args[1:]
	if command != "seed-admin" && len(args) == 1 && isHelp(args[0]) {
		return printUsage(out)
	}
	switch command {
	case "help", "-h", "--help":
		return printUsage(out)
	case "version", "--version":
		if err := noArgs(command, args); err != nil {
			return err
		}
		return printVersion(out)
	case "serve":
		if err := noArgs(command, args); err != nil {
			return err
		}
		return runServe(ctx)
	case "migrate":
		if err := noArgs(command, args); err != nil {
			return err
		}
		return runMigrate(ctx, out)
	case "seed-admin":
		return runSeedAdmin(ctx, args, out)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func isHelp(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func noArgs(command string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("%s accepts no arguments", command)
	}
	return nil
}

func printUsage(out io.Writer) error {
	_, err := fmt.Fprint(out, `Run and manage the Momobase payment service.

Usage:
  momobase [serve]
  momobase migrate
  momobase seed-admin --password PASSWORD [--email EMAIL] [--name NAME]
  momobase version
`)
	return err
}

func printVersion(out io.Writer) error {
	_, err := fmt.Fprintf(
		out,
		"momobase %s\ncommit: %s\nbuilt:  %s\ngo:     %s %s/%s\n",
		version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
	return err
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
func providerFactories() map[string]providers.Factory {
	return map[string]providers.Factory{
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

func withInstance(run func(*momobase.Instance) error) (err error) {
	instance, err := loadInstance()
	if err != nil {
		return err
	}
	defer closeInstance(instance, &err)
	return run(instance)
}

func runServe(ctx context.Context) error {
	return withInstance(func(instance *momobase.Instance) error {
		return instance.Serve(ctx)
	})
}

func runMigrate(ctx context.Context, out io.Writer) error {
	return withInstance(func(instance *momobase.Instance) error {
		if err := instance.Migrate(ctx); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "migrations applied")
		return err
	})
}

type seedAdminOptions struct {
	email    string
	password string
	name     string
}

func parseSeedAdmin(args []string, out io.Writer) (seedAdminOptions, error) {
	options := seedAdminOptions{}
	flags := flag.NewFlagSet("seed-admin", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.StringVar(&options.email, "email", "admin@momobase.local", "admin email")
	flags.StringVar(&options.password, "password", "", "admin password")
	flags.StringVar(&options.name, "name", "Super Admin", "admin name")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() != 0 {
		return options, errors.New("seed-admin accepts no positional arguments")
	}
	if options.password == "" {
		return options, errors.New("seed-admin: --password is required")
	}
	return options, nil
}

func runSeedAdmin(ctx context.Context, args []string, out io.Writer) error {
	options, err := parseSeedAdmin(args, out)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	return withInstance(func(instance *momobase.Instance) error {
		if err := instance.SeedAdmin(ctx, options.email, options.password, options.name); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "admin created: %s\n", options.email)
		return err
	})
}
