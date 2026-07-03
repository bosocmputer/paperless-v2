package api

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
)

const maxSignatureImageBytes = 2 * 1024 * 1024

func normalizeSignatureImage(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("signature image is invalid")
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, fmt.Errorf("signature image is invalid")
	}

	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	visiblePixels := 0
	visibleAlpha := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if isSignatureBackgroundPixel(c) {
				out.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, color.RGBA{})
				continue
			}
			out.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A})
			if c.A >= 32 && signaturePixelLuminance(c) < 235 {
				visiblePixels++
				visibleAlpha += int(c.A)
			}
		}
	}
	if visiblePixels < 8 || visibleAlpha < 1024 {
		return nil, fmt.Errorf("signature image has no visible ink")
	}

	var normalized bytes.Buffer
	if err := png.Encode(&normalized, out); err != nil {
		return nil, fmt.Errorf("signature image is invalid")
	}
	return normalized.Bytes(), nil
}

func isSignatureBackgroundPixel(c color.NRGBA) bool {
	if c.A <= 8 {
		return true
	}
	maxChannel := maxUint8(c.R, c.G, c.B)
	minChannel := minUint8(c.R, c.G, c.B)
	return c.R >= 240 && c.G >= 240 && c.B >= 240 && maxChannel-minChannel <= 20
}

func signaturePixelLuminance(c color.NRGBA) int {
	return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
}

func maxUint8(values ...uint8) uint8 {
	maximum := values[0]
	for _, value := range values[1:] {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func minUint8(values ...uint8) uint8 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}
