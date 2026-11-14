// Package photostore keeps uploaded photo files on the local disk and hands
// out the public URLs the catalog templates render. Every upload becomes two
// JPEG variants: full (long edge 1600 px) and small (long edge 640 px).
package photostore

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // регистрирует формат для image.Decode
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"time"

	// webp-декодер регистрирует формат в image.Decode.
	_ "golang.org/x/image/webp"

	"github.com/disintegration/imaging"
)

const (
	fullMaxEdge  = 1600
	smallMaxEdge = 640
	jpegQuality  = 85
)

// Store writes processed files under Dir and addresses them with Prefix,
// so the files must be reachable from the web root: Dir = web/uploads,
// Prefix = /uploads.
type Store struct {
	Dir    string
	Prefix string
}

// File is one stored variant with its public URL.
type File struct {
	URL    string
	Path   string
	Width  int
	Height int
}

// Saved is the result of processing one upload.
type Saved struct {
	Full  File
	Small File
}

// Save decodes data, validates it is a raster image, writes the full and
// small JPEG variants and returns them. Files land in
// photos/yyyy-mm/{id}.jpg and photos/yyyy-mm/{id}_sm.jpg. Callers generate
// id (uuid v7 from the database row) so the file name matches the row.
func (s *Store) Save(_ context.Context, id string, data []byte) (Saved, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Saved{}, fmt.Errorf("decode image config: %w", err)
	}
	if cfg.Width < 2 || cfg.Height < 2 || cfg.Width > 10000 || cfg.Height > 10000 {
		return Saved{}, fmt.Errorf("unsupported image dimensions %dx%d", cfg.Width, cfg.Height)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Saved{}, fmt.Errorf("decode image: %w", err)
	}
	img = flatten(img)

	subdir := filepath.Join("photos", time.Now().UTC().Format("2006-01"))
	dir := filepath.Join(s.Dir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Saved{}, fmt.Errorf("create storage dir: %w", err)
	}

	base := filepath.Join(dir, id)
	full, err := s.writeVariant(imaging.Fit(img, fullMaxEdge, fullMaxEdge, imaging.Lanczos), base+".jpg", subdir, id+".jpg")
	if err != nil {
		return Saved{}, err
	}

	small, err := s.writeVariant(imaging.Fit(img, smallMaxEdge, smallMaxEdge, imaging.Lanczos), base+"_sm.jpg", subdir, id+"_sm.jpg")
	if err != nil {
		os.Remove(base + ".jpg") // половинчатый результат не оставляем
		return Saved{}, err
	}

	return Saved{Full: full, Small: small}, nil
}

// flatten кладёт изображения с альфа-каналом на белый фон — JPEG его не
// умеет, а полупрозрачные миниатюры выглядят битыми на тёмной теме.
func flatten(img image.Image) *image.NRGBA {
	b := img.Bounds()
	if opaque(img) {
		return imaging.Clone(img)
	}

	canvas := imaging.New(b.Dx(), b.Dy(), color.White)
	return imaging.Paste(canvas, img, b.Min)
}

func opaque(img image.Image) bool {
	if _, ok := img.(*image.NRGBA); !ok {
		if _, ok := img.(*image.RGBA); !ok {
			return true
		}
	}

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return false
			}
		}
	}

	return true
}

// Remove deletes stored variant files after a failed registration, so no
// orphaned files stay on disk.
func (s *Store) Remove(_ context.Context, saved Saved) error {
	var first error
	for _, f := range []File{saved.Full, saved.Small} {
		if f.Path == "" {
			continue
		}
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}

	return first
}

func (s *Store) writeVariant(img *image.NRGBA, path, subdir, name string) (File, error) {
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(jpegQuality)); err != nil {
		return File{}, fmt.Errorf("encode jpeg: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return File{}, fmt.Errorf("write file: %w", err)
	}

	return File{
		URL:    s.Prefix + "/" + subdir + "/" + name,
		Path:   path,
		Width:  img.Bounds().Dx(),
		Height: img.Bounds().Dy(),
	}, nil
}
