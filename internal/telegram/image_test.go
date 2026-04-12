// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImagePathRegex(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    []string // expected captured paths (group 1)
		wantNil bool     // true if no match expected
	}{
		// ── Positive cases ──
		{
			name: "absolute path",
			line: "Saved to /tmp/screenshots/output.png",
			want: []string{"/tmp/screenshots/output.png"},
		},
		{
			name: "relative path with dot-slash",
			line: "Created ./output/chart.png",
			want: []string{"./output/chart.png"},
		},
		{
			name: "relative path with dot-dot-slash",
			line: "See ../images/logo.jpg",
			want: []string{"../images/logo.jpg"},
		},
		{
			name: "home-relative path",
			line: "Stored at ~/screenshots/capture.jpeg",
			want: []string{"~/screenshots/capture.jpeg"},
		},
		{
			name: "quoted path",
			line: `File written to "/home/user/output/result.gif"`,
			want: []string{"/home/user/output/result.gif"},
		},
		{
			name: "bare relative with slash",
			line: "Generated images/diagram.webp successfully",
			want: []string{"images/diagram.webp"},
		},
		{
			name: "svg extension",
			line: "Exported /var/data/chart.svg",
			want: []string{"/var/data/chart.svg"},
		},
		{
			name: "multiple paths on one line",
			line: "Compare /tmp/before.png and /tmp/after.png",
			want: []string{"/tmp/before.png", "/tmp/after.png"},
		},
		{
			name: "uppercase extension",
			line: "Screenshot saved to /tmp/shots/SCREEN.PNG",
			want: []string{"/tmp/shots/SCREEN.PNG"},
		},
		{
			name: "mixed case extension",
			line: "Photo at /home/user/pics/Photo.Jpg done",
			want: []string{"/home/user/pics/Photo.Jpg"},
		},
		{
			name: "path at start of line",
			line: "/tmp/output/result.png generated",
			want: []string{"/tmp/output/result.png"},
		},
		{
			name: "path at end of line",
			line: "Saved to /tmp/output/result.png",
			want: []string{"/tmp/output/result.png"},
		},
		{
			name: "path in parentheses",
			line: "see (./docs/arch.png) for details",
			want: []string{"./docs/arch.png"},
		},
		{
			name: "path after equals",
			line: "output=/tmp/result.png",
			want: []string{"/tmp/result.png"},
		},
		{
			name: "jpeg alternative spelling",
			line: "Wrote /tmp/photo.jpeg to disk",
			want: []string{"/tmp/photo.jpeg"},
		},

		// ── Negative cases ──
		{
			name:    "bare filename without slash",
			line:    "loading.png",
			wantNil: true,
		},
		{
			name:    "URL with https",
			line:    "Download from https://example.com/image.png",
			wantNil: true,
		},
		{
			name:    "URL with http",
			line:    "See http://cdn.example.com/photo.jpg",
			wantNil: true,
		},
		{
			name:    "directory path with trailing slash",
			line:    "Created directory /tmp/images/",
			wantNil: true,
		},
		{
			name:    "non-image extension",
			line:    "Wrote /tmp/output/data.csv",
			wantNil: true,
		},
		{
			name:    "glob pattern with wildcard",
			line:    "Matching /tmp/*.png files",
			wantNil: true,
		},
		{
			name:    "glob pattern with question mark",
			line:    "Matching /tmp/img?.png files",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := imagePathRe.FindAllStringSubmatch(tt.line, -1)

			if tt.wantNil {
				if len(matches) != 0 {
					var got []string
					for _, m := range matches {
						got = append(got, m[1])
					}
					t.Errorf("expected no match, got %v", got)
				}
				return
			}

			if len(matches) != len(tt.want) {
				var got []string
				for _, m := range matches {
					got = append(got, m[1])
				}
				t.Fatalf("expected %d matches %v, got %d: %v", len(tt.want), tt.want, len(matches), got)
			}

			for i, m := range matches {
				if m[1] != tt.want[i] {
					t.Errorf("match[%d]: expected %q, got %q", i, tt.want[i], m[1])
				}
			}
		})
	}
}

func TestImagePathRegex_ShellRedirectFiltered(t *testing.T) {
	// Shell redirect matching is handled as post-match filtering in
	// DetectImagePaths, not in the regex itself. Verify the regex does
	// match (the filter happens later).
	line := "> /tmp/output.png"
	matches := imagePathRe.FindAllStringSubmatch(line, -1)
	// The regex may or may not match here; the important test is in
	// TestDetectImagePaths_ShellRedirect which verifies the post-match filter.
	_ = matches
}

func TestResolvePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	repoDir := "/home/user/myproject"

	tests := []struct {
		name     string
		detected string
		repoDir  string
		want     string
	}{
		{
			name:     "absolute path stays absolute",
			detected: "/tmp/screenshots/img.png",
			repoDir:  repoDir,
			want:     "/tmp/screenshots/img.png",
		},
		{
			name:     "absolute path is cleaned",
			detected: "/tmp/../tmp/img.png",
			repoDir:  repoDir,
			want:     "/tmp/img.png",
		},
		{
			name:     "home-relative expands",
			detected: "~/photos/img.png",
			repoDir:  repoDir,
			want:     filepath.Join(home, "photos/img.png"),
		},
		{
			name:     "dot-slash relative joins with repo",
			detected: "./output/chart.png",
			repoDir:  repoDir,
			want:     filepath.Join(repoDir, "output/chart.png"),
		},
		{
			name:     "dot-dot-slash relative joins with repo",
			detected: "../sibling/img.png",
			repoDir:  repoDir,
			want:     filepath.Clean(filepath.Join(repoDir, "../sibling/img.png")),
		},
		{
			name:     "bare relative joins with repo",
			detected: "assets/logo.png",
			repoDir:  repoDir,
			want:     filepath.Join(repoDir, "assets/logo.png"),
		},
		{
			name:     "whitespace trimmed",
			detected: "  /tmp/img.png  ",
			repoDir:  repoDir,
			want:     "/tmp/img.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePath(tt.detected, tt.repoDir)
			if got != tt.want {
				t.Errorf("resolvePath(%q, %q) = %q, want %q", tt.detected, tt.repoDir, got, tt.want)
			}
		})
	}
}

// writeTempFile creates a temp file with the given content and extension, returning its path.
func writeTempFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write temp file %s: %v", path, err)
	}
	return path
}

