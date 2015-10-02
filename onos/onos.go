package onos

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"syscall"
)

const (
	clusterFileName = "/bp2/hooks/cluster.json" // default location for "next" cluster configuration
	tabletsFileName = "/bp2/hooks/tablets.json" // default location for "next" partition configuration
)

// WriteClusterConfig writes new cluster configuration files based on the addresses specified in the given array, if
// the new addresses differ from the addresses already in the existing configuration files. After the configuration
// files are updated onos is kicked (killed & restarted) so it will accept the new cluster information
func WriteClusterConfig(want []string) error {
	if want == nil || len(want) == 0 {
		return nil
	}
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

	// Calculate the prefix by stripping off the last octet and replacing with a wildcard
	ipPrefix := want[0]
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
	sort.Sort(IPOrder(want))

	// p0 is special and contains all nodes
	p0 := make([]map[string]interface{}, cnt)
	for i := 0; i < cnt; i++ {
		p0[i] = map[string]interface{}{
			"id":      want[i],
			"ip":      want[i],
			"tcpPort": 9876,
		}
	}

	// Make a bunch or partitions (p1 - pn) each containing 3 nodes
	for i := 0; i < cnt; i++ {
		part := make([]map[string]interface{}, 3)

		for j := 0; j < 3; j++ {
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
	return nil
}
