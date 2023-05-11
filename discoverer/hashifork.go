package discoverer

import (
	"fmt"
	"strings"
	"time"

	"go.codecomet.dev/core/log"
	"go.codecomet.dev/mdns/discoverer/hashifork"
)

const maxServiceEntries = 32

func (dv *Discoverer) Discover(stype string, timing time.Duration) {
	// Buffered enough
	entriesCh := make(chan *hashifork.ServiceEntry, maxServiceEntries)

	go func(results <-chan *hashifork.ServiceEntry) {
		for entry := range results {
			if strings.HasSuffix(entry.Name, fmt.Sprintf("%s.local.", stype)) {
				dv.Table[entry.Host] = &ServiceEntry{
					Name:      entry.Name,
					Port:      entry.Port,
					AddrV4:    entry.AddrV4,
					AddrV6:    entry.AddrV6,
					Text:      entry.Text,
					TTL:       entry.TTL,
					TimeStamp: time.Now(),
				}

				log.Debug().Msgf("Seeing %s", entry.Name)
			}
		}
	}(entriesCh)

	params := hashifork.DefaultParams(stype)
	// Workaround https://github.com/hashicorp/mdns/issues/35
	// XXX removed ipv6 support from the fork entirely now
	// params.DisableIPv6 = true
	params.Entries = entriesCh
	params.Timeout = timing

	err := hashifork.Query(params)
	if err != nil {
		log.Error().Err(err).Msg("Something wrong happened")
	}

	close(entriesCh)
}
