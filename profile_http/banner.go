package profile_http

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"strings"
)

const (
	BannerMaxRequestBytes = 768 * 1024
	BannerMaxDecodedBytes = 512 * 1024
	BannerMaxDimension    = 2048
	BannerStoredBytes     = 200 * 1024
	BannerWidth           = 960
	BannerHeight          = 320
)

var (
	ErrBannerInvalid  = errors.New("banner must be a JPEG or PNG data URL")
	ErrBannerTooLarge = errors.New("banner exceeds the upload limits")
)

// ProcessBanner вырезает центральную широкую область 3:1 и сжимает её до баннерного формата.
func ProcessBanner(dataURL string) ([]byte, error) {
	if len(dataURL) > BannerMaxRequestBytes {
		return nil, ErrBannerTooLarge
	}
	var payload, format string
	switch {
	case strings.HasPrefix(dataURL, "data:image/jpeg;base64,"):
		payload, format = strings.TrimPrefix(dataURL, "data:image/jpeg;base64,"), "jpeg"
	case strings.HasPrefix(dataURL, "data:image/png;base64,"):
		payload, format = strings.TrimPrefix(dataURL, "data:image/png;base64,"), "png"
	default:
		return nil, ErrBannerInvalid
	}
	if base64.StdEncoding.DecodedLen(len(payload)) > BannerMaxDecodedBytes+2 {
		return nil, ErrBannerTooLarge
	}
	if strings.ContainsAny(payload, " \t\r\n") {
		return nil, ErrBannerInvalid
	}
	data, err := base64.StdEncoding.Strict().DecodeString(payload)
	if err != nil || len(data) == 0 {
		return nil, ErrBannerInvalid
	}
	if len(data) > BannerMaxDecodedBytes {
		return nil, ErrBannerTooLarge
	}
	if (format == "jpeg" && (len(data) < 3 || !bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}))) ||
		(format == "png" && (len(data) < 8 || !bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}))) {
		return nil, ErrBannerInvalid
	}
	var cfg image.Config
	if format == "jpeg" {
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	} else {
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	}
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, ErrBannerInvalid
	}
	if cfg.Width > BannerMaxDimension || cfg.Height > BannerMaxDimension {
		return nil, ErrBannerTooLarge
	}
	var src image.Image
	if format == "jpeg" {
		src, err = jpeg.Decode(bytes.NewReader(data))
	} else {
		src, err = png.Decode(bytes.NewReader(data))
	}
	if err != nil || src.Bounds().Dx() != cfg.Width || src.Bounds().Dy() != cfg.Height {
		return nil, ErrBannerInvalid
	}
	w, h := cfg.Width, cfg.Height
	// Центральный кроп под соотношение 3:1.
	target := BannerWidth * h
	current := w * BannerHeight
	x0, y0 := 0, 0
	if current > target {
		cropW := target / BannerHeight
		x0 = (w - cropW) / 2
		w = cropW
	} else {
		cropH := current / BannerWidth
		y0 = (h - cropH) / 2
		h = cropH
	}
	flat := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(flat, flat.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), src, image.Point{X: x0, Y: y0}, draw.Over)
	for width := BannerWidth; width >= BannerWidth/2; {
		height := width * BannerHeight / BannerWidth
		banner, err := encodeBannerJPEG(resizeRect(flat, width, height))
		if err != nil {
			return nil, err
		}
		if len(banner) <= BannerStoredBytes {
			return banner, nil
		}
		width = width * 3 / 4
	}
	return nil, ErrBannerTooLarge
}

func encodeBannerJPEG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 78}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Прямоугольный даунсэмпл усреднением — тот же приём, что и в resizeSquare.
func resizeRect(src *image.RGBA, width, height int) *image.RGBA {
	if width == src.Bounds().Dx() && height == src.Bounds().Dy() {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()
	for y := 0; y < height; y++ {
		y0, y1 := y*srcH/height, (y+1)*srcH/height
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < width; x++ {
			x0, x1 := x*srcW/width, (x+1)*srcW/width
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, count uint32
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := src.RGBAAt(sx, sy)
					r, g, b, count = r+uint32(c.R), g+uint32(c.G), b+uint32(c.B), count+1
				}
			}
			dst.SetRGBA(x, y, color.RGBA{R: uint8(r / count), G: uint8(g / count), B: uint8(b / count), A: 255})
		}
	}
	return dst
}
