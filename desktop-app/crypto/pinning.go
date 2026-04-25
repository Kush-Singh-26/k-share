package crypto

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"desktop-app/config"
	"encoding/hex"
	"errors"
	"sync"
)

var ErrCertificateNotTrusted = errors.New("certificate not trusted")

// CertInfo holds information about a server certificate
type CertInfo struct {
	Hash        string
	Subject     string
	Issuer      string
	NotBefore   string
	NotAfter    string
	Fingerprint string
}

// PinningManager handles TOFU (Trust On First Use) certificate pinning
type PinningManager struct {
	mu            sync.RWMutex
	lastSeenCert  *CertInfo
	pendingCert   *CertInfo
	currentServer string
}

var Manager = &PinningManager{}

// GetCertHash computes SHA-256 hash of a certificate
func GetCertHash(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// GetCertInfo extracts display info from a certificate
func GetCertInfo(cert *x509.Certificate) *CertInfo {
	hash := GetCertHash(cert)
	return &CertInfo{
		Hash:        hash,
		Subject:     cert.Subject.CommonName,
		Issuer:      cert.Issuer.CommonName,
		NotBefore:   cert.NotBefore.Format("2006-01-02"),
		NotAfter:    cert.NotAfter.Format("2006-01-02"),
		Fingerprint: formatFingerprint(hash),
	}
}

func formatFingerprint(hash string) string {
	// Format as XX:XX:XX:XX... (first 32 chars = 16 bytes)
	if len(hash) < 32 {
		return hash
	}
	result := ""
	for i := 0; i < 32; i += 2 {
		if i > 0 {
			result += ":"
		}
		result += hash[i : i+2]
	}
	return result
}

// GetLastSeenCert returns the last certificate seen during connection
func (pm *PinningManager) GetLastSeenCert() *CertInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.lastSeenCert
}

// SetPendingCert stores a cert awaiting user approval
func (pm *PinningManager) SetPendingCert(cert *CertInfo, server string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.pendingCert = cert
	pm.currentServer = server
}

// GetPendingCert returns cert awaiting approval
func (pm *PinningManager) GetPendingCert() (*CertInfo, string) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.pendingCert, pm.currentServer
}

// ClearPending clears the pending certificate
func (pm *PinningManager) ClearPending() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.pendingCert = nil
	pm.currentServer = ""
}

// TrustCertificate saves a certificate hash as trusted for a server
func (pm *PinningManager) TrustCertificate(certHash, serverIP, displayName, authCode string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	_ = config.SetKnownServer(certHash, config.ServerIdentity{
		CertHash:    certHash,
		AuthCode:    authCode,
		LastIP:      serverIP,
		DisplayName: displayName,
	})
	pm.pendingCert = nil
}

// IsTrusted checks if a certificate hash is in the known servers list
func (pm *PinningManager) IsTrusted(certHash string) bool {
	return config.IsServerKnown(certHash)
}

// RemoveTrust removes a server from trusted list
func (pm *PinningManager) RemoveTrust(certHash string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	_ = config.RemoveKnownServer(certHash)
}

// CreateTLSConfig creates a TLS config with certificate capture
// It allows connection but captures the cert for verification
func CreateTLSConfig(serverName string, onCertSeen func(*CertInfo)) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // We do our own TOFU verification
		ServerName:         serverName,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("no certificates presented")
			}

			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}

			certInfo := GetCertInfo(cert)
			Manager.mu.Lock()
			Manager.lastSeenCert = certInfo
			Manager.mu.Unlock()

			if onCertSeen != nil {
				onCertSeen(certInfo)
				return nil
			}

			// If no capture callback, enforce trust
			if !Manager.IsTrusted(certInfo.Hash) {
				return ErrCertificateNotTrusted
			}

			return nil
		},
	}
}
