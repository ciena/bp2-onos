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
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"onos"
)

const (
	defaultSuffixList  = "_DATA,_CONFIG"            // variables that end with these suffixes will be assumed JSON formatted
	suffixListOverride = "BP_HOOK_DATA_SUFFIX_LIST" // override the default suffix list using env var
	defaultPrefixList  = "BP_"                      // will gather vars that are prefixed by items in this list
	prefixListOverride = "BP_HOOK_DATA_PREFIX_LIST" // override default prefix list using env var
	peerDataKey        = "BP_HOOK_PEER_DATA"        // env var that contains cluster peering data
	logFileName        = "/bp2/hooks/cluster.log"   // default location to log cluster prcoessing information
	clusterFileName    = "/bp2/hooks/cluster.json"  // default location for "next" cluster configuration
	tabletsFileName    = "/bp2/hooks/tablets.json"  // default location for "next" partition configuration
)

var blackList = []string{suffixListOverride, prefixListOverride}     // vars not to propagate (collect) from env
var logToConsole = flag.Bool("c", false, "log to console, not file") // optional log to console instead of file

// equal determine if two arrays contain the same data
func equal(a, b []string) bool {

	// If not the same length, then not equal, len of nil == 0
	if len(a) != len(b) {
		return false
	}

	// Sort the arrays so that we can walk them simultaneouslly checking for equality
	sort.Sort(onos.AlphaOrder(a))
	sort.Sort(onos.AlphaOrder(b))

	for i := 0; i < len(a); i = i + 1 {
		if strings.Compare(a[i], b[i]) != 0 {
			return false
		}
	}

	return true
}

