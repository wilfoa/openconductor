// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package telegram

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openconductorhq/openconductor/internal/logging"
)

// ImageDedup tracks which images have already been sent to prevent
// re-sending the same file on consecutive attention ticks. It is safe
// for concurrent use from the TUI goroutine (sendTelegramEvent) and
// from any future callers.
//
// The dedup key format is: "projectName|absPath|modTimeUnixNano"
//
// This means:
//   - Same project, same path, same mtime: deduplicated (skipped)
//   - Same project, same path, new mtime: re-sent (agent regenerated file)
//   - Different project, same path: each project gets its own send
//
// Entries expire after ttl (5 minutes). Cleanup runs lazily on each
// ShouldSend call -- no background goroutine needed.
type ImageDedup struct {
	mu   sync.Mutex
	sent map[string]time.Time // dedup key -> time.Now() when marked as sent
	ttl  time.Duration
}

// DefaultImageDedupTTL is the default expiry for image dedup entries.
// After this duration, the same image (same path + mtime) can be re-sent.
const DefaultImageDedupTTL = 5 * time.Minute

// NewImageDedup creates an ImageDedup with the given TTL.
// A TTL of 5 minutes is recommended (see HLD section 7.3).
func NewImageDedup(ttl time.Duration) *ImageDedup {
	return &ImageDedup{
		sent: make(map[string]time.Time),
		ttl:  ttl,
	}
}

// ShouldSend returns true if the image identified by (project, absPath, mtime)
// has not been sent within the TTL window. If it returns true, the entry is
// recorded as sent. Calling ShouldSend also lazily purges expired entries.
//
// This method is called from DetectImagePaths, which runs on the TUI goroutine
// inside sendTelegramEvent. The mutex protects against future concurrent callers.
func (d *ImageDedup) ShouldSend(project, absPath string, mtime time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Lazy cleanup: purge expired entries.
	now := time.Now()
	for k, sentAt := range d.sent {
		if now.Sub(sentAt) > d.ttl {
			delete(d.sent, k)
		}
	}

	key := project + "|" + absPath + "|" + strconv.FormatInt(mtime.UnixNano(), 10)
	if _, exists := d.sent[key]; exists {
		return false
	}

	d.sent[key] = now
	return true
}

// imagePathRe matches file paths with image extensions in terminal output lines.
//
// Matching strategy:
//   - Absolute paths: /foo/bar/img.png
//   - Relative paths: ./foo/img.png, ../foo/img.png
//   - Home-relative paths: ~/screenshots/img.png
//   - Bare relative paths: foo/bar/img.png (must contain at least one /)
//
// Exclusions:
//   - URLs (https://, http://) -- the left boundary excludes ":"
//   - Glob patterns containing * or ?
//   - Shell redirections (> output.png) -- filtered post-match
//   - Paths without a directory separator (bare filenames like "img.png")
//     are excluded to reduce false positives from English text
//
// The regex captures the full path including extension in group 1.
// Extensions are case-insensitive.
var imagePathRe = regexp.MustCompile(
	`(?:^|[\s"'=(,])` +
		`(` +
		`~?` +
		`(?:\.{0,2}/)?` +
		`[^\s"'<>|*?:=(),]+/` +
		`[^\s"'<>|*?:=(),/]+` +
		`\.(?i:png|jpe?g|gif|webp|svg)` +
		`)` +
		`(?:[\s"'),:;.\]}]|$)`,
)

// maxCaptionLen is Telegram's maximum caption length for sendPhoto/sendDocument.
const maxCaptionLen = 1024

const (
	// photoMaxSize is the maximum file size for sendPhoto (Telegram limit: 10MB).
	photoMaxSize = 10 * 1024 * 1024

	// documentMaxSize is the maximum file size for sendDocument (Telegram limit: 50MB).
	documentMaxSize = 50 * 1024 * 1024
)

// DetectImagePaths scans terminal output lines for file paths with image
// extensions. Each detected path is resolved, validated (existence, magic
// bytes, size), and deduplicated. Returns only images that should be sent.
//
// This function is called from sendTelegramEvent on the TUI goroutine.
// It is the single entry point for the image detection pipeline.
//
// Parameters:
//   - lines: filtered terminal screen lines (chrome already stripped)
//   - repoDir: absolute path to the project's repo directory, used to
//     resolve relative paths
//   - dedup: ImageDedup instance for cross-tick deduplication (may be nil,
//     in which case dedup is skipped -- useful for testing)
//   - project: project name, used as part of the dedup key
//
// Returns a slice of validated ImageRef structs ready to attach to an Event.
// The slice may be empty if no valid, non-duplicate images are found.
func DetectImagePaths(lines []string, repoDir string, dedup *ImageDedup, project string) []ImageRef {
	var refs []ImageRef
	seen := make(map[string]bool) // local dedup within this call (same path in multiple lines)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		matches := imagePathRe.FindAllStringSubmatchIndex(trimmed, -1)
		for _, match := range matches {
			rawPath := trimmed[match[2]:match[3]]

			// Post-match filter: skip if preceded by > (shell redirect).
			if match[2] > 0 {
				preceding := trimmed[match[2]-1]
				if preceding == '>' {
					continue
				}
			}

			absPath := resolvePath(rawPath, repoDir)

			if seen[absPath] {
				continue
			}
			seen[absPath] = true

			mode, err := validateImage(absPath)
			if err != nil {
				logging.Debug("telegram: image validation failed", "path", absPath, "err", err)
				continue
			}

			if dedup != nil {
				info, statErr := os.Stat(absPath)
				if statErr != nil {
					continue
				}
				if !dedup.ShouldSend(project, absPath, info.ModTime()) {
					logging.Debug("telegram: image deduplicated", "path", absPath)
					continue
				}
			}

			caption := extractCaption(trimmed, match[2], match[3])
			refs = append(refs, ImageRef{
				AbsPath:  absPath,
				Caption:  caption,
				SendMode: mode,
			})
		}
	}

	return refs
}

