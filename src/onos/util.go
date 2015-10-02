package onos

import (
	"net"
	"strings"
)

// GetMyIP uses hueristics and guessing to find a usable IP for the current host by iterating over interfaces and
// addresses assigned to those interfaces until one is found that is "up", not a loopback, nor a broadcast. if no
// address can be found then return an empty string
func GetMyIP() (string, error) {
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

// AlphaOrder interface implementation to sort an arrary of strings alpha-numerically. this is used to help compare two array for
// to see if they are equivalent. essentially we are implementing set methods using an array
type AlphaOrder []string

func (a AlphaOrder) Len() int {
	return len(a)
}

func (a AlphaOrder) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a AlphaOrder) Less(i, j int) bool {
	return strings.Compare(a[i], a[j]) < 0
}

// IPOrder interface implementation to sort an array of strings representing IP addresses. sorting is accomplished by
// comparing octets numberically from left to right. this is used to help generate partition groupings for ONOS's
// cluster configuration. by ordering them each ONOS instance will calculate the same partition groups
type IPOrder []string

func (a IPOrder) Len() int {
	return len(a)
}

func (a IPOrder) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a IPOrder) Less(i, j int) bool {
	// Need to sort IP addresses, we go from left to right sorting octets numerically
	iIP := net.ParseIP(a[i])
	jIP := net.ParseIP(a[j])

	for o := 0; o < len(iIP); o++ {
		if iIP[o] < jIP[o] {
			return true
		} else if iIP[o] > jIP[o] {
			return false
		}
	}

	// all equal is not less
	return false
}
