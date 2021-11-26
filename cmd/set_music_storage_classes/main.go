// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"cloud.google.com/go/storage"

	"github.com/derat/nup/types"

	"google.golang.org/api/iterator"
)

type storageClass string

const (
	standard storageClass = "STANDARD"
	nearline              = "NEARLINE"
	coldline              = "COLDLINE"
	archive               = "ARCHIVE"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage %v: [flag]...\n"+
			"Updates song files' storage classes in Google Cloud Storage.\n"+
			"Unmarshals \"dump_music\" song objects from stdin.\n\n",
			os.Args[0])
		flag.PrintDefaults()
	}
	bucketName := flag.String("bucket", "", "Google Cloud Storage bucket containing songs")
	classString := flag.String("class", string(coldline), "Storage class for infrequently-accessed files")
	maxUpdates := flag.Int("max-updates", -1, "Maximum number of files to update")
	numWorkers := flag.Int("workers", 10, "Maximum concurrent Google Cloud Storage updates")
	ratingCutoff := flag.Float64("rating-cutoff", 0.75, "Minimum song rating for standard storage class")
	flag.Parse()

	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		log.Fatal("Must set GOOGLE_APPLICATION_CREDENTIALS to service account key file")
	}
	if *bucketName == "" {
		log.Fatal("Must supply bucket name with -bucket")
	}
	class := storageClass(*classString)
	if class != nearline && class != coldline && class != archive {
		log.Fatalf("Invalid -class %q (valid: %v %v %v)", class, nearline, coldline, archive)
	}

	// Read songs from stdin and determine the proper storage class for each.
	songClasses := make(map[string]storageClass)
	d := json.NewDecoder(os.Stdin)
	for {
		var s types.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal("Failed to read song: ", err)
		}
		cls := standard
		if s.Rating >= 0 && s.Rating < *ratingCutoff {
			cls = class
		}
		songClasses[s.Filename] = cls
	}

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatal("Failed creating client: ", err)
	}
	defer client.Close()

	// List the objects synchronously so we know how many jobs we'll have.
	var jobs []job
	bucket := client.Bucket(*bucketName)
	it := bucket.Objects(ctx, &storage.Query{Prefix: ""})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			log.Fatalf("Failed listing objects in %v: %v", *bucketName, err)
		}
		class, ok := songClasses[attrs.Name]
		if ok && attrs.StorageClass != string(class) {
			jobs = append(jobs, job{*attrs, class})
			if *maxUpdates > 0 && len(jobs) >= *maxUpdates {
				break
			}
		}
	}

	// See https://gobyexample.com/worker-pools.
	jobChan := make(chan job, len(jobs))
	resChan := make(chan result, len(jobs))

	// Start the workers.
	for i := 0; i < *numWorkers; i++ {
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
		log.Fatalf("Failed updating %v object(s)", numErrs)
	}
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
