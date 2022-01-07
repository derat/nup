// Copyright 2021 Daniel Erat.
// All rights reserved.

package config

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"cloud.google.com/go/datastore"

	"github.com/derat/nup/cmd/nup/client"
	srvconfig "github.com/derat/nup/server/config"
	"github.com/google/subcommands"

	"golang.org/x/oauth2/google"

	"google.golang.org/api/appengine/v1"
	"google.golang.org/api/option"
)

type Command struct {
	Cfg *client.Config

	deleteInstances bool   // delete instances after set
	setPath         string // path of config file to set
	service         string // service name whose instances should be deleted
}

func (*Command) Name() string     { return "config" }
func (*Command) Synopsis() string { return "manage server configuration" }
func (*Command) Usage() string {
	return `config [flags]:
	Manage the App Engine server's configuration in Datastore.
	By default, prints the existing JSON-marshaled configuration.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&cmd.deleteInstances, "delete-instances", false, "Delete running instances after setting config")
	f.StringVar(&cmd.setPath, "set", "", "Path of updated JSON config file to save to Datastore")
	f.StringVar(&cmd.service, "service", "default", "Service name for -delete-instances")
}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	projectID, err := cmd.Cfg.ProjectID()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed getting project ID:", err)
		return subcommands.ExitFailure
	}
	creds, err := google.FindDefaultCredentials(ctx,
		"https://www.googleapis.com/auth/datastore",
		"https://www.googleapis.com/auth/appengine.admin",
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed finding credentials:", err)
		return subcommands.ExitFailure
	}
	cl, err := datastore.NewClient(ctx, projectID, option.WithCredentials(creds))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed creating client:", err)
		return subcommands.ExitFailure
	}

	key := &datastore.Key{
		Kind: srvconfig.DatastoreKind,
		Name: srvconfig.DatastoreKeyName,
	}

	// Just fetch and print the active config if requested.
	if cmd.setPath == "" {
		var cfg srvconfig.SavedConfig
		if err := cl.Get(ctx, key, &cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Failed getting config:", err)
			return subcommands.ExitFailure
		}
		fmt.Print(cfg.JSON)
		return subcommands.ExitSuccess
	}

	// Check that the server code will be happy with the new config.
	data, err := ioutil.ReadFile(cmd.setPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed reading config:", err)
		return subcommands.ExitFailure
	}
	cfg, err := srvconfig.ParseConfig(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Bad config:", err)
		return subcommands.ExitFailure
	}

	// Check that the 'nup' command will still be able to access the server with the new config.
	var foundUser bool
	for _, ai := range cfg.BasicAuthUsers {
		if ai.Username == cmd.Cfg.Username {
			if ai.Password != cmd.Cfg.Password {
				fmt.Fprintf(os.Stderr, "Password for user %q doesn't match client config\n", ai.Username)
				return subcommands.ExitFailure
			}
			foundUser = true
			break
		}
	}
	if !foundUser {
		fmt.Fprintf(os.Stderr, "Config doesn't contain user %q for 'nup' command\n", cmd.Cfg.Username)
		return subcommands.ExitFailure
	}

	// Save the config to Datastore.
	if _, err := cl.Put(ctx, key, &srvconfig.SavedConfig{JSON: string(data)}); err != nil {
		fmt.Fprintln(os.Stderr, "Failed saving config:", err)
		return subcommands.ExitFailure
	}

	if cmd.deleteInstances {
		if err := deleteInstances(ctx, projectID, cmd.service, creds); err != nil {
			fmt.Fprintln(os.Stderr, "Failed deleting instances:", err)
			return subcommands.ExitFailure
		}
	}

	return subcommands.ExitSuccess
}

// deleteInstances deletes all App Engine instances of service in projectID.
func deleteInstances(ctx context.Context, projectID, service string, creds *google.Credentials) error {
	asrv, err := appengine.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return err
	}

	vsrv := appengine.NewAppsServicesVersionsService(asrv)
	isrv := appengine.NewAppsServicesVersionsInstancesService(asrv)

	resp, err := vsrv.List(projectID, service).Do()
	if err != nil {
		return err
	}
	for _, ver := range resp.Versions {
		resp, err := isrv.List(projectID, service, ver.Id).Do()
		if err != nil {
			return err
		}
		for _, inst := range resp.Instances {
			log.Println("Deleting instance", inst.Name)
			if _, err := isrv.Delete(projectID, service, ver.Id, inst.Id).Do(); err != nil {
				return err
			}
		}
	}
	return nil
}
