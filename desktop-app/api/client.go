package api

import (
	"bytes"
	"context"
	"desktop-app/crypto"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Client struct {
	baseURL    string
	authCode   string
	httpClient *http.Client
	mu         sync.RWMutex
}

func NewClient(serverIP string, authCode string) *Client {
	// Use TOFU certificate pinning
	tr := &http.Transport{
		TLSClientConfig: crypto.CreateTLSConfig(nil),
	}
	return &Client{
		baseURL:  "https://" + serverIP,
		authCode: authCode,
		httpClient: &http.Client{
			// We use a reasonably long default timeout, but contexts should override this.
			Timeout:   1 * time.Hour, 
			Transport: tr,
		},
	}
}

func (c *Client) SetAuthCode(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authCode = code
}

func (c *Client) SetServerIP(ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = "https://" + ip
}

func (c *Client) getBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

func (c *Client) getAuthCode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authCode
}

// Ping tests server connectivity and returns role
func (c *Client) Ping(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.getBaseURL()+"/ping", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
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

	if result["role"] == "none" {
		return "", fmt.Errorf("unauthorized: invalid pairing code")
	}

	return result["role"], nil
}

// GetClipboard fetches current clipboard text from server
func (c *Client) GetClipboard(ctx context.Context, channel string) (string, error) {
	targetURL := c.getBaseURL() + "/clipboard"
	if channel != "" {
		targetURL += "?channel=" + channel
	}
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("fetch clipboard failed: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// GetClipboardImage fetches current clipboard image from server
func (c *Client) GetClipboardImage(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.getBaseURL()+"/clipboard/image", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch image failed: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// GetThumbnail fetches a file thumbnail from the server
func (c *Client) GetThumbnail(ctx context.Context, filename string) ([]byte, error) {
	u, err := url.Parse(c.getBaseURL() + "/thumbnail")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("name", filename)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch thumbnail failed: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// PushClipboard sends clipboard text to server
func (c *Client) PushClipboard(ctx context.Context, text string, channel string) error {
	targetURL := c.getBaseURL() + "/clipboard"
	if channel != "" {
		targetURL += "?channel=" + channel
	}
	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewBufferString(text))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("push failed: status %d", resp.StatusCode)
	}

	return nil
}

// PushClipboardImage sends clipboard image to server
func (c *Client) PushClipboardImage(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.getBaseURL()+"/clipboard/image", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())
	req.Header.Set("Content-Type", "image/png")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("push image failed: status %d", resp.StatusCode)
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

// HistoryItem represents a clipboard history entry
type HistoryItem struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// ListFiles lists files in the server's directory
func (c *Client) ListFiles(ctx context.Context, path string) ([]FileInfo, error) {
	reqURL, err := url.Parse(c.getBaseURL() + "/files")
	if err != nil {
		return nil, err
	}
	
	if path != "" {
		q := reqURL.Query()
		q.Set("path", path)
		reqURL.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("list files failed: status %d", resp.StatusCode)
	}

	var files []FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	return files, nil
}

// DownloadFile downloads a file from server
func (c *Client) DownloadFile(ctx context.Context, filename string, folder string) (io.ReadCloser, error) {
	targetURL := fmt.Sprintf("%s/download/%s", c.getBaseURL(), filename)
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
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
func (c *Client) UploadFile(ctx context.Context, filename string, reader io.Reader) error {
	baseURL, err := url.Parse(c.getBaseURL() + "/upload")
	if err != nil {
		return err
	}
	params := url.Values{}
	params.Add("name", filename)
	baseURL.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("upload failed: status %d", resp.StatusCode)
	}

	return nil
}

// GetHistory fetches clipboard history
func (c *Client) GetHistory(ctx context.Context) ([]HistoryItem, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.getBaseURL()+"/clipboard/history", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch history failed: status %d", resp.StatusCode)
	}

	var items []HistoryItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	return items, nil
}

// DeleteHistoryItem deletes a history item by ID
func (c *Client) DeleteHistoryItem(ctx context.Context, id string) error {
	targetURL := c.getBaseURL() + "/clipboard/history?id=" + id
	req, err := http.NewRequestWithContext(ctx, "DELETE", targetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("delete failed: status %d", resp.StatusCode)
	}

	return nil
}

// DeleteFile moves a file to trash
func (c *Client) DeleteFile(ctx context.Context, filename string) error {
	targetURL := c.getBaseURL() + "/delete?name=" + url.QueryEscape(filename)
	req, err := http.NewRequestWithContext(ctx, "DELETE", targetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("delete failed: status %d", resp.StatusCode)
	}

	return nil
}

// SearchResult represents a single file search result from the server.
type SearchResult struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"isDirectory"`
	Size        int64  `json:"size"`
	ModTime     string `json:"modTime"`
}

// Search queries the server for files matching the given query.
func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	body := map[string]string{"query": query}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.getBaseURL()+"/search", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.getAuthCode())
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search failed: status %d", resp.StatusCode)
	}

	var results []SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("invalid search response: %w", err)
	}
	return results, nil
}
