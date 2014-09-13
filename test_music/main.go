package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	//"erat.org/nup"
)

func writeConfig(p, serverUrl, updateTimeFile string) {
	f, err := os.Create(p)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if err = json.NewEncoder(f).Encode(struct {
		LastUpdateTimeFile string
		ServerUrl          string
	}{updateTimeFile, serverUrl}); err != nil {
		panic(err)
	}
}

func runCommand(p string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(p, args...)
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return
	}
	if err = cmd.Start(); err != nil {
		return
	}

	outBytes, err := ioutil.ReadAll(outPipe)
	if err != nil {
		return
	}
	errBytes, err := ioutil.ReadAll(errPipe)
	if err != nil {
		return
	}
	stdout = string(outBytes)
	stderr = string(errBytes)
	err = cmd.Wait()
	return
}

func buildBinaries() {
	log.Printf("rebuilding binaries")
	for _, b := range []string{"dump_music", "update_music"} {
		p := filepath.Join("erat.org/nup", b)
		if _, stderr, err := runCommand("go", "install", p); err != nil {
			panic(stderr)
		}
	}
}

func main() {
	server := flag.String("server", "http://localhost:8080/", "URL of running dev_appengine server")
	binDir := flag.String("binary-dir", filepath.Join(os.Getenv("GOPATH"), "bin"), "Directory containing executables")
	flag.Parse()

	buildBinaries()

	dir, err := ioutil.TempDir("", "test_music.")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	cfgPath := filepath.Join(dir, "config.json")
	writeConfig(cfgPath, *server, filepath.Join(dir, "last_update_time"))

	stdout, stderr, err := runCommand(filepath.Join(*binDir, "dump_music"), "-config="+cfgPath)
	if err != nil {
		log.Fatalf("dumping songs failed: %v\nstderr: %v", err, stderr)
	}
	log.Print(stdout)

	/* check that server is empty
	   scan and import
	   do a query?
	   add file, scan and import
	   export
	*/
}
