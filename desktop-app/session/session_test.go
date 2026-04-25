package session

import (
	"context"
	"errors"
	"testing"

	"desktop-app/crypto"
)

type fakeAPI struct {
	role       string
	err        error
	serverIP   string
	authCode   string
	pingCalled bool
}

func (f *fakeAPI) Ping(ctx context.Context) (string, error) {
	f.pingCalled = true
	return f.role, f.err
}

func (f *fakeAPI) SetServerIP(ip string)   { f.serverIP = ip }
func (f *fakeAPI) SetAuthCode(code string) { f.authCode = code }

type fakeWS struct {
	serverIP   string
	authCode   string
	connectErr error
	calls      int
}

func (f *fakeWS) Connect() error {
	f.calls++
	return f.connectErr
}

func (f *fakeWS) SetServerIP(ip string)   { f.serverIP = ip }
func (f *fakeWS) SetAuthCode(code string) { f.authCode = code }
func (f *fakeWS) Close()                   {}

type fakeTrust struct {
	code          string
	checkErr      error
	pendingCert   *crypto.CertInfo
	pendingServer string
	pendingRole   string
	cancelled     bool
	trusted       bool
}

func (f *fakeTrust) SetPairingCode(code string)        { f.code = code }
func (f *fakeTrust) Check(serverIP, role string) error { return f.checkErr }
func (f *fakeTrust) Pending() (*crypto.CertInfo, string, string, bool) {
	if f.pendingCert == nil {
		return nil, "", "", false
	}
	return f.pendingCert, f.pendingServer, f.pendingRole, true
}
func (f *fakeTrust) CancelPending() { f.cancelled = true }
func (f *fakeTrust) TrustPending()  { f.trusted = true }

type fakeDiscover struct {
	code string
	ip   string
}

func (f *fakeDiscover) SetPairingCode(code string) { f.code = code }
func (f *fakeDiscover) Discover(port int, onStatus func(string)) string {
	return f.ip
}

func TestManagerSetPairingCodePropagates(t *testing.T) {
	api := &fakeAPI{}
	ws := &fakeWS{}
	trust := &fakeTrust{}
	discover := &fakeDiscover{}
	m := NewWithDependencies(api, ws, trust, discover, "127.0.0.1:26260", "1111")

	m.SetPairingCode("422974")

	if api.authCode != "422974" || ws.authCode != "422974" || trust.code != "422974" || discover.code != "422974" {
		t.Fatalf("pairing code not propagated: api=%q ws=%q trust=%q discover=%q", api.authCode, ws.authCode, trust.code, discover.code)
	}
}

func TestManagerConnectTrustRequired(t *testing.T) {
	api := &fakeAPI{role: "admin"}
	ws := &fakeWS{}
	trust := &fakeTrust{checkErr: ErrTrustRequired, pendingCert: &crypto.CertInfo{}, pendingServer: "127.0.0.1:26260", pendingRole: "admin"}
	discover := &fakeDiscover{}
	m := NewWithDependencies(api, ws, trust, discover, "127.0.0.1:26260", "422974")

	role, err := m.Connect()
	if !errors.Is(err, ErrTrustRequired) {
		t.Fatalf("expected trust required error, got role=%q err=%v", role, err)
	}
	if !api.pingCalled {
		t.Fatal("expected ping to be called")
	}
}

func TestManagerCompleteConnectionSetsGuestState(t *testing.T) {
	api := &fakeAPI{}
	ws := &fakeWS{}
	trust := &fakeTrust{}
	discover := &fakeDiscover{}
	m := NewWithDependencies(api, ws, trust, discover, "192.168.1.6:26260", "422974")

	m.CompleteConnection("guest")

	if !m.IsGuest() {
		t.Fatal("expected guest mode")
	}
	if got := m.ClipboardChannel(); got != "guest" {
		t.Fatalf("unexpected clipboard channel: %q", got)
	}
	if ws.calls != 1 {
		t.Fatalf("expected websocket connect call, got %d", ws.calls)
	}
}

func TestManagerDiscoverUpdatesServerIP(t *testing.T) {
	api := &fakeAPI{}
	ws := &fakeWS{}
	trust := &fakeTrust{}
	discover := &fakeDiscover{ip: "192.168.1.50"}
	m := NewWithDependencies(api, ws, trust, discover, "localhost:26260", "422974")

	if got := m.Discover(26260, func(string) {}); got != "192.168.1.50" {
		t.Fatalf("unexpected discovered ip: %q", got)
	}
	if m.ServerIP() != "192.168.1.50:26260" {
		t.Fatalf("unexpected server ip: %q", m.ServerIP())
	}
}

func TestManagerCompleteConnectionSetsAdminState(t *testing.T) {
	api := &fakeAPI{}
	ws := &fakeWS{}
	trust := &fakeTrust{}
	discover := &fakeDiscover{}
	m := NewWithDependencies(api, ws, trust, discover, "192.168.1.6:26260", "422974")

	m.CompleteConnection("admin")

	if m.IsGuest() {
		t.Fatal("expected admin mode")
	}
	if got := m.ClipboardChannel(); got != "" {
		t.Fatalf("expected empty clipboard channel for admin, got %q", got)
	}
	if ws.calls != 1 {
		t.Fatalf("expected websocket connect call, got %d", ws.calls)
	}
}

func TestManagerCompleteConnectionAdminCaseInsensitive(t *testing.T) {
	api := &fakeAPI{}
	ws := &fakeWS{}
	trust := &fakeTrust{}
	discover := &fakeDiscover{}
	for _, role := range []string{"ADMIN", "Admin", "admin"} {
		m := NewWithDependencies(api, ws, trust, discover, "192.168.1.6:26260", "422974")
		m.CompleteConnection(role)
		if m.IsGuest() {
			t.Fatalf("expected admin mode for role %q", role)
		}
	}
}

func TestManagerCompleteConnectionGuestCaseInsensitive(t *testing.T) {
	api := &fakeAPI{}
	ws := &fakeWS{}
	trust := &fakeTrust{}
	discover := &fakeDiscover{}
	for _, role := range []string{"GUEST", "Guest", "guest"} {
		m := NewWithDependencies(api, ws, trust, discover, "192.168.1.6:26260", "422974")
		m.CompleteConnection(role)
		if !m.IsGuest() {
			t.Fatalf("expected guest mode for role %q", role)
		}
		if m.ClipboardChannel() != "guest" {
			t.Fatalf("expected guest clipboard channel for role %q, got %q", role, m.ClipboardChannel())
		}
	}
}
