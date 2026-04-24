package trustops

import (
	"desktop-app/crypto"
	"errors"
	"sync"
)

var ErrTrustRequired = errors.New("trust required")

type Service struct {
	pairingCode   string
	pendingCert   *crypto.CertInfo
	pendingServer string
	pendingRole   string
	mu            sync.RWMutex
}

func New(pairingCode string) *Service {
	return &Service{pairingCode: pairingCode}
}

func (s *Service) SetPairingCode(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairingCode = code
}

func (s *Service) Pending() (*crypto.CertInfo, string, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pendingCert == nil {
		return nil, "", "", false
	}
	return s.pendingCert, s.pendingServer, s.pendingRole, true
}

func (s *Service) CancelPending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingCert = nil
	s.pendingServer = ""
	s.pendingRole = ""
	crypto.Manager.ClearPending()
}

func (s *Service) TrustPending() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingCert == nil {
		return
	}

	crypto.Manager.TrustCertificate(s.pendingCert.Hash, s.pendingServer, "K-Share Server", s.pairingCode)
	s.pendingCert = nil
	s.pendingServer = ""
	s.pendingRole = ""
}

func (s *Service) Check(serverIP, role string) error {
	certInfo := crypto.Manager.GetLastSeenCert()
	if certInfo != nil && !crypto.Manager.IsTrusted(certInfo.Hash) {
		s.mu.Lock()
		s.pendingCert = certInfo
		s.pendingServer = serverIP
		s.pendingRole = role
		s.mu.Unlock()
		
		crypto.Manager.SetPendingCert(certInfo, serverIP)
		return ErrTrustRequired
	}
	return nil
}
