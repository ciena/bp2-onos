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
	clusterFileName = "/bp2/hooks/cluster.json"
	tabletsFileName = "/bp2/hooks/tablets.json"
)

func echo(from io.ReadCloser, to *os.File) {
	log.Printf("ABOUT TO READ %v\n", from)
	buf := make([]byte, 1024)
	for {
		if cnt, err := from.Read(buf); cnt >= 0 && err == nil {
			to.Write(buf[:cnt])
		} else {
			break
		}
	}
	from.Close()
	log.Printf("DONE READING\n")
}

func endsIn(v string, l []string) bool {
	for _, t := range l {
		if strings.HasSuffix(v, t) {
			return true
		}
	}
	return false
}

func in(v string, l []string) bool {
	for _, t := range l {
		if t == v {
			return true
		}
	}
	return false
}

func wipe(path string, suffixlist []string, excludes []string) {
	log.Printf("ATTEMPTING TO CLEAN: %s\n", path)
	if f, err := os.Stat(path); err == nil {
		if f.IsDir() {
			if infos, err := ioutil.ReadDir(path); err != nil {
				//log.Fatalf("Unable to clean up %s: %s\n", path, err)
				log.Printf("WARN: Unable to clean up %s: %s\n", path, err)
			} else {
				for _, info := range infos {
					if !endsIn(info.Name(), suffixlist) && !in(info.Name(), excludes) {
						log.Printf("REMOVE: %s\n", path+"/"+info.Name())
						if err := os.RemoveAll(path + "/" + info.Name()); err != nil {
							log.Printf("ERROR: unable to remove %s\n", path+"/"+info.Name())
						}
					}
				}
			}
		} else {
			if err := os.Remove(path); err != nil {
				log.Printf("ERROR: unable to remove %s\n", path)
			}
		}
	} else {
		log.Printf("ERROR: unable to stat file: %s\n", path)
	}
}
func startOnos() *exec.Cmd {

	wipe("/tmp", []string{}, []string{})

	// clean("/root/onos/apache-karaf-3.0.3/data", []string{}, []string{})
	// clean("/root/onos/apache-karaf-3.0.3/instances", []string{}, []string{})
	// clean("/root/onos/apache-karaf-3.0.3/lock", []string{}, []string{})

	// log.Println("SAVING CONFIG")
	// save := exec.Command("tar", "-P", "-zcf", "/bp2/save/save.tgz", "/root/onos/config/cluster.json", "/root/onos/config/tablets.json")
	// if err := save.Run(); err != nil {
	// 	log.Printf("ERROR: unable to save config")
	// }

	log.Println("PURGE ONOS")
	clean := exec.Command("rm", "-rf", "/root/onos")
	if err := clean.Run(); err != nil {
		log.Printf("ERROR: unable to remove tree")
	}

	log.Println("RESTORE FROM CLEAN ONOS")
	restore := exec.Command("tar", "-P", "-zxf", "/bp2/save/clean.tgz")
	if err := restore.Run(); err != nil {
		log.Printf("ERROR: unable to restore from clean")
	}

	// log.Println("RESTORE FROM CONFIG")
	// restorec := exec.Command("tar", "-P", "-zxf", "/bp2/save/save.tgz")
	// if err := restorec.Run(); err != nil {
	// 	log.Printf("ERROR: unable to restore config")
	// }

	log.Println("copy service")
	cp := exec.Command("cp", "/bp2/hooks/onos-service", "/root/onos/bin")
	if err := cp.Run(); err != nil {
		log.Printf("ERROR: unable to copy my service %s\n", err)
	}

	log.Println("make dir")
	md := exec.Command("mkdir", "-p", "/root/onos/config")
	if err := md.Run(); err != nil {
		log.Printf("ERROR: unable to mkdir %s\n", err)
	}

	log.Println("copy cluster.json")
	cp = exec.Command("cp", clusterFileName, "/root/onos/config/cluster.json")
	if err := cp.Run(); err != nil {
		log.Printf("ERROR: unable to copy my cluster config %s\n", err)
	}

	// rm := exec.Command("rm", clusterFileName)
	// if err := rm.Run(); err != nil {
	// 	log.Printf("ERROR: unable to remove cluster config %s\n", err)
	// }

	log.Println("copy tablets.json")
	cp = exec.Command("cp", tabletsFileName, "/root/onos/config/tablets.json")
	if err := cp.Run(); err != nil {
		log.Printf("ERROR: unable to copy my tablets: %s\n", err)
	}

	// rm = exec.Command("rm", tabletsFileName)
	// if err := rm.Run(); err != nil {
	// 	log.Printf("ERROR: unable to remove tablets config %s\n", err)
	// }

	onos := exec.Command(os.Args[1], os.Args[2:]...)
	onos.Env = nil
	onos.Dir = "/root/onos"
	if pipe, err := onos.StdoutPipe(); err == nil {
		go echo(pipe, os.Stdout)
	} else {
		log.Printf("ERROR: unable to read stdout %s\n", err)
	}
	if pipe, err := onos.StderrPipe(); err == nil {
		go echo(pipe, os.Stderr)
	} else {
		log.Printf("ERROR: unable to read stderr %s\n", err)
	}

	if err := onos.Start(); err != nil {
		log.Fatalf("ERROR: unable to start ONOS: %s\n", err)
	}
	return onos
}

func main() {

	log.Println("WRAPPER COMMIT: 0009")
	log.Printf("COMMAND: %s", os.Args[1])
	log.Printf("ARGS: %v", os.Args[2:])

	onos := startOnos()

	go func() {
		for {
			log.Println("Waiting for processes to complete")
			onos.Wait()
			log.Println("Process complete, restarting")
			onos = startOnos()
		}
	}()

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Attempting to kill %v\n", onos.Process)
		onos.Process.Kill()
	})
	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Attempting to kill %v\n", onos.Process)
		onos.Process.Kill()
		log.Printf("Stopping wrapper")
		os.Exit(0)
	})

	log.Printf("About to listen and serve")
	log.Fatalf("Failed to start API server: %s\n", http.ListenAndServe("127.0.0.1:4343", nil))
}
