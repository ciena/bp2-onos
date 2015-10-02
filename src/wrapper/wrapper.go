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
	"crypto/tls"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"onos"
	"github.com/davidkbainbridge/jsonq"
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
	kubeCreds           = "vagrant:vagrant"
	kubeOnosSelector    = "name in (onos)"
	httpTimeout         = "5s"
	maxErrorCount       = 10
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

// watchPods watches updates from kubernetes and updates the cluster information (and kicks onos) when membership
// changes.
func watchPods(kube string) {

	cluster := onos.StringSet{}
	// The set in the cluster will always include myself.
	ip, err := onos.GetMyIP()
	if err != nil {
		// add loopback, may not be the best solution
		cluster.Add("127.0.0.1")
	} else {
		cluster.Add(ip)
	}

	// We are going to use a SSL transport with verification turned off
	timeout, err := time.ParseDuration(httpTimeout)
	if err != nil {
		log.Printf("ERROR: unable to parse default HTTP timeout of '%s', will default to no timeout\n", httpTimeout)
	}
	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}

	log.Printf("INFO: fetch cluster information from 'https://%s@%s/api/v1/namespaces/default/pods?labelSelector=%s'\n",
		kubeCreds, kube, url.QueryEscape(kubeOnosSelector))

	resp, err := client.Get("https://" + kubeCreds + "@" + kube + "/api/v1/namespaces/default/pods?labelSelector=" + url.QueryEscape(kubeOnosSelector))
	if err != nil {
		log.Fatalf("ERROR: Unable to communciate to kubernetes to maintain cluster information: %s\n", err)
	}

	var data map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		log.Fatalf("ERROR: Unable to parse response back from kubernetes: %s\n", err)
	}

	// Populate the cluster set with the base from the query
	jq := jsonq.NewQuery(data)
	items, err := jq.Array("items")
	if err != nil {
		log.Printf("ERROR: Unexpected response from kubernetes: %s\n", err)
	} else {
		modified := false
		for _, item := range items {
			jq = jsonq.NewQuery(item)
			ip, err = jq.String("status.podIP")
			if err == nil {
				if !cluster.Contains(ip) {
					cluster.Add(ip)
					modified = true
				}
			}
		}
		if modified {
			onos.WriteClusterConfig(cluster.Array())
		} else {
			log.Println("INFO: no modification of cluster information based on update from kubernetes")
		}
	}

	log.Printf("INFO: base set of cluster members is %v\n", cluster.Array())

	b, _ := json.MarshalIndent(data, "", "    ")
	log.Printf("DEBUG: %s\n", string(b))

	errCount := 0
	client.Timeout = 0
	for {
		resp, err = client.Get("https://" + kubeCreds + "@" + kube + "/api/v1/namespaces/default/pods?labelSelector=" + url.QueryEscape(kubeOnosSelector) + "&watch=true")
		if err != nil {
			errCount++
			if errCount > maxErrorCount {
				log.Fatalf("ERROR: Too many errors (%d) while attempting to communicate with kubernetes: %s", errCount, err)
			}
		} else {
			// Worked, reset error count
			errCount = 0

			decoder := json.NewDecoder(resp.Body)
			if err != nil {
				errCount++
				if errCount > maxErrorCount {
					log.Fatalf("ERROR: Too many errors (%d) while attempting to communicate with kubernetes: %s", errCount, err)
				}
			} else {
				// Worked, reset error count
				errCount = 0

				for {
					var data map[string]interface{}
					err := decoder.Decode(&data)
					if err == nil {
						b, _ := json.MarshalIndent(data, "", "    ")
						log.Printf("DEBUG: retrieved: %v\n", string(b))
						jq := jsonq.NewQuery(data)
						ip, err = jq.String("object.status.podIP")
						if err == nil {
							modified := false
							log.Printf("IP: (%s) %s == %s\n", jq.AsString("type"), jq.AsString("object.metadata.name"),
								jq.AsString("object.status.podIP"))
							switch jq.AsString("type") {
							case "DELETED":
								if cluster.Contains(ip) {
									cluster.Remove(ip)
									modified = true
								}
							case "MODIFIED":
								fallthrough
							case "ADDED":
								if !cluster.Contains(ip) {
									cluster.Add(ip)
									modified = true
								}
							}
							if modified {
								onos.WriteClusterConfig(cluster.Array())
							} else {
								log.Println("INFO: no modification of cluster information based on update from kubernetes")
							}
						} else {
							log.Printf("ERROR COULD NOT FIND IP: %s\n", err)
						}
					} else {
						log.Printf("ERROR: unable to decode %s\n", err)
					}
				}
			}
		}
	}
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
		if onos.Process.Pid != 0 {
			log.Printf("INFO: killing ONOS process '%d'\n", onos.Process.Pid)
			if err := onos.Process.Kill(); err != nil {
				log.Printf("ERROR: failed to kill ONOS process, bad things about to happen: %s\n", err)
			} else {
				onos.Process.Pid = 0
			}
		} else {
			log.Println("INFO: ONOS already dead, nothing to kill")
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

	// if we can get an IP and port for the kubernetes API so we can monitor the cluster member changes then start
	// a routine to monitor them. We will first attempt to resolve kubernetes as this is the recommended method, if
	// that fails then we will try to environment variables. If that fails, assume not running in kubernetes
	// environment.
	var kube string
	addrs, err := net.LookupHost("kubernetes")
	if err == nil && len(addrs) > 0 {
		// Take the first address
		kube = addrs[0]
	} else {
		log.Printf("INFO: unable to resolve the service name 'kubernetes', will check environment for link: %s\n",
			err)
		// Unable to resolve, attempt environment variables
		kube = os.Getenv("KUBERNETES_SERVICE_HOST")
		log.Printf("INFO: found KUBERNETES_SERVICE_HOST in environment as '%s'\n", kube)
	}

	if kube != "" {
		log.Printf("INFO: resolved 'kubernetes' service to '%s', will monitor pod instances to cluster membership\n",
			kube)
		go watchPods(kube)
	} else {
		log.Println("INFO: cannot resolve 'kubernetes' service, assuming instance not part of a kubernetes deployment")
	}

	log.Printf("INFO: listen and serve REST requests on '%s'\n", serveOnAddr)
	log.Fatalf("ERROR: failed to start API server on '%s': %s\n", serveOnAddr, http.ListenAndServe(serveOnAddr, nil))
}
