package discovery

import (
	"context"

	"github.com/grandcat/zeroconf"
)

type ZeroconfProvider struct{}

func (z *ZeroconfProvider) Advertise(instanceName, serviceType, domain string, port int, txt []string) (func(), error) {
	server, err := zeroconf.Register(
		instanceName,
		serviceType,
		domain,
		port,
		txt,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return func() {
		server.Shutdown()
	}, nil
}

func (z *ZeroconfProvider) Browse(ctx context.Context, serviceType, domain string, entriesCh chan<- *MDNSServiceEntry) error {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return err
	}

	zcfEntries := make(chan *zeroconf.ServiceEntry)
	go func() {
		defer close(entriesCh)
		for entry := range zcfEntries {
			entriesCh <- &MDNSServiceEntry{
				Instance: entry.Instance,
				Service:  entry.Service,
				Domain:   entry.Domain,
				HostName: entry.HostName,
				Port:     entry.Port,
				Text:     entry.Text,
				AddrIPv4: entry.AddrIPv4,
				AddrIPv6: entry.AddrIPv6,
			}
		}
	}()

	return resolver.Browse(ctx, serviceType, domain, zcfEntries)
}
