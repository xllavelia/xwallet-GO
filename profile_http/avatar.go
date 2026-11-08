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
	AvatarMaxRequestBytes = 384 * 1024
	AvatarMaxDecodedBytes = 256 * 1024
	AvatarMaxDimension    = 512
	AvatarMaxStoredBytes  = 96 * 1024
	AvatarThumbBytes      = 12 * 1024
	AvatarThumbDimension  = 96
)

var (
	ErrAvatarInvalid  = errors.New("avatar must be a square JPEG or PNG data URL")
	ErrAvatarTooLarge = errors.New("avatar exceeds the upload limits")
)

// Проверка размеров до декодирования пикселей защищает от чрезмерного выделения памяти.
func ProcessAvatar(dataURL string) ([]byte, []byte, error) {
	if len(dataURL) > AvatarMaxRequestBytes {
		return nil, nil, ErrAvatarTooLarge
	}
	var payload, format string
	switch {
	case strings.HasPrefix(dataURL, "data:image/jpeg;base64,"):
		payload, format = strings.TrimPrefix(dataURL, "data:image/jpeg;base64,"), "jpeg"
	case strings.HasPrefix(dataURL, "data:image/png;base64,"):
		payload, format = strings.TrimPrefix(dataURL, "data:image/png;base64,"), "png"
	default:
		return nil, nil, ErrAvatarInvalid
	}
	if base64.StdEncoding.DecodedLen(len(payload)) > AvatarMaxDecodedBytes+2 {
		return nil, nil, ErrAvatarTooLarge
	}
	if strings.ContainsAny(payload, " \t\r\n") {
		return nil, nil, ErrAvatarInvalid
	}
	data, err := base64.StdEncoding.Strict().DecodeString(payload)
	if err != nil || len(data) == 0 {
		return nil, nil, ErrAvatarInvalid
	}
	if len(data) > AvatarMaxDecodedBytes {
		return nil, nil, ErrAvatarTooLarge
	}
	if (format == "jpeg" && (len(data) < 3 || !bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}))) ||
		(format == "png" && (len(data) < 8 || !bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}))) {
		return nil, nil, ErrAvatarInvalid
	}
	var cfg image.Config
	if format == "jpeg" {
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	} else {
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	}
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width != cfg.Height {
		return nil, nil, ErrAvatarInvalid
	}
	if cfg.Width > AvatarMaxDimension || cfg.Height > AvatarMaxDimension {
		return nil, nil, ErrAvatarTooLarge
	}
	var src image.Image
	if format == "jpeg" {
		src, err = jpeg.Decode(bytes.NewReader(data))
	} else {
		src, err = png.Decode(bytes.NewReader(data))
	}
	if err != nil || src.Bounds().Dx() != cfg.Width || src.Bounds().Dy() != cfg.Height {
		return nil, nil, ErrAvatarInvalid
	}
	flat := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	draw.Draw(flat, flat.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), src, src.Bounds().Min, draw.Over)
	thumb, err := encodeJPEG(resizeSquare(flat, AvatarThumbDimension))
	if err != nil {
		return nil, nil, err
	}
	if len(thumb) > AvatarThumbBytes {
		return nil, nil, ErrAvatarTooLarge
	}
	for size := cfg.Width; size > 0; {
		avatar, err := encodeJPEG(resizeSquare(flat, size))
		if err != nil {
			return nil, nil, err
		}
		if len(avatar) <= AvatarMaxStoredBytes {
			return avatar, thumb, nil
		}
		if size <= AvatarThumbDimension {
			break
		}
		size = size * 3 / 4
		if size < AvatarThumbDimension {
			size = AvatarThumbDimension
		}
	}
	return nil, nil, ErrAvatarTooLarge
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Усреднение при уменьшении подавляет высокочастотный шум в миниатюрах.
func resizeSquare(src *image.RGBA, size int) *image.RGBA {
	if size == src.Bounds().Dx() {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	width := src.Bounds().Dx()
	for y := 0; y < size; y++ {
		y0, y1 := y*width/size, (y+1)*width/size
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < size; x++ {
			x0, x1 := x*width/size, (x+1)*width/size
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
