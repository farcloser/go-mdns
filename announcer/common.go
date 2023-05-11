package announcer

import (
	"net"

	"go.codecomet.dev/core/log"
)

func listIPv4() ([]net.Interface, []net.Addr) {
	list, err := net.Interfaces()
	if err != nil {
		panic(err)
	}

	var addresses []net.Addr

	var interfaces []net.Interface

	for _, iface := range list {
		if (iface.Flags & net.FlagUp) == 0 {
			continue
		}

		if (iface.Flags & net.FlagLoopback) > 0 {
			continue
		}

		if (iface.Flags & net.FlagPointToPoint) > 0 {
			continue
		}

		if (iface.Flags & net.FlagMulticast) == 0 {
			continue
		}

		if (iface.Flags & net.FlagBroadcast) == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			panic(err)
		}

		for _, addr := range addrs {
			if addr.(*net.IPNet).IP.To4() != nil { //nolint:forcetypeassert
				log.Debug().Str("iface name", iface.Name).
					Str("addr", addr.(*net.IPNet).String()). //nolint:forcetypeassert
					Msg("Found eligible interface")

				addresses = append(addresses, addr)
				interfaces = append(interfaces, iface)
			}
		}
	}

	return interfaces, addresses
}
