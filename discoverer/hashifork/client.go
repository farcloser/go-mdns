package hashifork

// Forked from: https://github.com/hashicorp/mdns
// Under MIT License: https://github.com/hashicorp/mdns/blob/master/LICENSE

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"go.codecomet.dev/core/log"
	"golang.org/x/net/ipv4"
)

var (
	errFailedToBindMulticast = errors.New("failed to bind to any multicast udp port")
	errFailedToBindUnicast   = errors.New("failed to bind to any unicast udp port")
)

const bufferSize = 65536

const (
	ipv4mdns = "224.0.0.251"
	mdnsPort = 5353
)

var ipv4Addr = &net.UDPAddr{ //nolint:gochecknoglobals
	IP:   net.ParseIP(ipv4mdns),
	Port: mdnsPort,
}

type ServiceEntry struct {
	Host      string    `json:"host,omitempty"`
	Name      string    `json:"name,omitempty"`
	AddrV4    []net.IP  `json:"addv4,omitempty"`
	AddrV6    []net.IP  `json:"addv6,omitempty"`
	Port      int       `json:"port,omitempty"`
	TimeStamp time.Time `json:"timestamp,omitempty"`

	Text    []string `json:"text,omitempty"`
	TTL     uint32   `json:"TTL,omitempty"` //nolint:tagliatelle
	Service string   `json:"service,omitempty"`

	//	Expiration time.Duration `json:"timestamp,omitempty"`
	hasTXT bool

	sent bool
}

// complete is used to check if we have all the info we need.
func (s *ServiceEntry) complete() bool {
	return (s.AddrV4 != nil || s.AddrV6 != nil) && s.Port != 0 && s.hasTXT
}

// QueryParam is used to customize how a Lookup is performed.
type QueryParam struct {
	Service             string               // Service to lookup
	Domain              string               // Lookup domain, default "local"
	Timeout             time.Duration        // Lookup timeout, default 1 second
	Interface           *net.Interface       // Multicast interface to use
	Entries             chan<- *ServiceEntry // Entries Channel
	WantUnicastResponse bool                 // Unicast response desired, as per 5.4 in RFC
}

// DefaultParams is used to return a default set of QueryParam's.
func DefaultParams(service string) *QueryParam {
	return &QueryParam{
		Service:             service,
		Domain:              "local",
		Timeout:             time.Second,
		Entries:             make(chan *ServiceEntry),
		WantUnicastResponse: false,
	}
}

// Query looks up a given service, in a domain, waiting at most
// for a timeout before finishing the query. The results are streamed
// to a channel. Sends will not block, so clients should make sure to
// either read or buffer.
func Query(params *QueryParam) error {
	// Create a new client
	client, err := newClient()
	if err != nil {
		return err
	}
	defer client.Close()

	// Set the multicast interface
	if params.Interface != nil {
		if err := client.setInterface(params.Interface); err != nil {
			return err
		}
	}

	// Ensure defaults are set
	if params.Domain == "" {
		params.Domain = "local"
	}

	if params.Timeout == 0 {
		params.Timeout = time.Second
	}

	// Run the query
	return client.query(params)
}

// Client provides a query interface that can be used to
// search for service providers using mDNS.
type client struct {
	ipv4UnicastConn   *net.UDPConn
	ipv4MulticastConn *net.UDPConn

	closed   int32
	closedCh chan struct{}
}

// NewClient creates a new mdns Client that can be used to query
// for records.
func newClient() (*client, error) {
	//nolint:godox
	// TODO(reddaly): At least attempt to bind to the port required in the spec.
	// Create a IPv4 listener
	var uconn4 *net.UDPConn

	var mconn4 *net.UDPConn

	var err error

	uconn4, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Error().Err(err).Msgf("mdns: Failed to bind to udp4 port: %v", err)
	}

	if uconn4 == nil {
		return nil, errFailedToBindUnicast
	}

	mconn4, err = net.ListenMulticastUDP("udp4", nil, ipv4Addr)
	if err != nil {
		log.Error().Err(err).Msgf("mdns: Failed to bind to udp4 port: %v", err)
	}

	if mconn4 == nil {
		return nil, errFailedToBindMulticast
	}

	cli := &client{
		ipv4MulticastConn: mconn4,
		ipv4UnicastConn:   uconn4,
		closedCh:          make(chan struct{}),
	}

	return cli, nil
}

// Close is used to cleanup the client.
func (c *client) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		// something else already closed it
		return nil
	}

	log.Debug().Msgf("mdns: Closing client %v", *c)
	close(c.closedCh)

	if c.ipv4UnicastConn != nil {
		c.ipv4UnicastConn.Close()
	}

	if c.ipv4MulticastConn != nil {
		c.ipv4MulticastConn.Close()
	}

	return nil
}

// setInterface is used to set the query interface, uses system
// default if not provided.
func (c *client) setInterface(iface *net.Interface) error {
	packConn := ipv4.NewPacketConn(c.ipv4UnicastConn)
	if err := packConn.SetMulticastInterface(iface); err != nil {
		return err
	}

	packConn = ipv4.NewPacketConn(c.ipv4MulticastConn)
	if err := packConn.SetMulticastInterface(iface); err != nil {
		return err
	}

	return nil
}