// resolvePath resolves a detected path string to an absolute path.
//
// Resolution rules:
//   - Absolute paths (/foo/bar.png): returned as-is after filepath.Clean
//   - Home-relative (~/ prefix): expanded via os.UserHomeDir()
//   - Relative paths (./foo, ../foo, foo/bar): joined with repoDir
//
// Returns the cleaned absolute path. Does NOT verify the file exists --
// that is the caller's responsibility (via validateImage).
func resolvePath(detected, repoDir string) string {
	detected = strings.TrimSpace(detected)

	if strings.HasPrefix(detected, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Clean(detected) // best effort
		}
		return filepath.Clean(filepath.Join(home, detected[2:]))
	}

	if filepath.IsAbs(detected) {
		return filepath.Clean(detected)
	}

	// Relative path: resolve against repo directory.
	return filepath.Clean(filepath.Join(repoDir, detected))
}

// validateImage checks that the file at path is a valid image file suitable
// for sending to Telegram. It returns the appropriate SendMode.
//
// Validation pipeline:
//  1. os.Stat: must exist, must be a regular file (not dir, symlink target is ok)
//  2. Size: must be > 0 bytes, must be <= documentMaxSize (50MB)
//  3. Magic bytes: first 12 bytes must match the expected signature for the
//     file extension (SVG is exempt -- text format with variable headers)
//  4. SendMode determination:
//     - SVG -> SendAsDocument (Telegram cannot render SVG inline)
//     - Size > photoMaxSize (10MB) -> SendAsDocument
//     - Otherwise -> SendAsPhoto
//
// Returns an error if the file should be skipped. The error message is
// suitable for debug logging but not for user display.
func validateImage(path string) (SendMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("file not found: %s", path)
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("not a regular file: %s", path)
	}

	size := info.Size()
	if size == 0 {
		return 0, fmt.Errorf("empty file: %s", path)
	}
	if size > documentMaxSize {
		return 0, fmt.Errorf("file too large (%d bytes): %s", size, path)
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))

	if ext == "svg" {
		return SendAsDocument, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("cannot open: %s", path)
	}
	defer f.Close()

	header := make([]byte, 12)
	n, _ := f.Read(header)
	header = header[:n]

	if !matchesMagic(header, ext) {
		return 0, fmt.Errorf("magic bytes mismatch for .%s: %s", ext, path)
	}

	if size > photoMaxSize {
		return SendAsDocument, nil
	}
	return SendAsPhoto, nil
}

// matchesMagic checks whether the given file header matches the expected
// magic bytes for the extension. Returns true for SVG (no magic check)
// and for extensions not in the known list (permissive fallback).
func matchesMagic(data []byte, ext string) bool {
	switch ext {
	case "svg":
		return true
	case "gif":
		return bytes.HasPrefix(data, []byte("GIF87a")) ||
			bytes.HasPrefix(data, []byte("GIF89a"))
	case "webp":
		return len(data) >= 12 &&
			bytes.HasPrefix(data, []byte("RIFF")) &&
			bytes.Equal(data[8:12], []byte("WEBP"))
	case "png":
		return bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	case "jpg", "jpeg":
		return bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF})
	default:
		return true // unknown extension, permissive
	}
}

// extractCaption builds a caption string from the terminal line where an
// image path was detected. The caption is HTML-escaped and truncated to
// Telegram's 1024-character limit.
//
// If the line is longer than maxCaptionLen, the caption is trimmed to the
// path plus 50 characters of surrounding context on each side, with "..."
// ellipsis indicators.
func extractCaption(line string, pathStart, pathEnd int) string {
	trimmed := strings.TrimSpace(line)
	escaped := html.EscapeString(trimmed)
	if len(escaped) <= maxCaptionLen {
		return escaped
	}

	// Line is too long. Extract a window around the path.
	contextRadius := 50
	windowStart := pathStart - contextRadius
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := pathEnd + contextRadius
	if windowEnd > len(trimmed) {
		windowEnd = len(trimmed)
	}

	excerpt := trimmed[windowStart:windowEnd]
	if windowStart > 0 {
		excerpt = "..." + excerpt
	}
	if windowEnd < len(trimmed) {
		excerpt = excerpt + "..."
	}

	return html.EscapeString(excerpt)
}
