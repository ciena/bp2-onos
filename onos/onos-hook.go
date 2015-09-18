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
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unicode"
)

const (
	defaultSuffixList    = "_DATA,_CONFIG"
	suffixListOverride   = "BP_HOOK_DATA_SUFFIX_LIST"
	defaultPrefixList    = "BP_"
	prefixListOverride   = "BP_HOOK_DATA_PREFIX_LIST"
	bluePlanetDataSuffix = "_DATA"
	defaultOnosUser      = "karaf"
	defaultOnosPassword  = "karaf"
	onosUserKey          = "ONOS_USER"
	onosPasswordKey      = "ONOS_PASSWORD"
	peerDataKey          = "BP_HOOK_PEER_DATA"
	peerGroupKey         = "BP_HOOK_PEER_GROUP"
	peerConfigKey        = "BP_HOOK_PEER_CONFIG"
	logFileName          = "/bp2/hooks/cluster.log"
	clusterFileName      = "/bp2/hooks/cluster"
)

var blackList = []string{suffixListOverride, prefixListOverride}
var displayOnly = flag.Bool("n", false, "display REST call, but don't actually make it")
var logToConsole = flag.Bool("c", false, "log to console, not file")

func getMyIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // down
		}
		if iface.Flags&(net.FlagLoopback) != 0 {
			continue // broadcast or loopback
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		var ip net.IP
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() || ip.IsMulticast() {
				continue
			}
			if ip.To4() == nil {
				continue // not v4
			}
			return ip.String(), nil
		}
	}
	return "", nil
}

type stringOrder []string

func (a stringOrder) Len() int {
	return len(a)
}

func (a stringOrder) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a stringOrder) Less(i, j int) bool {
	return strings.Compare(a[i], a[j]) < 0
}

func equal(a, b []string) bool {

	// If not the same length, then not equal, len of nil == 0
	if len(a) != len(b) {
		return false
	}

	sort.Sort(stringOrder(a))
	sort.Sort(stringOrder(b))

	for i := 0; i < len(a); i = i + 1 {
		if strings.Compare(a[i], b[i]) != 0 {
			return false
		}
	}

	return true
}

