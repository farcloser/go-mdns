# Farcloser mDNS

mDNS based on third-party go libraries.

See "dev notes".

## Dev

### Makefile

```bash
make lint
make lint-fix
make tidy
```

### Local documentation

```bash
go install golang.org/x/pkgsite/cmd/pkgsite@latest
pkgsite
open http://localhost:8080/go.farcloser.world/mdns
```

### Dev notes

There are a handful of mDNS options out there:

* https://github.com/grandcat/zeroconf
* https://github.com/hashicorp/mdns
* https://github.com/brutella/dnssd

#### Discovery

Hashicorp:
* works kind of ok if there are records for the service name
* for whatever reason, it randomly returns other records as well

Grancat:
* a lot more complex (exponential backoff, timeout)
* unfortunately, it does fail to bring existing services, and only surfaces new entries - reason unclear...

#### Announcer

Hashicorp:
* segfault, or does not persist... - unusable

Goello:
* extreme disaster when multiple interfaces are involved - unusable

Grandcat:
* ok?

#### Forking

hashicorp uses `log` in annoying ways - it is also not doing a good job with ipv6

Forking it for the time being for discovery only, and keeping grandcat for announces
seems like the best option.