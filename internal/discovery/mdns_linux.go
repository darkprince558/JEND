//go:build linux

package discovery

import (
	"context"
	"net"

	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
)

type AvahiProvider struct {
	fallback *ZeroconfProvider
}

func init() {
	// Try connecting to Avahi System Bus. If it fails (e.g., Avahi isn't running),
	// we fall back to the generic zeroconf provider.
	conn, err := dbus.SystemBus()
	if err == nil {
		server, err := avahi.ServerNew(conn)
		if err == nil && server != nil {
			DefaultMDNSProvider = &AvahiProvider{
				fallback: &ZeroconfProvider{},
			}
			return
		}
	}

	// Fallback to Zeroconf
	DefaultMDNSProvider = &ZeroconfProvider{}
}

func (a *AvahiProvider) Advertise(instanceName, serviceType, domain string, port int, txt []string) (func(), error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return a.fallback.Advertise(instanceName, serviceType, domain, port, txt)
	}

	server, err := avahi.ServerNew(conn)
	if err != nil {
		return a.fallback.Advertise(instanceName, serviceType, domain, port, txt)
	}

	eg, err := server.EntryGroupNew()
	if err != nil {
		return a.fallback.Advertise(instanceName, serviceType, domain, port, txt)
	}

	// Format text records for Avahi
	var txtRecords [][]byte
	for _, t := range txt {
		txtRecords = append(txtRecords, []byte(t))
	}

	err = eg.AddService(
		avahi.InterfaceUnspec,
		avahi.ProtoUnspec,
		0,
		instanceName,
		serviceType,
		domain,
		"", // host
		uint16(port),
		txtRecords,
	)
	if err != nil {
		server.EntryGroupFree(eg)
		return a.fallback.Advertise(instanceName, serviceType, domain, port, txt)
	}

	err = eg.Commit()
	if err != nil {
		server.EntryGroupFree(eg)
		return a.fallback.Advertise(instanceName, serviceType, domain, port, txt)
	}

	return func() {
		_ = eg.Reset()
		server.EntryGroupFree(eg)
		conn.Close()
	}, nil
}

func (a *AvahiProvider) Browse(ctx context.Context, serviceType, domain string, entriesCh chan<- *MDNSServiceEntry) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return a.fallback.Browse(ctx, serviceType, domain, entriesCh)
	}

	server, err := avahi.ServerNew(conn)
	if err != nil {
		return a.fallback.Browse(ctx, serviceType, domain, entriesCh)
	}

	browser, err := server.ServiceBrowserNew(avahi.InterfaceUnspec, avahi.ProtoUnspec, serviceType, domain, 0)
	if err != nil {
		return a.fallback.Browse(ctx, serviceType, domain, entriesCh)
	}

	go func() {
		defer close(entriesCh)
		defer server.ServiceBrowserFree(browser)
		defer conn.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case service := <-browser.AddChannel:
				// Resolve the service to get IP and TXT
				resolved, err := server.ResolveService(
					service.Interface,
					service.Protocol,
					service.Name,
					service.Type,
					service.Domain,
					avahi.ProtoUnspec,
					0,
				)
				if err != nil {
					continue
				}

				entry := &MDNSServiceEntry{
					Instance: resolved.Name,
					Service:  resolved.Type,
					Domain:   resolved.Domain,
					HostName: resolved.Host,
					Port:     int(resolved.Port),
				}

				// Convert IP address
				ip := net.ParseIP(resolved.Address)
				if ip != nil {
					if ip.To4() != nil {
						entry.AddrIPv4 = []net.IP{ip}
					} else {
						entry.AddrIPv6 = []net.IP{ip}
					}
				}

				// Convert TXT records
				for _, t := range resolved.Txt {
					entry.Text = append(entry.Text, string(t))
				}

				select {
				case <-ctx.Done():
					return
				case entriesCh <- entry:
				}
			}
		}
	}()

	return nil
}
