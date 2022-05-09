// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/derat/nup/cmd/nup/check"
	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/config"
	"github.com/derat/nup/cmd/nup/covers"
	"github.com/derat/nup/cmd/nup/dump"
	"github.com/derat/nup/cmd/nup/storage"
	"github.com/derat/nup/cmd/nup/update"
	"github.com/google/subcommands"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage %v: [flag]...\n"+
			"Interacts with the nup App Engine server.\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	configFile := flag.String("config", filepath.Join(os.Getenv("HOME"), ".nup/config.json"),
		"Path to config file")

	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.HelpCommand(), "")

	var cfg client.Config
	subcommands.Register(&check.Command{Cfg: &cfg}, "")
	subcommands.Register(&config.Command{Cfg: &cfg}, "")
	subcommands.Register(&covers.Command{Cfg: &cfg}, "")
	subcommands.Register(&dump.Command{Cfg: &cfg}, "")
	subcommands.Register(&projectidCommand{cfg: &cfg}, "")
	subcommands.Register(&storage.Command{Cfg: &cfg}, "")
	subcommands.Register(&update.Command{Cfg: &cfg}, "")

	flag.Parse()

	if cmd := flag.Arg(0); cmd != "commands" && cmd != "flags" && cmd != "help" {
		if err := client.LoadConfig(*configFile, &cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Unable to read config file:", err)
			os.Exit(int(subcommands.ExitUsageError))
		}
	}

	os.Exit(int(subcommands.Execute(context.Background())))
}

// This is too simple to get its own package.
type projectidCommand struct{ cfg *client.Config }

func (*projectidCommand) Name() string     { return "projectid" }
func (*projectidCommand) Synopsis() string { return "print GCP project ID" }
func (*projectidCommand) Usage() string {
	return `projectid:
	Print the Google Cloud Platform project ID (as derived from the
	config's serverURL field).

`
}
func (cmd *projectidCommand) SetFlags(f *flag.FlagSet) {}
func (cmd *projectidCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	pid, err := cmd.cfg.ProjectID()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed getting project ID:", err)
		return subcommands.ExitFailure
	}
	fmt.Println(pid)
	return subcommands.ExitSuccess
}
