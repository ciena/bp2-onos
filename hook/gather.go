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
	"log"
	"os"
	"strings"
)

// hasDataSuffix returns true if the given string (s) ends in one of the strings specified
// as a suffix, else returns false
func hasDataSuffix(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// include returns true if the given string starts with one of the prefixes in the list and thus should be included
// in the data reaping
func include(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// ignore returns true if the environment variable is essentially in a black list of
// variables used for configuration and not to be passed to the REST interface
func ignore(s string, blacklist []string) bool {
	for _, bad := range blacklist {
		if s == bad {
			return true
		}
	}
	return false
}

// Config allows the specification of a configuration for gather information from the environment
type Config struct {
	Verbose           bool
	DataSuffixList    []string
	IncludePrefixList []string
	ExcludeList       []string
}

// Gather collects blue planet information form the environment and puts it into a JSON compatible structure
func Gather(config *Config) interface{} {

	flag.Parse()

	if config.Verbose {
		if b, err := json.MarshalIndent(config, "    ", "    "); err != nil {
			log.Printf("ERROR: unable to marshal config to string: %s\n", err)
		} else {
			log.Printf("INFO: reaping blueplanet information from the environment variables with configuration:\n    %s\n",
				string(b))
		}
	}

	env := os.Environ()
	bpData := make(map[string]interface{})
	for _, s := range env {
		// Rather annoying that we get the environment as an array of "=" separated strings given that I really want
		// to turn this into data, so split the string on "=" so we have a key and a value
		nv := strings.SplitN(s, "=", 2)
		if include(nv[0], config.IncludePrefixList) {
			// Before we add this into the data to push to the hook, lets check to see if the environment
			// overrides the target URL. The var to override would be BP_HOOK_URL_REDIRECT_<name>. The name
			// will be the name of the BP hook such as southbound-update, which is also the name of the executable.
			if config.Verbose {
				log.Printf("INFO: processing environment variable '%s'\n", nv[0])
			}
			if !ignore(nv[0], config.ExcludeList) {
				if hasDataSuffix(nv[0], config.DataSuffixList) {
					// If we have a data var then this "should be" string version of a JSON object or array, so lets
					// convert it to a proper JSON structure.
					if config.Verbose {
						log.Printf("INFO: processing value of '%s' as JSON data\n", nv[0])
					}
					var obj interface{}
					if err := json.Unmarshal([]byte(nv[1]), &obj); err != nil {
						// If we can't parse it, then log the warning and set it as a string value
						log.Printf("WARN: unable to unmarshal data value '%s' as JSON, passing vaue as string instead: %s\n", nv[1], err)
						bpData[nv[0]] = nv[1]
					} else {
						bpData[nv[0]] = obj
					}
				} else {
					bpData[nv[0]] = nv[1]
				}
			}
		} else if config.Verbose {
			log.Printf("INFO: ignoring environment variable '%s' because it does not start with prefix '%s'\n",
				nv[0], config.IncludePrefixList)
		}
	}

	return bpData
}