// query is used to perform a lookup and stream results.
func (c *client) query(params *QueryParam) error { //nolint:gocognit
	ctx := context.TODO()
	// Create the service name
	serviceAddr := fmt.Sprintf("%s.%s.", strings.Trim(params.Service, "."), strings.Trim(params.Domain, "."))

	// Start listening for response packets
	msgCh := make(chan *dns.Msg, 32) //nolint:gomnd
	go c.recv(ctx, c.ipv4UnicastConn, msgCh)
	go c.recv(ctx, c.ipv4MulticastConn, msgCh)

	// Send the query
	mess := new(dns.Msg)
	mess.SetQuestion(serviceAddr, dns.TypePTR)
	// RFC 6762, section 18.12.  Repurposing of Top Bit of qclass in Question
	// Section
	//
	// In the Question Section of a Multicast DNS query, the top bit of the qclass
	// field is used to indicate that unicast responses are preferred for this
	// particular question.  (See Section 5.4.)
	if params.WantUnicastResponse {
		mess.Question[0].Qclass |= 1 << 15 //nolint:gomnd
	}

	mess.RecursionDesired = false
	if err := c.sendQuery(mess); err != nil {
		return err
	}

	// Map the in-progress responses
	inprogress := make(map[string]*ServiceEntry)

	// Listen until we reach the timeout
	finish := time.After(params.Timeout)

	for {
		select {
		case resp := <-msgCh:
			var inp *ServiceEntry

			for _, answer := range append(resp.Answer, resp.Extra...) {
				//nolint:godox
				// TODO(reddaly): Check that response corresponds to serviceAddr?
				switch rrRecord := answer.(type) {
				case *dns.PTR:
					// Create new entry for this
					inp = attachServiceEntry(inprogress, rrRecord.Ptr)
					inprogress[rrRecord.Ptr].TTL = rrRecord.Hdr.Ttl

				case *dns.SRV:
					// Check for a target mismatch
					if rrRecord.Target != rrRecord.Hdr.Name {
						// Alias
						inprogress[rrRecord.Target] = attachServiceEntry(inprogress, rrRecord.Hdr.Name)
					}

					// Get the port
					inp = attachServiceEntry(inprogress, rrRecord.Hdr.Name)
					inp.Host = rrRecord.Target
					inp.Port = int(rrRecord.Port)

				case *dns.TXT:
					// Pull out the txt
					inp = attachServiceEntry(inprogress, rrRecord.Hdr.Name)
					inp.Text = rrRecord.Txt
					inp.hasTXT = true

				case *dns.A:
					// Pull out the IP
					inp = attachServiceEntry(inprogress, rrRecord.Hdr.Name)
					inp.AddrV4 = append(inp.AddrV4, rrRecord.A)

				case *dns.AAAA:
					// Pull out the IP
					inp = attachServiceEntry(inprogress, rrRecord.Hdr.Name)
					inp.AddrV6 = append(inp.AddrV6, rrRecord.AAAA)
				}
			}

			if inp == nil {
				continue
			}

			// Check if this entry is complete
			if inp.complete() {
				if inp.sent {
					continue
				}

				inp.sent = true
				select {
				case params.Entries <- inp:
				default:
				}
			} else {
				// Fire off a node specific query
				m := new(dns.Msg)
				m.SetQuestion(inp.Name, dns.TypePTR)
				m.RecursionDesired = false
				if err := c.sendQuery(m); err != nil {
					log.Error().Err(err).Msgf("mdns: Failed to query instance %s: %v", inp.Name, err)
				}
			}
		case <-finish:
			return nil
		}
	}
}

// sendQuery is used to multicast a query out.
func (c *client) sendQuery(q *dns.Msg) error {
	buf, err := q.Pack()
	if err != nil {
		return err
	}

	if c.ipv4UnicastConn != nil {
		_, err = c.ipv4UnicastConn.WriteToUDP(buf, ipv4Addr)
		if err != nil {
			return err
		}
	}

	return nil
}

// Data receiving routine reads from connection, unpacks packets into dns.Msg
// structures and sends them to a given msgCh channel.
func (c *client) recv(ctx context.Context, l *net.UDPConn, msgCh chan *dns.Msg) {
	buf := make([]byte, bufferSize)
	for atomic.LoadInt32(&c.closed) == 0 {
		bytesReceived, err := l.Read(buf)

		if atomic.LoadInt32(&c.closed) == 1 {
			return
		}

		if err != nil {
			log.Error().Err(err).Msg("mdns: Failed to read packet")

			return
		}

		msg := new(dns.Msg)
		if err := msg.Unpack(buf[:bytesReceived]); err != nil {
			log.Error().Err(err).Msg("mdns: Failed to unpack packet")
			log.Debug().Str("packet", string(buf[:bytesReceived]))

			continue
		}
		select {
		case msgCh <- msg:
			// Submit decoded DNS message and continue.
		case <-ctx.Done():
			// Abort.
			return
		}
	}
}

// attachServiceEntry is used to ensure the named node is in progress.
func attachServiceEntry(inprogress map[string]*ServiceEntry, name string) *ServiceEntry {
	if _, ok := inprogress[name]; !ok {
		inprogress[name] = &ServiceEntry{
			Name: name,
		}
	}

	return inprogress[name]
}
