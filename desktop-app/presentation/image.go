package presentation

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

// ANSIImage renders an image buffer into an ANSI color string using half-blocks
func ANSIImage(data []byte, maxWidth int) string {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "[Binary Image Data]"
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width > maxWidth && maxWidth > 0 {
		// Scale down (Nearest neighbor for simplicity)
		scale := float64(maxWidth) / float64(width)
		width = maxWidth
		height = int(float64(height) * scale)
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("\n[Image Preview: %dx%d]\n", bounds.Dx(), bounds.Dy()))

	// Use half blocks \u2580 (upper half). 
	// Each terminal character holds two pixels vertically (top and bottom).
	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x++ {
			// Source coordinates
			srcX := int(float64(x) * float64(bounds.Dx()) / float64(width))
			srcY1 := int(float64(y) * float64(bounds.Dy()) / float64(height))
			srcY2 := int(float64(y+1) * float64(bounds.Dy()) / float64(height))

			if srcY2 >= bounds.Dy() {
				srcY2 = bounds.Dy() - 1
			}

			r1, g1, b1, _ := img.At(srcX, srcY1).RGBA()
			r2, g2, b2, _ := img.At(srcX, srcY2).RGBA()

			// Convert to 8-bit
			r1, g1, b1 = r1>>8, g1>>8, b1>>8
			r2, g2, b2 = r2>>8, g2>>8, b2>>8

			// True color ANSI escape sequence: \x1b[38;2;R;G;Bm (fg) \x1b[48;2;R;G;Bm (bg)
			out.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm\u2580", r1, g1, b1, r2, g2, b2))
		}
		out.WriteString("\x1b[0m\n") // Reset
	}

	return out.String()
}
