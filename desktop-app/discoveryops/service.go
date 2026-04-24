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
	ip := discovery.FindServer(port, s.pairingCode, onStatus)
	if ip == "" {
		return ""
	}

	_ = config.SetServerIP(fmt.Sprintf("%s:%d", ip, port))
	return ip
}

func AddressForTest(ip string, port int) string {
	return fmt.Sprintf("%s:%d", ip, port)
}
