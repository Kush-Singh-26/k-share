package session

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"

	"desktop-app/api"
	"desktop-app/config"
	"desktop-app/crypto"
	"desktop-app/discoveryops"
	"desktop-app/trustops"
)

var ErrTrustRequired = trustops.ErrTrustRequired

type APIClient interface {
	Ping(context.Context) (string, error)
	SetServerIP(string)
	SetAuthCode(string)
}

type WSClient interface {
	Connect() error
	SetServerIP(string)
	SetAuthCode(string)
	Close()
}

type TrustManager interface {
	SetPairingCode(string)
	Check(serverIP, role string) error
	Pending() (*crypto.CertInfo, string, string, bool)
	CancelPending()
	TrustPending()
}

type Discoverer interface {
	SetPairingCode(string)
	Discover(port int, onStatus func(string)) string
}

type Manager struct {
	apiClient   APIClient
	wsClient    WSClient
	apiConcrete *api.Client
	wsConcrete  *api.WSClient
	trust       TrustManager
	discover    Discoverer

	serverIP    string
	pairingCode string

	isGuest          bool
	clipboardChannel string
	lastImageHash    string
}

func New(serverIP, pairingCode string) *Manager {
	m := &Manager{}
	m.apiConcrete = api.NewClient(serverIP, pairingCode)
	m.wsConcrete = api.NewWSClient(serverIP, pairingCode)
	m.apiClient = m.apiConcrete
	m.wsClient = m.wsConcrete
	m.trust = trustops.New(pairingCode)
	m.discover = discoveryops.New(pairingCode)
	m.serverIP = serverIP
	m.pairingCode = pairingCode
	return m
}

func NewWithDependencies(apiClient APIClient, wsClient WSClient, trust TrustManager, discover Discoverer, serverIP, pairingCode string) *Manager {
	return &Manager{
		apiClient:        apiClient,
		wsClient:         wsClient,
		trust:            trust,
		discover:         discover,
		serverIP:         serverIP,
		pairingCode:      pairingCode,
		clipboardChannel: "",
	}
}

func (m *Manager) APIClient() *api.Client {
	return m.apiConcrete
}

func (m *Manager) WSClient() *api.WSClient {
	return m.wsConcrete
}

func (m *Manager) SetServerIP(ip string) {
	m.serverIP = ip
	m.apiClient.SetServerIP(ip)
	m.wsClient.SetServerIP(ip)
}

func (m *Manager) SetPairingCode(code string) {
	m.pairingCode = code
	m.apiClient.SetAuthCode(code)
	m.wsClient.SetAuthCode(code)
	m.trust.SetPairingCode(code)
	m.discover.SetPairingCode(code)
}

func (m *Manager) ServerIP() string {
	return m.serverIP
}

func (m *Manager) PairingCode() string {
	return m.pairingCode
}

func (m *Manager) IsGuest() bool {
	return m.isGuest
}

func (m *Manager) ClipboardChannel() string {
	return m.clipboardChannel
}

func (m *Manager) LastImageHash() string {
	return m.lastImageHash
}

func (m *Manager) SetLastImageHash(hash string) {
	m.lastImageHash = hash
}

func (m *Manager) PendingTrust() (*crypto.CertInfo, string, string, bool) {
	return m.trust.Pending()
}

func (m *Manager) CancelPendingTrust() {
	m.trust.CancelPending()
}

func (m *Manager) TrustPending() {
	m.trust.TrustPending()
}

func (m *Manager) Connect() (string, error) {
	role, err := m.apiClient.Ping(context.TODO())
	if err != nil {
		return "", err
	}

	if err := m.trust.Check(m.serverIP, role); err != nil {
		return "", err
	}

	return role, nil
}

func (m *Manager) CompleteConnection(role string) {
	m.isGuest = role == "guest"
	if m.isGuest {
		m.clipboardChannel = "guest"
	} else {
		m.clipboardChannel = ""
	}

	if err := m.wsClient.Connect(); err != nil {
		log.Printf("WebSocket connect failed: %v", err)
	}

	host, _, err := net.SplitHostPort(m.serverIP)
	if err != nil {
		host = m.serverIP
	}
	if host != "" && host != "localhost" && host != "127.0.0.1" {
		parts := strings.Split(host, ".")
		if len(parts) == 4 {
			subnet := fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
			_ = config.AddSavedNetwork(subnet, host)
		}
	}
}

func (m *Manager) Discover(port int, onStatus func(string)) string {
	ip := m.discover.Discover(port, onStatus)
	if ip == "" {
		return ""
	}
	address := discoveryops.AddressForTest(ip, port)
	m.SetServerIP(address)
	return ip
}

func (m *Manager) Close() {
	if m.wsClient != nil {
		m.wsClient.Close()
	}
}
