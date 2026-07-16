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

const (
	maxSavedSignatureSourceBytes = 12 * 1024 * 1024
	maxSavedSignaturePixels      = 60_000_000
	maxSavedSignatureDimension   = 12_000
	maxSavedSignatureOutputSide  = 1600
)

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

// normalizeSavedSignatureImage keeps the large decoded source in its native
// image representation and allocates RGBA memory only for the cropped output.
func normalizeSavedSignatureImage(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > maxSavedSignatureSourceBytes {
		return nil, fmt.Errorf("saved signature image is too large")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return nil, fmt.Errorf("saved signature image is invalid")
	}
	if config.Width > maxSavedSignatureDimension || config.Height > maxSavedSignatureDimension || int64(config.Width)*int64(config.Height) > maxSavedSignaturePixels {
		return nil, fmt.Errorf("saved signature image dimensions are too large")
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("saved signature image is invalid")
	}
	bounds := img.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X-1, bounds.Min.Y-1
	visiblePixels := 0
	visibleAlpha := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if isSignatureBackgroundPixel(pixel) {
				continue
			}
			if pixel.A < 32 || signaturePixelLuminance(pixel) >= 235 {
				continue
			}
			visiblePixels++
			visibleAlpha += int(pixel.A)
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if visiblePixels < 8 || visibleAlpha < 1024 || maxX < minX || maxY < minY {
		return nil, fmt.Errorf("saved signature image has no visible ink")
	}

	// Keep a tiny source-space margin so anti-aliased edge pixels are retained.
	margin := signatureMaxInt(2, signatureMaxInt(maxX-minX+1, maxY-minY+1)/100)
	minX = signatureMaxInt(bounds.Min.X, minX-margin)
	minY = signatureMaxInt(bounds.Min.Y, minY-margin)
	maxX = signatureMinInt(bounds.Max.X-1, maxX+margin)
	maxY = signatureMinInt(bounds.Max.Y-1, maxY+margin)
	sourceWidth := maxX - minX + 1
	sourceHeight := maxY - minY + 1
	targetWidth, targetHeight := sourceWidth, sourceHeight
	if longest := signatureMaxInt(sourceWidth, sourceHeight); longest > maxSavedSignatureOutputSide {
		scale := float64(maxSavedSignatureOutputSide) / float64(longest)
		targetWidth = signatureMaxInt(1, int(float64(sourceWidth)*scale+0.5))
		targetHeight = signatureMaxInt(1, int(float64(sourceHeight)*scale+0.5))
	}

	out := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceY := minY + signatureMinInt(sourceHeight-1, int(float64(y)*float64(sourceHeight)/float64(targetHeight)))
		for x := 0; x < targetWidth; x++ {
			sourceX := minX + signatureMinInt(sourceWidth-1, int(float64(x)*float64(sourceWidth)/float64(targetWidth)))
			pixel := color.NRGBAModel.Convert(img.At(sourceX, sourceY)).(color.NRGBA)
			if isSignatureBackgroundPixel(pixel) {
				pixel = color.NRGBA{}
			}
			out.SetNRGBA(x, y, pixel)
		}
	}

	var normalized bytes.Buffer
	if err := png.Encode(&normalized, out); err != nil {
		return nil, fmt.Errorf("saved signature image is invalid")
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

func signatureMaxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func signatureMinInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
