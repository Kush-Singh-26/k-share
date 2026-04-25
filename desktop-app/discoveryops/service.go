package discoveryops

import (
	"desktop-app/config"
	"desktop-app/discovery"
	"fmt"
)

type Service struct {
	pairingCode string
}

func New(pairingCode string) *Service {
	return &Service{pairingCode: pairingCode}
}

func (s *Service) SetPairingCode(code string) {
	s.pairingCode = code
}

func (s *Service) Discover(port int, onStatus func(string)) string {
	res := discovery.FindServer(port, s.pairingCode, onStatus)
	if res.IP == "" {
		return ""
	}

	_ = config.SetServerIP(fmt.Sprintf("%s:%d", res.IP, port))
	// If we captured a cert hash, we could store it here too (TOFU)
	if res.CertHash != "" {
		// Log it or store it if we have a way to prompt user later
	}
	return res.IP
}

func AddressForTest(ip string, port int) string {
	return fmt.Sprintf("%s:%d", ip, port)
}
