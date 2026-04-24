package discovery

import (
	"context"
	"crypto/tls"
	"desktop-app/config"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	WorkerCount      = 255 // Fast parallel scan for laptops
	ConnectTimeoutMs = 350 // Balance between speed and reliability on slow networks
)

// FindServer scans for the server on local network
func FindServer(port int, pairingCode string, onStatus func(string)) string {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 0. Try mDNS first
	onStatus("Searching via mDNS...")
	if entry, err := DiscoverMDNS(3 * time.Second); err == nil {
		if checkIP(ctx, entry.IP, port, pairingCode) {
			return entry.IP
		}
	}

	// 1. Check Zone 0: Localhost
	onStatus("Scanning Zone 0 (Localhost)...")
	if checkIP(ctx, "127.0.0.1", port, pairingCode) {
		return "127.0.0.1"
	}

	// 2. Identify Local Interfaces and Subnets
	interfaces, err := net.Interfaces()
	if err != nil {
		onStatus("Network error")
		return ""
	}

	type subnetInfo struct {
		ip     string
		subnet string // e.g., "192.168.1"
	}
	var subnets []subnetInfo

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			ipStr := ip.String()
			parts := strings.Split(ipStr, ".")
			if len(parts) == 4 {
				subnet := fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
				subnets = append(subnets, subnetInfo{ip: ipStr, subnet: subnet})
			}
		}
	}

	if len(subnets) == 0 {
		onStatus("No network found")
		return ""
	}

	// 3. Check Zone Cache (Saved Networks)
	onStatus("Checking saved networks...")
	for _, sn := range subnets {
		if savedIP, ok := config.Current.SavedNetworks[sn.subnet]; ok {
			if checkIP(ctx, savedIP, port, pairingCode) {
				return savedIP
			}
		}
	}

	// 4. Zone 1: Scan /24 of all active interfaces
	ipsToScan := make([]string, 0)
	for _, sn := range subnets {
		onStatus(fmt.Sprintf("Scanning %s.1-255...", sn.subnet))
		for i := 1; i <= 254; i++ {
			ipsToScan = append(ipsToScan, fmt.Sprintf("%s.%d", sn.subnet, i))
		}
	}

	// Shuffle IPs to avoid rate limiting (Go 1.26.1+ behavior)
	rand.Shuffle(len(ipsToScan), func(i, j int) {
		ipsToScan[i], ipsToScan[j] = ipsToScan[j], ipsToScan[i]
	})

	// Parallel Scan
	foundIP := make(chan string)
	var wg sync.WaitGroup
	jobs := make(chan string, len(ipsToScan))

	// Worker Pool
	for w := 0; w < WorkerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{
				Timeout: ConnectTimeoutMs * time.Millisecond,
			}

			// Context-aware select to prevent goroutine leak
			for {
				select {
				case <-ctx.Done():
					return
				case ip, ok := <-jobs:
					if !ok {
						return
					}
					if checkTarget(ctx, client, ip, port, pairingCode) {
						select {
						case foundIP <- ip:
						case <-ctx.Done():
						}
						// Trigger cancel to stop other workers
						cancel()
						return
					}
				}
			}
		}()
	}

	// Feed jobs
	go func() {
		for _, ip := range ipsToScan {
			select {
			case <-ctx.Done():
				close(jobs)
				return
			case jobs <- ip:
			}
		}
		close(jobs)
	}()

	// Wait for result or exhaustive search
	go func() {
		wg.Wait()
		close(foundIP)
	}()

	select {
	case ip := <-foundIP:
		if ip != "" {
			return ip
		}
	case <-ctx.Done():
		// If context cancelled (found), drain channel if needed
		select {
		case ip := <-foundIP:
			return ip
		default:
		}
	}

	onStatus("Server not found")
	return ""
}

// checkIP checks a single IP synchronously
func checkIP(ctx context.Context, ip string, port int, pairingCode string) bool {
	client := &http.Client{
		Timeout: ConnectTimeoutMs * time.Millisecond,
	}
	return checkTarget(ctx, client, ip, port, pairingCode)
}

func checkTarget(ctx context.Context, client *http.Client, ip string, port int, pairingCode string) bool {
	// Configure client for HTTPS with self-signed certs if not already configured
	if transport, ok := client.Transport.(*http.Transport); !ok || transport == nil {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client.Transport = tr
	} else if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	// HTTPS Application Ping (Verify)
	url := fmt.Sprintf("https://%s:%d/ping", ip, port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	return true
}
