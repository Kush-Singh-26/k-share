package tui

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

type progressMsg struct {
	percent    float64
	isDownload bool
}

// ProgressReader wraps an io.Reader to report progress to Bubble Tea
type ProgressReader struct {
	reader  io.Reader
	total   int64
	read    int64
	program *tea.Program
}

func NewProgressReader(reader io.Reader, total int64, program *tea.Program) *ProgressReader {
	return &ProgressReader{
		reader:  reader,
		total:   total,
		program: program,
	}
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.read += int64(n)
		
		// Calculate percentage
		var percent float64
		if pr.total > 0 {
			percent = float64(pr.read) / float64(pr.total)
		}
		
		// Throttle messages if needed, but for now we'll just send
		if pr.program != nil {
			pr.program.Send(progressMsg{percent: percent, isDownload: false})
		}
	}
	return n, err
}
