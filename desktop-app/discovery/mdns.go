package discovery

import (
	"context"
	"fmt"
	"time"

	"github.com/grandcat/zeroconf"
)

// ServiceEntry holds details of a discovered K-Share server.
type ServiceEntry struct {
	Name string
	Host string
	Port int
	IP   string
}

// DiscoverMDNS browses for K-Share servers via mDNS.
// It returns the first service found within the timeout, or an error if none are found.
func DiscoverMDNS(timeout time.Duration) (*ServiceEntry, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create mDNS resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		_ = resolver.Browse(ctx, "_kshare._tcp", "local.", entries)
	}()

	select {
	case entry := <-entries:
		if entry == nil {
			return nil, fmt.Errorf("no mDNS entries found")
		}
		ip := ""
		if len(entry.AddrIPv4) > 0 {
			ip = entry.AddrIPv4[0].String()
		} else if len(entry.AddrIPv6) > 0 {
			ip = entry.AddrIPv6[0].String()
		}
		return &ServiceEntry{
			Name: entry.Instance,
			Host: entry.HostName,
			Port: entry.Port,
			IP:   ip,
		}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("mDNS discovery timed out")
	}
}
