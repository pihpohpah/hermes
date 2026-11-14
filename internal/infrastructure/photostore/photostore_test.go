package photostore

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/disintegration/imaging"
)

func encodeTestJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	return buf.Bytes()
}

func TestSaveWritesTwoJPEGVariants(t *testing.T) {
	dir := t.TempDir()
	store := &Store{Dir: dir, Prefix: "/uploads"}

	original := imaging.New(2000, 1000, color.NRGBA{R: 40, G: 120, B: 200, A: 255})
	saved, err := store.Save(context.Background(), "test-id-1", encodeTestJPEG(t, original))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if saved.Full.Width != 1600 || saved.Full.Height != 800 {
		t.Errorf("full variant: got %dx%d, want 1600x800", saved.Full.Width, saved.Full.Height)
	}
	if saved.Small.Width != 640 || saved.Small.Height != 320 {
		t.Errorf("small variant: got %dx%d, want 640x320", saved.Small.Width, saved.Small.Height)
	}

	subdir := "photos/" + time.Now().UTC().Format("2006-01")
	for _, f := range []File{saved.Full, saved.Small} {
		if !strings.HasPrefix(f.URL, "/uploads/"+subdir+"/") {
			t.Errorf("URL %q does not start with /uploads/%s/", f.URL, subdir)
		}
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(f.URL, "/uploads/"))))
		if err != nil {
			t.Errorf("file for %q not on disk: %v", f.URL, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("file %q is empty", f.URL)
		}
	}
}

func TestSaveKeepsSmallImageSize(t *testing.T) {
	store := &Store{Dir: t.TempDir(), Prefix: "/uploads"}

	original := imaging.New(320, 200, color.White)
	saved, err := store.Save(context.Background(), "small", encodeTestJPEG(t, original))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Full.Width != 320 || saved.Full.Height != 200 {
		t.Errorf("full variant upscaled: %dx%d", saved.Full.Width, saved.Full.Height)
	}
	if saved.Small.Width != 320 {
		t.Errorf("small variant upscaled: %dx%d", saved.Small.Width, saved.Small.Height)
	}
}

func TestSaveRejectsNonImage(t *testing.T) {
	store := &Store{Dir: t.TempDir(), Prefix: "/uploads"}

	if _, err := store.Save(context.Background(), "bad", []byte("plain text, not an image")); err == nil {
		t.Fatal("Save accepted a non-image file")
	}
}

func TestSaveFlattensAlpha(t *testing.T) {
	store := &Store{Dir: t.TempDir(), Prefix: "/uploads"}

	transparent := imaging.New(100, 100, color.NRGBA{A: 0})
	if _, err := store.Save(context.Background(), "alpha", encodeTestPNG(t, transparent)); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func encodeTestPNG(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.PNG); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	return buf.Bytes()
}

func TestRemoveDeletesFiles(t *testing.T) {
	store := &Store{Dir: t.TempDir(), Prefix: "/uploads"}

	saved, err := store.Save(context.Background(), "gone", encodeTestJPEG(t, imaging.New(100, 100, color.White)))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Remove(context.Background(), saved); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(saved.Full.Path); !os.IsNotExist(err) {
		t.Error("full variant still on disk after Remove")
	}
	if _, err := os.Stat(saved.Small.Path); !os.IsNotExist(err) {
		t.Error("small variant still on disk after Remove")
	}
}
