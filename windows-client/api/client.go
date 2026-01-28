package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"k-share-client/crypto"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	BaseURL    string
	AuthCode   string
	HTTPClient *http.Client
}

func NewClient(serverIP string, authCode string) *Client {
	// Use TOFU certificate pinning
	tr := &http.Transport{
		TLSClientConfig: crypto.CreateTLSConfig(nil),
	}
	return &Client{
		BaseURL:  "https://" + serverIP,
		AuthCode: authCode,
		HTTPClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: tr,
		},
	}
}

func (c *Client) SetAuthCode(code string) {
	c.AuthCode = code
}

func (c *Client) SetServerIP(ip string) {
	c.BaseURL = "https://" + ip
}

// Ping tests server connectivity and returns role
func (c *Client) Ping() (string, error) {
	req, _ := http.NewRequest("GET", c.BaseURL+"/ping", nil)
	// Send Auth to check role
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result["status"] != "ok" {
		return "", fmt.Errorf("server status not ok")
	}

	return result["role"], nil // "admin", "guest", or "none"
}

// GetClipboard fetches current clipboard text from server
func (c *Client) GetClipboard(channel string) (string, error) {
	url := c.BaseURL + "/clipboard"
	if channel != "" {
		url += "?channel=" + channel
	}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// PushClipboard sends clipboard text to server
func (c *Client) PushClipboard(text string, channel string) error {
	url := c.BaseURL + "/clipboard"
	if channel != "" {
		url += "?channel=" + channel
	}
	req, _ := http.NewRequest("POST", url, bytes.NewBufferString(text))
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.HTTPClient.Do(req)
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

// ListFiles lists files in the server's directory
func (c *Client) ListFiles() ([]FileInfo, error) {
	req, _ := http.NewRequest("GET", c.BaseURL+"/files", nil)
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("list files failed: server returned status %d", resp.StatusCode)
	}

	var files []FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	return files, nil
}

// DownloadFile downloads a file from server
func (c *Client) DownloadFile(filename string, folder string) (io.ReadCloser, error) {
	// Folder param is ignored by new server logic usually, but we keep signature for now
	url := fmt.Sprintf("%s/download/%s", c.BaseURL, filename)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// UploadFile uploads a file to server
func (c *Client) UploadFile(filename string, reader io.Reader) error {
	// Use net/url to escape the filename properly
	baseURL, _ := url.Parse(c.BaseURL + "/upload")
	params := url.Values{}
	params.Add("name", filename)
	baseURL.RawQuery = params.Encode()

	req, _ := http.NewRequest("POST", baseURL.String(), reader)
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.HTTPClient.Do(req)
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
	req, _ := http.NewRequest("GET", c.BaseURL+"/clipboard/history", nil)
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var items []HistoryItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
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
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)

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

// DeleteFile moves a file to trash (admin only)
func (c *Client) DeleteFile(filename string) error {
	req, err := http.NewRequest("DELETE", c.BaseURL+"/delete?name="+url.QueryEscape(filename), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.AuthCode)

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
