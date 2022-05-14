// Copyright 2021 Daniel Erat.
// All rights reserved.

package storage

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"cloud.google.com/go/storage"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
	"github.com/google/subcommands"

	"golang.org/x/oauth2/google"

	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type storageClass string

const (
	standard storageClass = "STANDARD"
	nearline              = "NEARLINE"
	coldline              = "COLDLINE"
	archive               = "ARCHIVE"
)

type Command struct {
	Cfg *client.Config

	bucketName   string // GCS bucket name
	class        string // storage class for low-rated files
	maxUpdates   int    // files to update
	numWorkers   int    // concurrent GCS updates
	ratingCutoff int    // min rating for standard storage class
}

func (*Command) Name() string     { return "storage" }
func (*Command) Synopsis() string { return "update song storage classes" }
func (*Command) Usage() string {
	return `storage [flags]:
	Update song files' storage classes in Google Cloud Storage based on
	ratings in dumped songs from stdin.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.bucketName, "bucket", "", "Google Cloud Storage bucket containing songs")
	f.StringVar(&cmd.class, "class", string(coldline), "Storage class for infrequently-accessed files")
	f.IntVar(&cmd.maxUpdates, "max-updates", -1, "Maximum number of files to update")
	f.IntVar(&cmd.numWorkers, "workers", 10, "Maximum concurrent Google Cloud Storage updates")
	f.IntVar(&cmd.ratingCutoff, "rating-cutoff", 4, "Minimum song rating for standard storage class")
}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	if cmd.bucketName == "" {
		fmt.Fprintln(os.Stderr, "Must supply bucket name with -bucket")
		return subcommands.ExitUsageError
	}
	class := storageClass(cmd.class)
	if class != nearline && class != coldline && class != archive {
		fmt.Fprintf(os.Stderr, "Invalid -class %q (valid: %v %v %v)\n", class, nearline, coldline, archive)
		return subcommands.ExitUsageError
	}

	creds, err := google.FindDefaultCredentials(ctx,
		"https://www.googleapis.com/auth/devstorage.read_write",
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed finding credentials:", err)
		return subcommands.ExitFailure
	}
	client, err := storage.NewClient(ctx, option.WithCredentials(creds))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed creating client:", err)
		return subcommands.ExitFailure
	}
	defer client.Close()

	// Read songs from stdin and determine the proper storage class for each.
	songClasses := make(map[string]storageClass)
	d := json.NewDecoder(os.Stdin)
	for {
		var s db.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to read song:", err)
			return subcommands.ExitFailure
		}
		cls := standard
		if s.Rating > 0 && s.Rating < cmd.ratingCutoff {
			cls = class
		}
		songClasses[s.Filename] = cls
	}

	// List the objects synchronously so we know how many jobs we'll have.
	var jobs []job
	bucket := client.Bucket(cmd.bucketName)
	it := bucket.Objects(ctx, &storage.Query{Prefix: ""})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "Failed listing objects in %v: %v\n", cmd.bucketName, err)
			return subcommands.ExitFailure
		}
		class, ok := songClasses[attrs.Name]
		if ok && attrs.StorageClass != string(class) {
			jobs = append(jobs, job{*attrs, class})
			if cmd.maxUpdates > 0 && len(jobs) >= cmd.maxUpdates {
				break
			}
		}
	}

	// See https://gobyexample.com/worker-pools.
	jobChan := make(chan job, len(jobs))
	resChan := make(chan result, len(jobs))

	// Start the workers.
	for i := 0; i < cmd.numWorkers; i++ {
		go worker(ctx, bucket, jobChan, resChan)
	}

	// Submit the jobs.
	for _, j := range jobs {
		jobChan <- j
	}
	close(jobChan)

	// Wait for all the jobs to finish.
	var numErrs int
	for i := 0; i < len(jobs); i++ {
		res := <-resChan
		msg := fmt.Sprintf("[%d/%d] %q: %v -> %v", i+1, len(jobs),
			res.attrs.Name, res.attrs.StorageClass, res.class)
		if res.err == nil {
			log.Print(msg)
		} else {
			numErrs++
			log.Printf("%s failed: %v", msg, res.err)
		}
	}
	if numErrs > 0 {
		fmt.Fprintf(os.Stderr, "Failed updating %v object(s)\n", numErrs)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type job struct {
	attrs storage.ObjectAttrs // original attributes
	class storageClass        // new storage class
}

type result struct {
	job
	err error
}

func worker(ctx context.Context, bucket *storage.BucketHandle, jobs <-chan job, results chan<- result) {
	for j := range jobs {
		obj := bucket.Object(j.attrs.Name)
		copier := obj.CopierFrom(obj)
		copier.StorageClass = string(j.class)

		// Preserve a bunch of random junk.
		copier.ContentType = j.attrs.ContentType
		copier.ContentLanguage = j.attrs.ContentLanguage
		copier.CacheControl = j.attrs.CacheControl
		copier.ACL = j.attrs.ACL
		copier.PredefinedACL = j.attrs.PredefinedACL
		copier.ContentEncoding = j.attrs.ContentEncoding
		copier.ContentDisposition = j.attrs.ContentDisposition
		copier.Metadata = j.attrs.Metadata

		_, err := copier.Run(ctx)
		results <- result{j, err}
	}
}
