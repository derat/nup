// Copyright 2021 Daniel Erat.
// All rights reserved.

package test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

const appserverTimeout = 15 * time.Second

// DevAppserver wraps a dev_appserver.py process.
type DevAppserver struct {
	addr       string    // address of app's HTTP server, e.g. "localhost:8080".
	cmd        *exec.Cmd // dev_appserver.py process
	storageDir string    // temp dir used for datastore
}

// NewDevAppserver starts a dev_appserver.py process.
// If appPort is 0 or negative, an unused port will be chosen.
// Close should be called later to kill the process.
func NewDevAppserver(appPort int, showLogging bool) (*DevAppserver, error) {
	libDir, err := CallerDir()
	if err != nil {
		return nil, err
	}

	// Prevent multiple instances from trying to bind to the same ports.
	ports, err := FindUnusedPorts(2)
	if err != nil {
		return nil, err
	}
	if appPort <= 0 {
		appPort = ports[0]
	}
	adminPort := ports[1]

	// Prevent multiple instances from using the same Datastore directory.
	storageDir, err := ioutil.TempDir("", "nup_dev_appserver.*")
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(
		"dev_appserver.py",
		"--application=nup-test",
		"--port="+strconv.Itoa(appPort),
		"--admin_port="+strconv.Itoa(adminPort),
		"--storage_path="+storageDir,
		"--datastore_consistency_policy=consistent",
		".")
	cmd.Dir = filepath.Join(libDir, "..") // directory containing app.yaml
	if showLogging {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(storageDir)
		return nil, err
	}

	srv := &DevAppserver{
		addr:       net.JoinHostPort("localhost", strconv.Itoa(appPort)),
		cmd:        cmd,
		storageDir: storageDir,
	}

	// Wait for the server to accept connections.
	start := time.Now()
	for {
		if conn, err := net.DialTimeout("tcp", srv.addr, time.Second); err == nil {
			conn.Close()
			break
		} else if time.Now().Sub(start) > appserverTimeout {
			srv.Close()
			return nil, fmt.Errorf("couldn't connect: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// I was seeing occasional hangs in response to the first request:
	//  ..
	//  INFO     2021-12-06 01:48:34,122 instance.py:294] Instance PID: 18017
	//  INFO     2021-12-06 01:48:34,128 instance.py:294] Instance PID: 18024
	//  2021/12/06 01:48:34 http.ListenAndServe: listen tcp 127.0.0.1:20020: bind: address already in use
	//  INFO     2021-12-06 01:48:34,143 module.py:883] default: "POST /config HTTP/1.1" 200 2
	//
	// I think the http.ListenAndServer call comes from the appengine.Main call in the app's main
	// function. Oddly, the port is always already bound by the app itself. My best guess is that
	// there's a race in dev_appserver.py that can be triggered when a request comes in soon after
	// it starts handling requests. It happens infrequently, and I've still seen the error at least
	// once with a 1-second delay. It didn't happen across dozens of runs with a 3-second delay,
	// though.
	time.Sleep(3 * time.Second)

	return srv, nil
}

// Close stops dev_appserver.py and cleans up its resources.
func (srv *DevAppserver) Close() error {
	defer os.RemoveAll(srv.storageDir)

	// I struggled with reliably cleaning up processes on exit. In the normal-exit case, this seems
	// pretty straightforward (at least for processes that we exec ourselves): start each process in
	// its own process group, and at exit, send SIGTERM to the process group, which will kill all of
	// its members.
	//
	// Things are harder when the test process is killed via SIGINT or SIGTERM. I don't think
	// there's an easy way to run normal cleanup code from the signal handler: the main goroutine
	// will be blocked doing initialization or running tests, and trying to explicitly kill each
	// process group from the signal-handling goroutine seems inherently racy.
	//
	// I experimented with using unix.Setsid to put the test process (and all of its children) into
	// a new session and then iterating through each process and checking its SID, but that prevents
	// the test process from receiving SIGINT when Ctrl+C is typed into the terminal. I also think
	// we can't put each child process into its own session, since setsid automatically creates a
	// new process group as well (leaving us with no easy way to kill all processes).
	//
	// What I've settled on here is letting dev_appserver.py and its child processes inherit our
	// process group (which appears to be rooted at the main test process), and then just sending
	// SIGINT to the root dev_appserver.py process here, which appears to make it exit cleanly. The
	// signal handler (see HandleSignals) sends SIGTERM to the process group, which also seems to
	// make dev_appserver.py exit (possibly less-cleanly).
	//
	// It's also possible to do something like this earlier to make the root dev_appserver.py
	// process receive SIGINT when the test process dies:
	//
	//  cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGINT}
	//
	// PR_SET_PDEATHSIG is Linux-specific and won't help with processes that are started by other
	// packages, though, and it doesn't seem to be necessary here.
	const sig = unix.SIGINT
	if err := unix.Kill(srv.cmd.Process.Pid, sig); err != nil {
		log.Printf("Failed sending %v to %v: %v", sig, srv.cmd.Process.Pid, err)
	}
	return srv.cmd.Wait()
}

// URL returns the app's slash-terminated URL.
func (srv *DevAppserver) URL() string {
	return fmt.Sprintf("http://%v/", srv.addr)
}