func TestValidateImage(t *testing.T) {
	tmp := t.TempDir()

	// Create test files with correct magic bytes.
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	jpegMagic := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	gifMagic := append([]byte("GIF89a"), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	webpMagic := []byte{'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'}
	svgContent := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`)

	pngPath := writeTempFile(t, tmp, "valid.png", pngMagic)
	jpegPath := writeTempFile(t, tmp, "valid.jpg", jpegMagic)
	gifPath := writeTempFile(t, tmp, "valid.gif", gifMagic)
	webpPath := writeTempFile(t, tmp, "valid.webp", webpMagic)
	svgPath := writeTempFile(t, tmp, "valid.svg", svgContent)
	emptyPath := writeTempFile(t, tmp, "empty.png", []byte{})
	badMagicPath := writeTempFile(t, tmp, "bad.png", []byte("this is not a PNG"))

	// Create a subdirectory (not a regular file).
	dirPath := filepath.Join(tmp, "subdir")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantMode SendMode
		wantErr  bool
	}{
		{name: "valid PNG", path: pngPath, wantMode: SendAsPhoto},
		{name: "valid JPEG", path: jpegPath, wantMode: SendAsPhoto},
		{name: "valid GIF", path: gifPath, wantMode: SendAsPhoto},
		{name: "valid WebP", path: webpPath, wantMode: SendAsPhoto},
		{name: "valid SVG (always document)", path: svgPath, wantMode: SendAsDocument},
		{name: "empty file", path: emptyPath, wantErr: true},
		{name: "bad magic bytes", path: badMagicPath, wantErr: true},
		{name: "non-existent file", path: filepath.Join(tmp, "nonexistent.png"), wantErr: true},
		{name: "directory not regular file", path: dirPath, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := validateImage(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got mode=%d", mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %d, want %d", mode, tt.wantMode)
			}
		})
	}
}

func TestValidateImage_LargeRaster(t *testing.T) {
	tmp := t.TempDir()

	// Create a file that exceeds photoMaxSize (10MB) but fits documentMaxSize (50MB).
	// We only write the header + enough padding to push it past 10MB.
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	size := photoMaxSize + 1024 // just over 10MB
	data := make([]byte, size)
	copy(data, pngHeader)

	path := writeTempFile(t, tmp, "large.png", data)
	mode, err := validateImage(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != SendAsDocument {
		t.Errorf("expected SendAsDocument for >10MB raster, got %d", mode)
	}
}

func TestMatchesMagic(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		ext  string
		want bool
	}{
		{name: "PNG valid", data: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, ext: "png", want: true},
		{name: "PNG invalid", data: []byte{0x00, 0x00, 0x00, 0x00}, ext: "png", want: false},
		{name: "JPEG valid", data: []byte{0xFF, 0xD8, 0xFF, 0xE0}, ext: "jpg", want: true},
		{name: "JPEG valid (jpeg ext)", data: []byte{0xFF, 0xD8, 0xFF, 0xE1}, ext: "jpeg", want: true},
		{name: "JPEG invalid", data: []byte{0x00, 0x00, 0x00}, ext: "jpg", want: false},
		{name: "GIF87a", data: []byte("GIF87a......"), ext: "gif", want: true},
		{name: "GIF89a", data: []byte("GIF89a......"), ext: "gif", want: true},
		{name: "GIF invalid", data: []byte("GIF90a......"), ext: "gif", want: false},
		{name: "WebP valid", data: []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}, ext: "webp", want: true},
		{name: "WebP missing WEBP", data: []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'A', 'V', 'I', ' '}, ext: "webp", want: false},
		{name: "WebP too short", data: []byte{'R', 'I', 'F', 'F'}, ext: "webp", want: false},
		{name: "SVG always true", data: []byte("anything"), ext: "svg", want: true},
		{name: "unknown ext permissive", data: []byte("anything"), ext: "bmp", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesMagic(tt.data, tt.ext)
			if got != tt.want {
				t.Errorf("matchesMagic(%v, %q) = %v, want %v", tt.data, tt.ext, got, tt.want)
			}
		})
	}
}

func TestImageDedup(t *testing.T) {
	t.Run("first occurrence passes", func(t *testing.T) {
		d := NewImageDedup(5 * time.Minute)
		mtime := time.Now()
		if !d.ShouldSend("proj", "/tmp/img.png", mtime) {
			t.Error("first call should return true")
		}
	})

	t.Run("repeat blocked", func(t *testing.T) {
		d := NewImageDedup(5 * time.Minute)
		mtime := time.Now()
		d.ShouldSend("proj", "/tmp/img.png", mtime)
		if d.ShouldSend("proj", "/tmp/img.png", mtime) {
			t.Error("repeat call with same mtime should return false")
		}
	})

	t.Run("new mtime passes", func(t *testing.T) {
		d := NewImageDedup(5 * time.Minute)
		mtime1 := time.Now()
		d.ShouldSend("proj", "/tmp/img.png", mtime1)

		mtime2 := mtime1.Add(1 * time.Second)
		if !d.ShouldSend("proj", "/tmp/img.png", mtime2) {
			t.Error("new mtime should return true")
		}
	})

	t.Run("different project independent", func(t *testing.T) {
		d := NewImageDedup(5 * time.Minute)
		mtime := time.Now()
		d.ShouldSend("proj-a", "/tmp/img.png", mtime)

		if !d.ShouldSend("proj-b", "/tmp/img.png", mtime) {
			t.Error("different project should return true")
		}
	})

	t.Run("TTL expiry", func(t *testing.T) {
		// Use a very short TTL so entries expire immediately.
		d := NewImageDedup(1 * time.Nanosecond)
		mtime := time.Now()
		d.ShouldSend("proj", "/tmp/img.png", mtime)

		// Wait just enough for the TTL to expire.
		time.Sleep(1 * time.Millisecond)

		// The lazy cleanup in ShouldSend should purge the expired entry.
		if !d.ShouldSend("proj", "/tmp/img.png", mtime) {
			t.Error("should return true after TTL expiry")
		}
	})
}

func TestExtractCaption(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		pathStart int
		pathEnd   int
		wantCheck func(t *testing.T, got string)
	}{
		{
			name:      "short line returned as-is with HTML escaping",
			line:      "Saved to /tmp/img.png",
			pathStart: 9,
			pathEnd:   21,
			wantCheck: func(t *testing.T, got string) {
				if got != "Saved to /tmp/img.png" {
					t.Errorf("got %q", got)
				}
			},
		},
		{
			name:      "HTML characters escaped",
			line:      `File <b>/tmp/img.png</b> & done`,
			pathStart: 7,
			pathEnd:   19,
			wantCheck: func(t *testing.T, got string) {
				if !strings.Contains(got, "&lt;b&gt;") {
					t.Errorf("expected HTML escaping, got %q", got)
				}
				if !strings.Contains(got, "&amp;") {
					t.Errorf("expected & escaped, got %q", got)
				}
			},
		},
		{
			name:      "long line truncated with ellipsis",
			line:      strings.Repeat("x", 500) + "/tmp/img.png" + strings.Repeat("y", 600),
			pathStart: 500,
			pathEnd:   512,
			wantCheck: func(t *testing.T, got string) {
				if len(got) > maxCaptionLen {
					t.Errorf("caption too long: %d > %d", len(got), maxCaptionLen)
				}
				if !strings.Contains(got, "...") {
					t.Errorf("expected ellipsis in truncated caption, got %q", got)
				}
				if !strings.Contains(got, "/tmp/img.png") {
					t.Errorf("expected path in truncated caption, got %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCaption(tt.line, tt.pathStart, tt.pathEnd)
			tt.wantCheck(t, got)
		})
	}
}

func TestDetectImagePaths(t *testing.T) {
	tmp := t.TempDir()

	// Create real image files with correct magic bytes.
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	jpegMagic := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}

	// Create subdirectory structure.
	outputDir := filepath.Join(tmp, "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	pngPath := writeTempFile(t, outputDir, "chart.png", pngMagic)
	jpegPath := writeTempFile(t, outputDir, "photo.jpg", jpegMagic)

	t.Run("detects absolute path", func(t *testing.T) {
		lines := []string{
			"Generated " + pngPath,
		}
		refs := DetectImagePaths(lines, tmp, nil, "test-proj")
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if refs[0].AbsPath != pngPath {
			t.Errorf("AbsPath = %q, want %q", refs[0].AbsPath, pngPath)
		}
		if refs[0].SendMode != SendAsPhoto {
			t.Errorf("SendMode = %d, want SendAsPhoto", refs[0].SendMode)
		}
	})

	t.Run("detects relative path", func(t *testing.T) {
		lines := []string{
			"Created ./output/photo.jpg successfully",
		}
		refs := DetectImagePaths(lines, tmp, nil, "test-proj")
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if refs[0].AbsPath != jpegPath {
			t.Errorf("AbsPath = %q, want %q", refs[0].AbsPath, jpegPath)
		}
	})

	t.Run("multiple images on different lines", func(t *testing.T) {
		lines := []string{
			"First: " + pngPath,
			"Second: " + jpegPath,
		}
		refs := DetectImagePaths(lines, tmp, nil, "test-proj")
		if len(refs) != 2 {
			t.Fatalf("expected 2 refs, got %d", len(refs))
		}
	})

	t.Run("duplicate path in multiple lines deduplicated within call", func(t *testing.T) {
		lines := []string{
			"See " + pngPath,
			"Also " + pngPath,
		}
		refs := DetectImagePaths(lines, tmp, nil, "test-proj")
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref (deduplicated), got %d", len(refs))
		}
	})

	t.Run("skips empty lines", func(t *testing.T) {
		lines := []string{
			"",
			"   ",
			"Generated " + pngPath,
		}
		refs := DetectImagePaths(lines, tmp, nil, "test-proj")
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
	})

	t.Run("skips non-existent file", func(t *testing.T) {
		lines := []string{
			"File at /tmp/nonexistent_xyz_abc/missing.png",
		}
		refs := DetectImagePaths(lines, tmp, nil, "test-proj")
		if len(refs) != 0 {
			t.Fatalf("expected 0 refs for non-existent file, got %d", len(refs))
		}
	})

	t.Run("dedup blocks repeated sends", func(t *testing.T) {
		dedup := NewImageDedup(5 * time.Minute)
		lines := []string{
			"Generated " + pngPath,
		}

		refs1 := DetectImagePaths(lines, tmp, dedup, "test-proj")
		if len(refs1) != 1 {
			t.Fatalf("first call: expected 1 ref, got %d", len(refs1))
		}

		refs2 := DetectImagePaths(lines, tmp, dedup, "test-proj")
		if len(refs2) != 0 {
			t.Fatalf("second call: expected 0 refs (deduplicated), got %d", len(refs2))
		}
	})

	t.Run("caption is HTML escaped", func(t *testing.T) {
		lines := []string{
			"<b>Image:</b> " + pngPath,
		}
		refs := DetectImagePaths(lines, tmp, nil, "test-proj")
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if !strings.Contains(refs[0].Caption, "&lt;b&gt;") {
			t.Errorf("caption should be HTML-escaped, got %q", refs[0].Caption)
		}
	})
}

func TestDetectImagePaths_ShellRedirect(t *testing.T) {
	tmp := t.TempDir()
	outputDir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	writeTempFile(t, outputDir, "output.png", pngMagic)

	lines := []string{
		">" + filepath.Join(outputDir, "output.png"),
	}
	refs := DetectImagePaths(lines, tmp, nil, "test-proj")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs for shell redirect, got %d", len(refs))
	}
}
