package api

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestNormalizeSignatureImagePreservesTransparentPNG(t *testing.T) {
	data := encodeSignaturePNG(t, color.RGBA{})
	normalized, err := normalizeSignatureImage(data)
	if err != nil {
		t.Fatalf("normalize transparent png: %v", err)
	}
	img := decodePNG(t, normalized)
	if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
		t.Fatalf("transparent background alpha = %d, want 0", alpha)
	}
	if _, _, _, alpha := img.At(5, 4).RGBA(); alpha == 0 {
		t.Fatal("signature ink should remain visible")
	}
}

func TestNormalizeSignatureImageRemovesWhiteBackground(t *testing.T) {
	data := encodeSignaturePNG(t, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	normalized, err := normalizeSignatureImage(data)
	if err != nil {
		t.Fatalf("normalize white png: %v", err)
	}
	img := decodePNG(t, normalized)
	if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
		t.Fatalf("white background alpha = %d, want 0", alpha)
	}
	if _, _, _, alpha := img.At(5, 4).RGBA(); alpha == 0 {
		t.Fatal("signature ink should remain visible")
	}
}

func TestNormalizeSignatureImageRemovesJPEGWhiteBackground(t *testing.T) {
	img := signatureTestImage(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	normalized, err := normalizeSignatureImage(buf.Bytes())
	if err != nil {
		t.Fatalf("normalize jpeg: %v", err)
	}
	decoded := decodePNG(t, normalized)
	if _, _, _, alpha := decoded.At(0, 0).RGBA(); alpha != 0 {
		t.Fatalf("jpeg white background alpha = %d, want 0", alpha)
	}
	if _, _, _, alpha := decoded.At(5, 4).RGBA(); alpha == 0 {
		t.Fatal("signature ink should remain visible")
	}
}

func TestNormalizeSignatureImageRejectsBlankSignature(t *testing.T) {
	if _, err := normalizeSignatureImage(encodeBlankPNG(t, color.RGBA{R: 255, G: 255, B: 255, A: 255})); err == nil {
		t.Fatal("expected blank white signature to be rejected")
	}
	if _, err := normalizeSignatureImage(encodeBlankPNG(t, color.RGBA{})); err == nil {
		t.Fatal("expected blank transparent signature to be rejected")
	}
}

func TestNormalizeSavedSignatureImageCropsAndScalesLargeJPEG(t *testing.T) {
	source := image.NewGray(image.Rect(0, 0, 2400, 1800))
	for index := range source.Pix {
		source.Pix[index] = 255
	}
	for y := 700; y < 900; y++ {
		for x := 300; x < 2200; x++ {
			if (x+y)%17 < 5 {
				source.SetGray(x, y, color.Gray{Y: 20})
			}
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}

	normalized, err := normalizeSavedSignatureImage(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(normalized))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" {
		t.Fatalf("expected PNG output, got %s", format)
	}
	if config.Width > maxSavedSignatureOutputSide || config.Height > maxSavedSignatureOutputSide {
		t.Fatalf("normalized image exceeds output cap: %dx%d", config.Width, config.Height)
	}
	if config.Height >= 1800 {
		t.Fatalf("expected whitespace crop, got height %d", config.Height)
	}
}

func TestNormalizeSavedSignatureImageRejectsBlank(t *testing.T) {
	if _, err := normalizeSavedSignatureImage(encodeBlankPNG(t, color.RGBA{R: 255, G: 255, B: 255, A: 255})); err == nil {
		t.Fatal("expected blank saved signature to be rejected")
	}
}

func signatureTestImage(background color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 24, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 24; x++ {
			img.SetRGBA(x, y, background)
		}
	}
	for x := 2; x < 22; x++ {
		img.SetRGBA(x, 4, color.RGBA{R: 17, G: 24, B: 39, A: 255})
		img.SetRGBA(x, 5, color.RGBA{R: 17, G: 24, B: 39, A: 255})
	}
	return img
}

func encodeSignaturePNG(t *testing.T, background color.RGBA) []byte {
	t.Helper()
	img := signatureTestImage(background)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeBlankPNG(t *testing.T, background color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 24; x++ {
			img.SetRGBA(x, y, background)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode blank png: %v", err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return img
}