func main() {

	var logfile *os.File
	if !*logToConsole {
		var err error
		logfile, err = os.OpenFile(logFileName, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
		if err != nil {
			log.Fatalf("ERROR: Unable to open log file: %s\n", err)
		}
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	flag.Parse()

	// Check to see if the data suffix list is overrider by an environment variable and then convert the given list,
	// or the default to a string array for checking against later down the code
	suffixList := os.Getenv(suffixListOverride)
	if suffixList == "" {
		suffixList = defaultSuffixList
	}
	log.Printf("overriding data suffix list to '%s'\n", suffixList)

	// Support both space and comma separated lists
	suffixes := strings.FieldsFunc(suffixList,
		func(r rune) bool { return unicode.IsSpace(r) || r == ',' })

	// Check to see if the data suffix list is overrider by an environment variable and then convert the given list,
	// or the default to a string array for checking against later down the code
	prefixList := os.Getenv(prefixListOverride)
	if prefixList == "" {
		prefixList = defaultPrefixList
	}
	log.Printf("overriding data prefix list to '%s'\n", prefixList)

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
		log.Println("SOUTHBOUND-UPDATE")
	case "heartbeat":
		// Open the cluster file, and if it is not empty then send what is there to ONOS as a cluster Config. If the
		// request to ONOS fails, don't fail this process as it is just ONOS likely in some initialization state and
		// take way to long to get to a point where it will respond
		log.Println("HEARTBEAT")
		var cluster map[string]interface{}
		var want, have []string
		if file, err := os.OpenFile(clusterFileName, os.O_RDONLY, 0644); err == nil {
			defer file.Close()
			fmt.Printf("cluster file open, '%s'\n", clusterFileName)
			if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err == nil {
				defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				fmt.Printf("obtained exclusive lock on cluster file\n")
				if err := json.NewDecoder(file).Decode(&cluster); err == nil {
					if b, err := json.MarshalIndent(cluster, "    ", "    "); err != nil {
						fmt.Printf("decoded information from cluster file:\n%s\n", string(b))
					} else {
						fmt.Printf("decoded information from cluster file\n")
					}
					nodes := cluster["nodes"].([]interface{})
					for _, node := range nodes {
						want = append(want, node.(map[string]interface{})["ip"].(string))
					}
				} else {
					log.Printf("ERROR: failed to decode information from cluster file, ignoring: %s\n", err)
				}
			} else {
				log.Printf("ERROR: failed to obtain exclusive lock on file, ignoring: %s\n", err)
			}
		} else {
			log.Printf("ERROR: failed to open cluster information file: %s\n", err)
		}

		if len(want) == 0 {
			log.Println("no information about desired cluster state, no update to ONOS being executed")
			return
		}

		log.Printf("desired cluster information detected, attempting to update ONOS: %v\n", want)

		// Attempt to get from ONOS what is current set as the cluster information, so that we will only update
		// if there is a change
		onosUser := os.Getenv(onosUserKey)
		if onosUser == "" {
			onosUser = defaultOnosUser
		}
		onosPassword := os.Getenv(onosPasswordKey)
		if onosPassword == "" {
			onosPassword = defaultOnosPassword
		}
		log.Printf("authenticating to ONOS with '%s:%s\n", onosUser, onosPassword)

		client := &http.Client{}
		log.Printf("fetching current cluster information from ONOS\n")
		if req, err := http.NewRequest("GET", "http://localhost:8181/onos/v1/cluster", nil); err == nil {
			req.SetBasicAuth(onosUser, onosPassword)
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				if int(resp.StatusCode/100) == 2 {
					log.Printf("received successful HTTP response from ONOS: %s\n", resp.Status)
					var current map[string]interface{}
					err = json.NewDecoder(resp.Body).Decode(&current)
					if err != nil {
						log.Printf("ERROR: failed to decode response form ONOS, ignoring: %s\n", err)
					} else {
						for _, node := range current["nodes"].([]interface{}) {
							have = append(have, node.(map[string]interface{})["ip"].(string))
						}
					}
				} else {
					log.Printf("ERROR: received non-success response from ONOS, ignoring: %s\n", resp.Status)
				}
			} else {
				log.Printf("ERROR: failed to retrieve current cluster information from ONOS, ignoring: %s\n", err)
			}
		} else {
			log.Printf("ERROR: failed to build request to ONOS for current cluster information, ignoring: %s\n", err)
		}

		// If we have nothing then ONOS is not responding
		if len(have) == 0 {
			log.Println("unable to retrieve current cluster information from ONOS, no update action to ONOS")
			return
		}
		log.Printf("current cluster information from ONOS: %v\n", have)

		// If want and have are not equal then we need to update ONOS
		if equal(want, have) {
			log.Println("current and desired cluster information are identical, no update action to ONOS")
		} else {
			// Because it is possible / likely that ONOS is not up and running yet and thus will not respond to
			// REST request
			log.Println("attempting to update ONOS cluster information via REST")

			if data, err := json.Marshal(cluster); err != nil {
				log.Printf("ERROR: unable to build encode information, ONOS not updated: %s\n", err)
			} else {
				dataReader := strings.NewReader(string(data))
				if req, err := http.NewRequest("POST", "http://localhost:8181/onos/v1/cluster/configuration", dataReader); err == nil {
					req.SetBasicAuth(onosUser, onosPassword)
					if resp, err := (&http.Client{}).Do(req); err != nil {
						log.Printf("ERROR: HTTP request to update onos failed: %s\n", err)
					} else {
						if int(resp.StatusCode/100) == 2 {
							log.Printf("ONOS successfully updated with desired cluster information: %s\n", resp.Status)
							// TODO erase cluster file so we don't keep retrying if we don't have an update
						} else {
							log.Printf("ERROR: ONOS not updated with desired cluster information: %s\n", resp.Status)
						}
					}
				} else {
					log.Printf("ERROR: failed to build HTTP request to update ONOS: %s\n", err)
				}
			}
		}
	case "peer-status":
		fallthrough
	case "peer-update":
		log.Println("PEER-STATUS | PEER-UPDATE")

		if peerData, ok := bpData.(map[string]interface{})[peerDataKey].(map[string]interface{}); !ok {
			log.Printf("no peer information received from platform, no action taken")
		} else {
			var want []string

			// We want to verify the the new peering information, if it is included in the message, against the existing
			// cluster information in the ONOS configuration. If it has changed we will need to update ONOS.
			if b, err := json.MarshalIndent(peerData, "    ", "    "); err == nil {
				log.Printf("received peer data from the platform:\n%s\n", string(b))
			} else {
				log.Printf("ERROR: unable to decode peer data from platform, curious: %s\n", err)
			}

			// TODO peerConfig := bpData.(map[string]interface{})[peerConfigKey].(map[string]interface{})
			peers := peerData["peers"].(map[string]interface{})

			// If the data set does not contain any peers then we will skip this data set
			if len(peers) == 0 {
				log.Printf("empty peering list from platform, no update action taken")
			} else {
				peerLeader := strconv.Itoa(int(peerData["leader"].(float64)))
				log.Printf("peer leader ID is: %s\n", peerLeader)

				myIP, err := getMyIP()
				if err == nil {
					want = append(want, myIP)
					log.Printf("append own IP '%s' to desired cluster list\n", myIP)
				} else {
					log.Println("unable to determine own ID, unable to add it to desired cluster list")
				}
				for _, peer := range peers {
					// If the IP of the peer is not "me" then add it to the list. We always add ourselves and we don't
					// wanted it added twice
					ip := peer.(map[string]interface{})["ip"].(string)
					if myIP != ip {
						log.Printf("append peer with IP '%s' to desired cluster list\n", ip)
						want = append(want, ip)
					}
				}

				// for _, peerID := range peerConfig["app_instances"].([]interface{}) {
				// 	if peer, ok := peers[strconv.Itoa(int(peerID.(float64)))]; ok {
				// 		want = append(want, peer.(map[string]interface{})["ip"].(string))
				// 		log.Printf("appending peer '%s' with IP '%s' to desired cluster list\n",
				// 			strconv.Itoa(int(peerID.(float64))),
				// 			peer.(map[string]interface{})["ip"].(string))
				// 	} else {
				// 		log.
				// 	}
				// }

				// Because it is possible / likely that ONOS is not up and running yet and thus will not respond to
				// REST request, we will write the update to a file that will be picked up during a heartbeat message
				// and at that point an attempt will be made to update ONOS ... what a pain

				// Construct ONOS JSON payload for cluster update
				cluster := make(map[string]interface{})
				var nodes []interface{}
				for _, ip := range want {
					node := make(map[string]string)
					node["ip"] = ip
					nodes = append(nodes, node)
				}
				cluster["nodes"] = nodes
				leader := peerData["peers"].(map[string]interface{})[peerLeader].(map[string]interface{})

				// Calculate the prefix by stripping off the last octet and replacing with a wildcard
				ipPrefix := leader["ip"].(string)
				idx := strings.LastIndex(ipPrefix, ".")
				ipPrefix = ipPrefix[:idx] + ".*"
				cluster["ipPrefix"] = ipPrefix

				if data, err := json.Marshal(cluster); err != nil {
					log.Printf("ERROR: Unable to encode cluster information to write to update file, no file written: %s\n", err)
				} else {
					if b, err := json.MarshalIndent(cluster, "    ", "    "); err == nil {
						log.Printf("writting ONOS cluster information to cluster file '%s'\n%s\n",
							clusterFileName, string(b))
					}
					// Open / Create the file with an exclusive lock (only one person can handle this at a time)
					if file, err := os.OpenFile(clusterFileName, os.O_RDWR|os.O_CREATE, 0644); err == nil {
						defer file.Close()
						if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err == nil {
							defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
							if _, err := file.Write(data); err != nil {
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
			}
		}

		// Return join message always, not the best form, but lets face it the platform should know
		myIP, err := getMyIP()
		if err != nil {
			myIP = "0.0.0.0"
		}
		b, err := json.Marshal(map[string]interface{}{
			"command": "peer-join",
			"data": map[string]interface{}{
				"ip": myIP,
			},
		})

		if err != nil {
			fmt.Println("{\"command\":\"peer-join\",\"data\":{}}")
		} else {
			fmt.Println(string(b))
		}
	}
}
