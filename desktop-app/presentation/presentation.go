package presentation

import (
	"desktop-app/api"
	"fmt"
	"strings"
	"time"
)

func IsImageFile(filename string) bool {
	return GetFileCategory(filename) == "image"
}

func GetFileCategory(filename string) string {
	lower := strings.ToLower(filename)

	if strings.HasSuffix(lower, ".jpg") ||
		strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") ||
		strings.HasSuffix(lower, ".gif") ||
		strings.HasSuffix(lower, ".webp") ||
		strings.HasSuffix(lower, ".bmp") {
		return "image"
	}

	if strings.HasSuffix(lower, ".pdf") {
		return "pdf"
	}

	if strings.HasSuffix(lower, ".doc") || strings.HasSuffix(lower, ".docx") ||
		strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".md") {
		return "document"
	}

	if strings.HasSuffix(lower, ".xls") || strings.HasSuffix(lower, ".xlsx") {
		return "spreadsheet"
	}

	if strings.HasSuffix(lower, ".ppt") || strings.HasSuffix(lower, ".pptx") {
		return "presentation"
	}

	if strings.HasSuffix(lower, ".mp3") || strings.HasSuffix(lower, ".wav") || strings.HasSuffix(lower, ".flac") {
		return "audio"
	}

	if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".avi") || strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".mov") {
		return "video"
	}

	if strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".rar") || strings.HasSuffix(lower, ".7z") || strings.HasSuffix(lower, ".tar") {
		return "archive"
	}

	if strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".js") ||
		strings.HasSuffix(lower, ".java") || strings.HasSuffix(lower, ".cpp") || strings.HasSuffix(lower, ".c") ||
		strings.HasSuffix(lower, ".h") || strings.HasSuffix(lower, ".cs") || strings.HasSuffix(lower, ".sh") {
		return "code"
	}

	return "other"
}

func FileIcon(filename string, isDirectory bool) string {
	if isDirectory {
		return "📁"
	}

	category := GetFileCategory(filename)
	switch category {
	case "image":
		return "🖼️"
	case "pdf":
		return "📕"
	case "document":
		return "📄"
	case "spreadsheet":
		return "📊"
	case "presentation":
		return "📽️"
	case "audio":
		return "🎵"
	case "video":
		return "🎬"
	case "archive":
		return "📦"
	case "code":
		return "💻"
	default:
		return "📄"
	}
}

func FormatSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTP"[exp])
}

func FileDisplayName(name string, isGuest bool) (string, string) {
	displayName := name
	guestLabel := ""
	if isGuest && strings.HasPrefix(displayName, "Public/") {
		displayName = strings.TrimPrefix(displayName, "Public/")
		guestLabel = " [Guest]"
	}
	return displayName, guestLabel
}

type FileRow struct {
	DisplayName  string
	GuestLabel   string
	Icon         string
	Info         string
	ThumbnailKey string
	IsDirectory  bool
	IsImage      bool
}

func BuildFileRow(file api.FileInfo, isGuest bool) FileRow {
	displayName, guestLabel := FileDisplayName(file.Name, isGuest)
	row := FileRow{
		DisplayName:  displayName,
		GuestLabel:   guestLabel,
		Icon:         FileIcon(displayName, file.IsDirectory),
		ThumbnailKey: file.Name,
		IsDirectory:  file.IsDirectory,
		IsImage:      IsImageFile(displayName) && !file.IsDirectory,
	}
	if file.IsDirectory {
		row.Info = "Folder • " + file.ModTime
		return row
	}
	row.Info = FormatSize(file.Size) + " • " + file.ModTime
	return row
}

func HistoryTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}
