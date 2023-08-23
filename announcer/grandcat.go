package announcer

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/grandcat/zeroconf"
	"go.farcloser.world/core/log"
)

func Announce(name string, stype string, host string, port int, txt []string) {
	log.Debug().
		Str("Name", name).
		Str("Type", stype).
		Str("Host", host).
		Str("Port", strconv.Itoa(port)).
		Strs("Txt", txt).
		Msg("Going to announce")
	// server, err := zeroconf.Register(name, stype, "local.", port, txt, nil)
	interfaces, _ := listIPv4()

	server, err := zeroconf.RegisterProxy(name, stype, "local.", port, host, nil, txt, interfaces)
	if err != nil {
		panic(err)
	}

	defer server.Shutdown()

	// Clean exit.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select { //nolint:gosimple
	// Exit by user
	case <-sig:
	}
	// case <-time.After(time.Second * 120):
	// Exit by timeout

	log.Debug().Msg("Shutting down.")
}
