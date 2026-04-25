package clipboardops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"desktop-app/api"
	"encoding/hex"
	"time"

	"github.com/atotto/clipboard"
	xclipboard "golang.design/x/clipboard"
)

type Service struct {
	client *api.Client
}

func New(client *api.Client) *Service {
	return &Service{client: client}
}

func (s *Service) FetchText(ctx context.Context, channel string) (string, error) {
	return s.client.GetClipboard(ctx, channel)
}

func (s *Service) PushText(ctx context.Context, text, channel string) error {
	return s.client.PushClipboard(ctx, text, channel)
}

func (s *Service) CopyToSystemClipboard(text string) error {
	return clipboard.WriteAll(text)
}

func (s *Service) FetchImage(ctx context.Context) ([]byte, error) {
	return s.client.GetClipboardImage(ctx)
}

func (s *Service) PushImage(ctx context.Context, data []byte) error {
	return s.client.PushClipboardImage(ctx, data)
}

func (s *Service) SystemImageHash(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (s *Service) UploadScreenshot(ctx context.Context, data []byte) (string, error) {
	return s.UploadScreenshotWithPrefix(ctx, data, "")
}

func (s *Service) UploadScreenshotWithPrefix(ctx context.Context, data []byte, prefix string) (string, error) {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := "screenshot_" + timestamp + ".png"
	uploadPath := filename
	if prefix != "" {
		uploadPath = prefix + "/" + filename
	}
	if err := s.client.UploadFile(ctx, uploadPath, bytes.NewReader(data)); err != nil {
		return "", err
	}
	return uploadPath, nil
}

func (s *Service) ReadSystemImageClipboard() []byte {
	data := xclipboard.Read(xclipboard.FmtImage)
	if data == nil {
		return nil
	}
	return data
}

func (s *Service) WriteImageToSystemClipboard(data []byte) {
	xclipboard.Write(xclipboard.FmtImage, data)
}
