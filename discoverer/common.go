package discoverer

import (
	"encoding/json"
	"net"
	"os"
	"path"
	"sync"
	"time"

	"go.farcloser.world/core/filesystem"
	"go.farcloser.world/core/log"
)

const expiration = 600 * time.Second

type ServiceEntry struct {
	Name      string    `json:"name,omitempty"`
	AddrV4    []net.IP  `json:"ip4,omitempty"`
	AddrV6    []net.IP  `json:"ip6,omitempty"`
	Port      int       `json:"port,omitempty"`
	TimeStamp time.Time `json:"timestamp,omitempty"`

	Text    []string `json:"text,omitempty"`
	TTL     uint32   `json:"TTL,omitempty"` //nolint:tagliatelle
	Service string   `json:"service,omitempty"`
	//	Expiration time.Duration `json:"timestamp,omitempty"`
}

type Discoverer struct {
	mu      *sync.Mutex
	Table   map[string]*ServiceEntry
	Storage string
}

func New(location string) *Discoverer {
	dvr := &Discoverer{
		mu:      &sync.Mutex{},
		Table:   map[string]*ServiceEntry{},
		Storage: location,
	}
	if location != "" { //nolint:nestif
		dvr.mu.Lock()
		defer dvr.mu.Unlock()

		fileContent, err := os.ReadFile(location)
		if err != nil {
			log.Warn().Str("location", location).Msg("Failed reading existing cache file. Starting from scratch.")
		} else {
			err = json.Unmarshal(fileContent, &dvr.Table)
			if err != nil {
				log.Error().Str("location", location).Str("content", string(fileContent)).
					Msg("Failed parsing cache content. There will be no persistence.")
			} else {
				// Filter out expired entries
				now := time.Now()
				for k, v := range dvr.Table {
					if now.Sub(v.TimeStamp) > expiration {
						log.Debug().Msgf("Ignoring expired entry %s %s", v.Service, v.Name)
						delete(dvr.Table, k)
					}
				}
			}
		}
	}

	return dvr
}

func (dv *Discoverer) Flush() {
	now := time.Now()
	for k, v := range dv.Table {
		if now.Sub(v.TimeStamp) > expiration {
			log.Debug().Msgf("Deleting expired entry %s %s", v.Service, v.Name)
			delete(dv.Table, k)
		}
	}

	res, _ := json.MarshalIndent(dv.Table, "", "  ") //nolint:errchkjson
	dv.mu.Lock()
	defer dv.mu.Unlock()

	err := os.MkdirAll(path.Dir(dv.Storage), filesystem.DirPermissionsPrivate)
	if err != nil {
		log.Error().Str("location", dv.Storage).Msg("Failed creating directory. No persistence!")

		return
	}

	err = filesystem.WriteFile(dv.Storage, res, filesystem.FilePermissionsPrivate)
	if err != nil {
		log.Error().Str("location", dv.Storage).Msg("Failed writing cache file. No persistence!")
	}
}
