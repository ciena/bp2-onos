// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.
package main

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const (
	onosHome            = "/root/onos"
	cleanOnosArchive    = "/bp2/save/clean.tgz"
	onosConfigDir       = "/root/onos/config"
	clusterFileName     = "/bp2/hooks/cluster.json"
	onosClusterConfig   = "/root/onos/config/cluster.json"
	partitionFileName   = "/bp2/hooks/tablets.json"
	onosPartitionConfig = "/root/onos/config/tablets.json"
	serveOnAddr         = "127.0.0.1:4343"
)

// echo take what is ever on one reader and write it to the writer
func echo(from io.ReadCloser, to io.Writer) {
	log.Printf("INFO: about to start reading from %v\n", from)
	buf := make([]byte, 1024)
	for {
		if cnt, err := from.Read(buf); cnt >= 0 && err == nil {
			to.Write(buf[:cnt])
		} else {
			break
		}
	}
	from.Close()
	log.Printf("INFO: done reading from %v\n", from)
}

// endsId true if the string ends in one of the suffixes in the list, else false
func endsIn(v string, l []string) bool {
	for _, t := range l {
		if strings.HasSuffix(v, t) {
			return true
		}
	}
	return false
}

// in true if the string is in the list, else false
func in(v string, l []string) bool {
	for _, t := range l {
		if t == v {
			return true
		}
	}
	return false
}

// wipe removes all files form the specified directory, but not the directory, or if the specified path is a normal
// file, that file is just deleted.
func wipe(path string, suffixlist []string, excludes []string) {
	log.Printf("INFO: wiping '%s'\n", path)
	if f, err := os.Stat(path); err == nil {
		if f.IsDir() {
			if infos, err := ioutil.ReadDir(path); err != nil {
				log.Printf("WARN: unable to wipe '%s': %s\n", path, err)
			} else {
				for _, info := range infos {
					if !endsIn(info.Name(), suffixlist) && !in(info.Name(), excludes) {
						log.Printf("INFO: removing: '%s'\n", path+"/"+info.Name())
						if err := os.RemoveAll(path + "/" + info.Name()); err != nil {
							log.Printf("ERROR: unable to remove '%s': %s\n", path+"/"+info.Name(), err)
						}
					}
				}
			}
		} else {
			if err := os.Remove(path); err != nil {
				log.Printf("ERROR: unable to remove '%s': %s\n", path, err)
			}
		}
	} else {
		log.Printf("ERROR: unable to stat file: '%s': %s\n", path, err)
	}
}

// startOnos provides a uniform process for starting a clean ONOS instance. this assumes the existing instance has
// been killed. the following steps are taken:
//     1. clean up the /tmp directory
//     2. remove the ONOS installation
//     3. install a fresh / clean ONOS installation from a a save tree
//     4. install the updated cluster and partition configuration files
//     5. start the ONOS process
func startOnos() *exec.Cmd {

	log.Println("INFO: wiping /tmp directory")
	wipe("/tmp", []string{}, []string{})

	log.Println("INFO: wiping ONOS installation from /root/onos")
	if err := os.RemoveAll(onosHome); err != nil {
		log.Printf("ERROR: unable to remove ONOS installation: %s\n", err)
	}

	// this is a bit of a hack. there is a clean copy of the source tree in a TAR file, so just call the OS
	// tar command to untar the archive to the correct location. might be cleaner (more portable) it we used
	// the Go tar package, but for this we don't need to work that hard.
	log.Println("INFO: restoring ONOS from a clean archive")
	restore := exec.Command("tar", "-P", "-zxf", cleanOnosArchive)
	if err := restore.Run(); err != nil {
		log.Printf("ERROR: unable to restore from clean archive: %s\n", err)
	}

	log.Println("INFO: create ONOS cluster configuration directory")
	if err := os.MkdirAll(onosConfigDir, 0755); err != nil {
		log.Printf("ERROR: unable to create ONOS cluster configuration directory: %s\n", err)
	}

	// this is a bit of a hack. we could implement our own file copy func in Go, but instead we will use a
	// system call. this is not portable, but for now should be good enough
	log.Println("INFO: copy desired cluster configuration to ONOS configuration area")
	cp := exec.Command("cp", clusterFileName, onosClusterConfig)
	if err := cp.Run(); err != nil {
		log.Printf("ERROR: unable to copy desired cluster config into ONOS: %s\n", err)
	}

	// this is a bit of a hack. we could implement our own file copy func in Go, but instead we will use a
	// system call. this is not portable, but for now should be good enough
	log.Println("INFO: copy desired cluster partition configuration to ONOS configuration area")
	cp = exec.Command("cp", partitionFileName, onosPartitionConfig)
	if err := cp.Run(); err != nil {
		log.Printf("ERROR: unable to copy desired cluster partition config into ONOS: %s\n", err)
	}

	// Execute ONOS as a separate process. we allow for optional arguments to be specified to to ONOS
	onos := exec.Command(os.Args[1], os.Args[2:]...)
	onos.Env = nil
	onos.Dir = onosHome

	// Create readers for stdin and stderr
	if pipe, err := onos.StdoutPipe(); err == nil {
		go echo(pipe, os.Stdout)
	} else {
		log.Printf("ERROR: unable to read ONOS stdout: %s\n", err)
	}
	if pipe, err := onos.StderrPipe(); err == nil {
		go echo(pipe, os.Stderr)
	} else {
		log.Printf("ERROR: unable to read ONOS stderr: %s\n", err)
	}

	if err := onos.Start(); err != nil {
		log.Fatalf("ERROR: unable to start ONOS: %s\n", err)
	}
	return onos
}

func main() {

	log.Printf("INFO: starting ONOS with command '%s' and arguments '%v'\n", os.Args[1], os.Args[2:])
	onos := startOnos()

	// Start a routine that will monitor the ONOS process and when it stops will restart it
	go func() {
		for {
			log.Printf("INFO: waiting for processes to complete: '%d'\n", onos.Process.Pid)
			onos.Wait()
			log.Printf("INFO: ONOS process '%d' complete with '%s', restarting\n",
				onos.Process.Pid, onos.ProcessState.String())
			onos = startOnos()
		}
	}()

	// register a handler for the /reset REST call. this will kill the ONOS process which will automatically restart
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("INFO: killing ONOS process '%d'\n", onos.Process.Pid)
		if err := onos.Process.Kill(); err != nil {
			log.Printf("ERROR: failed to kill ONOS process, bad things about to happen: %s\n", err)
		}
	})

	// register a handler for the /stop REST call, which will stop the wrapper process
	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("INFO: killing ONOS process '%d'\n", onos.Process.Pid)
		if err := onos.Process.Kill(); err != nil {
			log.Printf("ERROR: failed to kill ONOS process, parent about to exit: %s\n", err)
		}
		log.Println("INFO: halting ONOS wrapper")
		os.Exit(0)
	})

	log.Printf("INFO: listen and serve REST requests on '%s'\n", serveOnAddr)
	log.Fatalf("ERROR: failed to start API server on '%s': %s\n", serveOnAddr, http.ListenAndServe(serveOnAddr, nil))
}
