package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"k-share-client/crypto"
	"net/http"
	"time"
)

type Client struct {
	BaseURL     string
	PairingCode string
	HTTPClient  *http.Client
}

func NewClient(serverIP string, pairingCode string) *Client {
	return &Client{
		BaseURL:     "http://" + serverIP,
		PairingCode: pairingCode,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) SetPairingCode(code string) {
	c.PairingCode = code
}

func (c *Client) SetServerIP(ip string) {
	c.BaseURL = "http://" + ip
}

// Ping tests server connectivity
func (c *Client) Ping() error {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/ping")
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	data, err := crypto.DecryptData(string(body), c.PairingCode)
	if err != nil {
		return fmt.Errorf("decryption failed (wrong code?): %w", err)
	}

	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}

	if result["status"] != "ok" {
		return fmt.Errorf("server status not ok")
	}

	return nil
}

// GetClipboard fetches current clipboard text from server
func (c *Client) GetClipboard() (string, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/clipboard")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	data, err := crypto.DecryptData(string(body), c.PairingCode)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// PushClipboard sends clipboard text to server
func (c *Client) PushClipboard(text string) error {
	encrypted, err := crypto.EncryptData([]byte(text), c.PairingCode)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/clipboard", "text/plain", bytes.NewReader([]byte(encrypted)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("push failed: status %d", resp.StatusCode)
	}

	return nil
}

// FileInfo represents a file or directory on the server
type FileInfo struct {
	Name        string `json:"name"`
	IsDirectory bool   `json:"isDirectory"`
	Size        int64  `json:"size"`
	ModTime     string `json:"modTime"`
}

// ListFromPhoneFiles lists files in the server's from_phone directory
func (c *Client) ListFromPhoneFiles() ([]FileInfo, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/files/fromphone")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	data, err := crypto.DecryptData(string(body), c.PairingCode)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, err
	}

	return files, nil
}

// DownloadFile downloads an encrypted file from server and decrypts it
func (c *Client) DownloadFile(filename string, folder string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/download/%s?folder=%s", c.BaseURL, filename, folder)
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// UploadFile uploads an encrypted file to server's to_phone directory
func (c *Client) UploadFile(filename string, reader io.Reader) error {
	// Encrypt the stream
	buf := &bytes.Buffer{}
	if err := crypto.EncryptStream(buf, reader, c.PairingCode); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/upload?folder=tophone&name=%s", c.BaseURL, filename)
	resp, err := c.HTTPClient.Post(url, "application/octet-stream", buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("upload failed: status %d", resp.StatusCode)
	}

	return nil
}

// HistoryItem represents a clipboard history entry
type HistoryItem struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// GetHistory fetches clipboard history
func (c *Client) GetHistory() ([]HistoryItem, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/clipboard/history")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	data, err := crypto.DecryptData(string(body), c.PairingCode)
	if err != nil {
		return nil, err
	}

	var items []HistoryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}

	return items, nil
}

// DeleteHistoryItem deletes a history item by ID
func (c *Client) DeleteHistoryItem(id string) error {
	req, err := http.NewRequest("DELETE", c.BaseURL+"/clipboard/history?id="+id, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("delete failed: status %d", resp.StatusCode)
	}

	return nil
}
