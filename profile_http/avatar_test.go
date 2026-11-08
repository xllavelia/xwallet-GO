package profile_http

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"strings"
	"testing"
)

func imageURL(t *testing.T, format string, img image.Image) string {
	t.Helper()
	var b bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 80})
	}
	if err != nil {
		t.Fatal(err)
	}
	return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestAvatarCanonicalJPEGAndThumbnail(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		img := image.NewRGBA(image.Rect(0, 0, 120, 120))
		for y := 0; y < 120; y++ {
			for x := 0; x < 120; x++ {
				img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
			}
		}
		avatar, thumb, err := ProcessAvatar(imageURL(t, format, img))
		if err != nil {
			t.Fatal(err)
		}
		for i, data := range [][]byte{avatar, thumb} {
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			want := 120
			max := AvatarMaxStoredBytes
			if i == 1 {
				want = 96
				max = AvatarThumbBytes
			}
			if cfg.Width != want || cfg.Height != want || len(data) > max {
				t.Fatalf("bad canonical size %d %dx%d bytes=%d", i, cfg.Width, cfg.Height, len(data))
			}
			if !bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) {
				t.Fatal("not JPEG")
			}
		}
	}
}

func TestAvatarMetadataStripped(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	url := imageURL(t, "jpeg", img)
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := []byte("Exif\x00\x00private location <script>evil</script>")
	segment := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(metadata)+2))
	modified := append([]byte{}, data[:2]...)
	modified = append(modified, segment...)
	modified = append(modified, metadata...)
	modified = append(modified, data[2:]...)
	avatar, thumb, err := ProcessAvatar("data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(modified))
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range [][]byte{avatar, thumb} {
		if bytes.Contains(out, metadata) || bytes.Contains(out, []byte("Exif")) || bytes.Contains(out, []byte("script")) {
			t.Fatal("metadata survived")
		}
	}
}

func TestAvatarHostileFormatsAndLimits(t *testing.T) {
	pngURL := imageURL(t, "png", image.NewRGBA(image.Rect(0, 0, 8, 8)))
	jpegURL := imageURL(t, "jpeg", image.NewRGBA(image.Rect(0, 0, 8, 8)))
	for _, url := range []string{
		"https://example.com/avatar.jpg", "data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=",
		"data:image/gif;base64,R0lGODlhAQABAAAAACw=", "data:image/webp;base64,UklGRg==",
		"data:image/jpeg;base64,R0lGODlhAQABAAAAACw=", "data:image/png;base64,/9j/AAAA",
		"data:image/jpeg;base64,!!!!", "data:image/jpeg;base64,", "data:image/jpeg;base64,/9j/AAAA\n",
		strings.Replace(pngURL, "image/png", "image/jpeg", 1), strings.Replace(jpegURL, "image/jpeg", "image/png", 1),
		strings.Replace(pngURL, ";base64,", ";charset=utf-8;base64,", 1),
		"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{137, 80, 78, 71, 13, 10, 26, 10}),
	} {
		if _, _, err := ProcessAvatar(url); !errors.Is(err, ErrAvatarInvalid) {
			t.Fatalf("hostile URL accepted/error changed %q: %v", url, err)
		}
	}
	for _, format := range []string{"png", "jpeg"} {
		if _, _, err := ProcessAvatar(imageURL(t, format, image.NewRGBA(image.Rect(0, 0, 8, 7)))); !errors.Is(err, ErrAvatarInvalid) {
			t.Fatalf("non-square accepted %s: %v", format, err)
		}
		if _, _, err := ProcessAvatar(imageURL(t, format, image.NewRGBA(image.Rect(0, 0, 513, 513)))); !errors.Is(err, ErrAvatarTooLarge) {
			t.Fatalf("oversized dimensions accepted %s: %v", format, err)
		}
	}
	data, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(jpegURL, "data:image/jpeg;base64,"))
	tooLarge := append(data, make([]byte, AvatarMaxDecodedBytes+1-len(data))...)
	if _, _, err := ProcessAvatar("data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(tooLarge)); !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("decoded limit failed: %v", err)
	}
	if _, _, err := ProcessAvatar(strings.Repeat("A", AvatarMaxRequestBytes+1)); !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("request limit failed: %v", err)
	}
	// Корректная CRC нужна, чтобы проверить лимит размеров, а не ошибку заголовка.
	bomb, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(pngURL, "data:image/png;base64,"))
	binary.BigEndian.PutUint32(bomb[16:20], 1<<24)
	binary.BigEndian.PutUint32(bomb[20:24], 1<<24)
	binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
	if _, _, err := ProcessAvatar("data:image/png;base64," + base64.StdEncoding.EncodeToString(bomb)); !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("config bomb was not blocked: %v", err)
	}
}

func TestAvatarOutputSizeAndTransparency(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	rng := rand.New(rand.NewSource(1234))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
		}
	}
	avatar, thumb, err := ProcessAvatar(imageURL(t, "jpeg", img))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(avatar))
	if err != nil {
		t.Fatal(err)
	}
	if len(avatar) > AvatarMaxStoredBytes || len(thumb) > AvatarThumbBytes || cfg.Width > 512 || cfg.Width != cfg.Height {
		t.Fatal("output exceeds limits")
	}
	cfg, err = jpeg.DecodeConfig(bytes.NewReader(thumb))
	if err != nil || cfg.Width != 96 || cfg.Height != 96 {
		t.Fatal("wrong thumbnail")
	}
	transparent := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	avatar, thumb, err = ProcessAvatar(imageURL(t, "png", transparent))
	if err != nil {
		t.Fatal(err)
	}
	out, err := jpeg.Decode(bytes.NewReader(avatar))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := out.At(0, 0).RGBA()
	if r < 65000 || g < 65000 || b < 65000 {
		t.Fatal("transparency was not composited on white")
	}
	cfg, err = jpeg.DecodeConfig(bytes.NewReader(thumb))
	if err != nil || cfg.Width != 96 {
		t.Fatal("tiny source thumbnail not enlarged")
	}
}