func main() {

	flag.Parse()

	// Open a log file for appending if they have not requested to log to the console
	var logfile *os.File
	if !*logToConsole {
		var err error
		logfile, err = os.OpenFile(logFileName, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
		if err != nil {
			log.Fatalf("ERROR: Unable to open log file, exiting: %s\n", err)
		}
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	// Check to see if the data suffix list is overrider by an environment variable and then convert the given list,
	// or the default to a string array for checking against later down the code
	suffixList := os.Getenv(suffixListOverride)
	if suffixList == "" {
		suffixList = defaultSuffixList
	}
	log.Printf("INFO: overriding data suffix list to '%s'\n", suffixList)

	// Support both space and comma separated lists
	suffixes := strings.FieldsFunc(suffixList,
		func(r rune) bool { return unicode.IsSpace(r) || r == ',' })

	// Check to see if the data suffix list is overrider by an environment variable and then convert the given list,
	// or the default to a string array for checking against later down the code
	prefixList := os.Getenv(prefixListOverride)
	if prefixList == "" {
		prefixList = defaultPrefixList
	}
	log.Printf("INFO: overriding data prefix list to '%s'\n", prefixList)

	// Support both space and comma separated lists
	prefixes := strings.FieldsFunc(prefixList,
		func(r rune) bool { return unicode.IsSpace(r) || r == ',' })

	bpData := Gather(&Config{
		Verbose:           true,
		DataSuffixList:    suffixes,
		IncludePrefixList: prefixes,
		ExcludeList:       blackList,
	})

	// OK, we have gathered the appropriate data from the environment, what we do with it depends on the hook that
	// was called. We will use the name of the executable to distinguish which hook was called.
	switch path.Base(os.Args[0]) {
	case "southbound-update":
		// Ignore, return 0
		log.Println("INFO: received SOUTHBOUND-UPDATE")
	case "heartbeat":
		// Open the cluster file, and if it is not empty then send what is there to ONOS as a cluster Config. If the
		// request to ONOS fails, don't fail this process as it is just ONOS likely in some initialization state and
		// take way to long to get to a point where it will respond
		log.Println("INFO: received HEARTBEAT")
		return
	case "peer-status":
		fallthrough
	case "peer-update":
		log.Printf("INFO: received %s\n", strings.ToUpper(path.Base(os.Args[0])))

		if peerData, ok := bpData.(map[string]interface{})[peerDataKey].(map[string]interface{}); !ok {
			log.Printf("INFOI: no peer information received from platform, no action taken")
		} else {
			var want []string

			// We want to verify the the new peering information, if it is included in the message, against the existing
			// cluster information in the ONOS configuration. If it has changed we will need to update ONOS.
			if b, err := json.MarshalIndent(peerData, "    ", "    "); err == nil {
				log.Printf("INFO: received peer data from the platform:\n    %s\n", string(b))
			} else {
				log.Printf("ERROR: unable to decode peer data from platform, curious: %s\n", err)
			}

			peers := peerData["peers"].(map[string]interface{})

			// If the data set does not contain any peers then we will skip this data set
			if len(peers) == 0 {
				log.Printf("INFO: empty peering list from platform, no update action taken")
			} else {
				peerLeader := strconv.Itoa(int(peerData["leader"].(float64)))
				log.Printf("INFO: peer leader ID is: %s\n", peerLeader)

				myIP, err := onos.GetMyIP()
				if err == nil {
					want = append(want, myIP)
					log.Printf("INFO: append own IP '%s' to desired cluster list\n", myIP)
				} else {
					log.Println("WARN: unable to determine own ID, unable to add it to desired cluster list")
				}
				for _, peer := range peers {
					// If the IP of the peer is not "me" then add it to the list. We always add ourselves and we don't
					// wanted it added twice
					ip := peer.(map[string]interface{})["ip"].(string)
					if myIP != ip {
						log.Printf("INFO: append peer with IP '%s' to desired cluster list\n", ip)
						want = append(want, ip)
					}
				}

				onos.WriteClusterConfig(want)
				/*
					// Construct object that represents the ONOS cluster information
					cluster := make(map[string]interface{})
					var nodes []interface{}
					for _, ip := range want {
						node := map[string]interface{}{
							"id":      ip,
							"ip":      ip,
							"tcpPort": 9876,
						}
						nodes = append(nodes, node)
					}
					cluster["nodes"] = nodes
					leader := peerData["peers"].(map[string]interface{})[peerLeader].(map[string]interface{})

					// Calculate the prefix by stripping off the last octet and replacing with a wildcard
					ipPrefix := leader["ip"].(string)
					idx := strings.LastIndex(ipPrefix, ".")
					ipPrefix = ipPrefix[:idx] + ".*"
					cluster["ipPrefix"] = ipPrefix

					// Construct object that represents the ONOS partition information. this is created by creating
					// the same number of partitions as there are ONOS instances in the cluster and then putting N - 1
					// instances in each partition.
					//
					// We sort the list of nodes in the cluster so that each instance will calculate the same partition
					// table.
					//
					// IT IS IMPORTANT THAT EACH INSTANCE HAVE IDENTICAL PARTITION CONFIGURATIONS
					partitions := make(map[string]interface{})
					cnt := len(want)
					sort.Sort(onos.IPOrder(want))

					size := cnt - 1
					if size < 3 {
						size = cnt
					}
					for i := 0; i < cnt; i++ {
						part := make([]map[string]interface{}, size)
						for j := 0; j < size; j++ {
							ip := want[(i+j)%cnt]
							part[j] = map[string]interface{}{
								"id":      ip,
								"ip":      ip,
								"tcpPort": 9876,
							}
						}
						name := fmt.Sprintf("p%d", i+1)
						partitions[name] = part
					}

					tablets := map[string]interface{}{
						"nodes":      nodes,
						"partitions": partitions,
					}

					// Write the partition table to a known location where it will be picked up by the ONOS "wrapper" and
					// pushed to ONOS when it is restarted (yes we marshal the data twice, once compact and once with
					// indentation, not efficient, but i want a pretty log file)
					if data, err := json.Marshal(tablets); err != nil {
						log.Printf("ERROR: Unable to encode tables information to write to update file, no file written: %s\n", err)
					} else {
						if b, err := json.MarshalIndent(tablets, "    ", "    "); err == nil {
							log.Printf("INFO: writting ONOS tablets information to cluster file '%s'\n    %s\n",
								tabletsFileName, string(b))
						}
						// Open / Create the file with an exclusive lock (only one person can handle this at a time)
						if fTablets, err := os.OpenFile(tabletsFileName, os.O_RDWR|os.O_CREATE, 0644); err == nil {
							defer fTablets.Close()
							if err := syscall.Flock(int(fTablets.Fd()), syscall.LOCK_EX); err == nil {
								defer syscall.Flock(int(fTablets.Fd()), syscall.LOCK_UN)
								if _, err := fTablets.Write(data); err != nil {
									log.Printf("ERROR: error writing tablets information to file '%s': %s\n",
										tabletsFileName, err)
								}
							} else {
								log.Printf("ERROR: unable to aquire lock to tables file '%s': %s\n", tabletsFileName, err)
							}
						} else {
							log.Printf("ERROR: unable to open tablets file '%s': %s\n", tabletsFileName, err)
						}
					}

					// Write the cluster info to a known location where it will be picked up by the ONOS "wrapper" and
					// pushed to ONOS when it is restarted (yes we marshal the data twice, once compact and once with
					// indentation, not efficient, but i want a pretty log file)
					if data, err := json.Marshal(cluster); err != nil {
						log.Printf("ERROR: Unable to encode cluster information to write to update file, no file written: %s\n", err)
					} else {
						if b, err := json.MarshalIndent(cluster, "    ", "    "); err == nil {
							log.Printf("INFO: writting ONOS cluster information to cluster file '%s'\n    %s\n",
								clusterFileName, string(b))
						}
						// Open / Create the file with an exclusive lock (only one person can handle this at a time)
						if fCluster, err := os.OpenFile(clusterFileName, os.O_RDWR|os.O_CREATE, 0644); err == nil {
							defer fCluster.Close()
							if err := syscall.Flock(int(fCluster.Fd()), syscall.LOCK_EX); err == nil {
								defer syscall.Flock(int(fCluster.Fd()), syscall.LOCK_UN)
								if _, err := fCluster.Write(data); err != nil {
									log.Printf("ERROR: error writing cluster information to file '%s': %s\n",
										clusterFileName, err)
								}
							} else {
								log.Printf("ERROR: unable to aquire lock to cluster file '%s': %s\n", clusterFileName, err)
							}
						} else {
							log.Printf("ERROR: unable to open cluster file '%s': %s\n", clusterFileName, err)
						}
					}
				*/
				// Now that we have written the new ("next") cluster configuration files to a known location, kick
				// the ONOS wrapper so it will do a HARD restart of ONOS, because ONOS needs a HARD reset in order to
				// come up propperly, silly ONOS
				client := &http.Client{}
				log.Println("INFO: kicking ONOS to the curb")
				if req, err := http.NewRequest("GET", "http://127.0.0.1:4343/reset", nil); err == nil {
					if _, err := client.Do(req); err != nil {
						log.Printf("ERROR: unable to restart ONOS: %s\n", err)
					}
				}
			}
		}

		// Return join message always, not the best form, but lets face it the platform should know
		myIP, err := onos.GetMyIP()
		if err != nil {
			myIP = "0.0.0.0"
		}
		b, err := json.Marshal(map[string]interface{}{
			"command": "peer-join",
			"data": map[string]interface{}{
				"ip": myIP,
			},
		})

		// This is a failsafe. I suppose we could just skip the previous and only do the failsafe, but the above is
		// cleaner
		if err != nil {
			fmt.Println("{\"command\":\"peer-join\",\"data\":{}}")
		} else {
			fmt.Println(string(b))
		}
	}
}
