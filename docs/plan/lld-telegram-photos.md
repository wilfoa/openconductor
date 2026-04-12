# Low-Level Design: Telegram Photo Sending

**Feature**: Detect image file paths in agent terminal output and forward them to Telegram Forum Topics
**Status**: Draft
**Date**: 2026-04-11
**Depends on**: HLD (`docs/plan/telegram-photos-hld.md`), User Story (`docs/user-stories/telegram-photo-sending.md`)
**Scope**: `internal/telegram/image.go` (new), `internal/telegram/bot.go` (modified), `internal/telegram/bridge.go` (modified), `internal/tui/app.go` (modified), and all associated tests


## 1. New File: `internal/telegram/image.go`

This file contains all image-related types, detection, validation, deduplication, and caption extraction. It has no dependency on TUI, session, or agent packages. All functions operate on plain strings and file paths.

### 1.1 Type: `SendMode`

```go
// SendMode determines whether an image is sent via sendPhoto (inline preview)
// or sendDocument (original quality, no inline preview).
type SendMode int

const (
    // SendAsPhoto sends raster images under 10MB via Telegram's sendPhoto API.
    // The image gets an inline preview and thumbnail in the chat.
    SendAsPhoto SendMode = iota

    // SendAsDocument sends SVG files or images over 10MB via Telegram's
    // sendDocument API. The file is sent at original quality without
    // compression, but has no inline preview.
    SendAsDocument
)
```

**Design note**: `SendMode` is an `int` enum rather than a `string` because it is never serialized or displayed to users. The two values map directly to Telegram API method names, and the `switch` in `sendEvent` is exhaustive.

### 1.2 Type: `ImageRef`

```go
// ImageRef is a validated, deduplicated reference to an image file detected
// in agent terminal output. By the time an ImageRef is created, the file has
// been confirmed to exist, pass magic-byte validation, and clear dedup checks.
type ImageRef struct {
    // AbsPath is the resolved absolute path to the image file on disk.
    AbsPath string

    // Caption is the HTML-escaped terminal line (or excerpt) where the
    // path was found. Truncated to Telegram's 1024-character caption limit.
    Caption string

    // SendMode determines whether to use sendPhoto or sendDocument.
    SendMode SendMode
}
```

**Why no `ModTime`, `Size`, or `Format` fields**: The HLD defines these on `ImageRef`, but they are only used transiently during `DetectImagePaths` (for validation and dedup key construction). Once the `ImageRef` is attached to an `Event`, the consumer (`sendEvent`) needs only the path, caption, and send mode. Storing intermediate validation data on the struct would be dead weight. The dedup key is computed at detection time and stored in `ImageDedup`, not carried on the ref.

### 1.3 Type: `ImageDedup`

```go
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

// NewImageDedup creates an ImageDedup with the given TTL.
// A TTL of 5 minutes is recommended (see HLD section 7.3).
func NewImageDedup(ttl time.Duration) *ImageDedup {
    return &ImageDedup{
        sent: make(map[string]time.Time),
        ttl:  ttl,
    }
}
```

**Default TTL**: 5 minutes. This is not configurable via YAML in the MVP. The constant is defined alongside the constructor:

```go
// DefaultImageDedupTTL is the default expiry for image dedup entries.
// After this duration, the same image (same path + mtime) can be re-sent.
const DefaultImageDedupTTL = 5 * time.Minute
```

### 1.4 Method: `ImageDedup.ShouldSend`

```go
// ShouldSend returns true if the image identified by (project, absPath, mtime)
// has not been sent within the TTL window. If it returns true, the entry is
// recorded as sent. Calling ShouldSend also lazily purges expired entries.
//
// This method is called from DetectImagePaths, which runs on the TUI goroutine
// inside sendTelegramEvent. The mutex protects against future concurrent callers.
func (d *ImageDedup) ShouldSend(project, absPath string, mtime time.Time) bool
```

**Algorithm**:

```
1. Lock d.mu
2. Lazy cleanup: iterate d.sent, delete entries where time.Since(sentAt) > d.ttl
   - Cleanup runs on every call. This is O(n) where n = number of entries.
     At ~100 bytes per entry and a 5-minute TTL, n is bounded by the number
     of unique images detected in 5 minutes. Even a Playwright-heavy session
     produces at most ~50 images in 5 minutes, so this is negligible.
3. Build key: project + "|" + absPath + "|" + strconv.FormatInt(mtime.UnixNano(), 10)
4. If key exists in d.sent: return false (already sent)
5. Record d.sent[key] = time.Now()
6. Unlock, return true
```

**Key format rationale**: Using `UnixNano` for the mtime component ensures nanosecond precision. Two file writes within the same second produce different keys if the OS reports different nanosecond timestamps. The `|` separator is safe because neither project names nor absolute paths contain `|` on macOS/Linux (it is not a valid filename character).

### 1.5 Constant: `imagePathRe` (compiled regex)

```go
// imagePathRe matches file paths with image extensions in terminal output lines.
//
// Matching strategy:
//   - Absolute paths: /foo/bar/img.png
//   - Relative paths: ./foo/img.png, ../foo/img.png
//   - Home-relative paths: ~/screenshots/img.png
//   - Bare relative paths: foo/bar/img.png (must contain at least one /)
//
// Exclusions:
//   - URLs (https://, http://) -- matched by a negative lookbehind approximation
//   - Glob patterns containing * or ?
//   - Shell redirections (> output.png)
//   - Paths without a directory separator (bare filenames like "img.png")
//     are excluded to reduce false positives from English text
//
// The regex captures the full path (group 1). Extensions are case-insensitive.
var imagePathRe = regexp.MustCompile(
    `(?:^|[\s"'=(,])` +           // left boundary: start-of-line or delimiter
    `(` +
        `~?` +                     // optional ~ prefix for home-relative paths
        `(?:\.{0,2}/)?` +         // optional ./ or ../ or / prefix
        `[^\s"'<>|*?]+` +         // path body: no whitespace or shell metacharacters
        `/` +                      // at least one directory separator in the path
        `[^\s"'<>|*?/]+` +        // filename portion (no / -- avoids trailing-slash dirs)
    `)` +
    `\.(?i:png|jpe?g|gif|webp|svg)` + // image extension, case-insensitive
    `(?:[\s"'),:;.\]}]|$)`,            // right boundary: delimiter or end-of-line
)
```

**Design decisions**:

1. **Require at least one `/` in the path body**: This prevents matching bare words that happen to end in `.png` (e.g., "loading.png" in prose). Agent output referencing image files almost always includes a directory component (`./output/chart.png`, `/tmp/screenshot.png`).

2. **Exclude URLs**: The left boundary `[\s"'=(,]` does not match `:` or `/`, so `https://example.com/image.png` does not match because `//example.com/image.png` starts after `https:` which leaves `:` as the preceding character.

3. **No keyword prefix requirement**: Per HLD resolved decision #2. Any path with an image extension that passes `os.Stat` + magic byte validation is a valid candidate.

4. **`(?i:...)` for extensions only**: The path body remains case-sensitive (filesystem paths on Linux are case-sensitive). Only the extension match is case-insensitive to catch `.PNG`, `.Jpg`, etc.

### 1.6 Constant: `maxCaptionLen`

```go
// maxCaptionLen is Telegram's maximum caption length for sendPhoto/sendDocument.
const maxCaptionLen = 1024
```

### 1.7 Size thresholds

```go
const (
    // photoMaxSize is the maximum file size for sendPhoto (Telegram limit: 10MB).
    photoMaxSize = 10 * 1024 * 1024

    // documentMaxSize is the maximum file size for sendDocument (Telegram limit: 50MB).
    documentMaxSize = 50 * 1024 * 1024
)
```

### 1.8 Magic byte signatures

```go
// magicBytes maps image formats to their file header signatures.
// Used to validate that a file with an image extension actually contains
// image data (not a misnamed text file or partial write).
var magicBytes = map[string][]byte{
    "png":  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // \x89PNG\r\n\x1a\n
    "jpg":  {0xFF, 0xD8, 0xFF},                                  // JPEG SOI + marker
    "jpeg": {0xFF, 0xD8, 0xFF},
    "gif":  nil, // checked separately: "GIF87a" or "GIF89a"
    "webp": nil, // checked separately: "RIFF" + 4 bytes + "WEBP"
    "svg":  nil, // text format, no magic byte check (extension only)
}
```

### 1.9 Function: `validateImage`

```go
// validateImage checks that the file at path is a valid image file suitable
// for sending to Telegram. It returns the appropriate SendMode and file size.
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
func validateImage(path string) (SendMode, int64, error)
```

**Algorithm**:

```
1.  info, err := os.Stat(path)
2.  if err != nil: return error "file not found: <path>"
3.  if !info.Mode().IsRegular(): return error "not a regular file: <path>"
4.  size := info.Size()
5.  if size == 0: return error "empty file: <path>"
6.  if size > documentMaxSize: return error "file too large (<size> bytes): <path>"
7.  ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
8.  if ext == "svg":
        if size > photoMaxSize: return SendAsDocument, size, nil
        return SendAsDocument, size, nil   // SVG always document
9.  f, err := os.Open(path)
10. if err != nil: return error "cannot open: <path>"
11. defer f.Close()
12. header := make([]byte, 12)
13. n, _ := f.Read(header)
14. header = header[:n]
15. if !matchesMagic(ext, header): return error "magic bytes mismatch for .<ext>: <path>"
16. if size > photoMaxSize: return SendAsDocument, size, nil
17. return SendAsPhoto, size, nil
```

### 1.10 Function: `matchesMagic`

```go
// matchesMagic checks whether the given file header matches the expected
// magic bytes for the extension. Returns true for SVG (no magic check)
// and for extensions not in the magicBytes table (permissive fallback).
func matchesMagic(ext string, header []byte) bool
```

**Algorithm**:

```
1. if ext == "svg": return true (no magic check for text format)
2. if ext == "gif":
       return bytes.HasPrefix(header, []byte("GIF87a")) ||
              bytes.HasPrefix(header, []byte("GIF89a"))
3. if ext == "webp":
       return len(header) >= 12 &&
              bytes.HasPrefix(header, []byte("RIFF")) &&
              bytes.Equal(header[8:12], []byte("WEBP"))
4. sig, ok := magicBytes[ext]
   if !ok: return true  // unknown extension, permissive
5. return bytes.HasPrefix(header, sig)
```

### 1.11 Function: `extractCaption`

```go
// extractCaption builds a caption string from the terminal line where an
// image path was detected. The caption is HTML-escaped and truncated to
// Telegram's 1024-character limit.
//
// If the line is longer than maxCaptionLen, the caption is trimmed to the
// path plus 50 characters of surrounding context on each side, with "..."
// ellipsis indicators.
//
// Parameters:
//   - line: the raw terminal line containing the path
//   - pathStart: byte index where the matched path begins in line
//   - pathEnd: byte index where the matched path ends in line
func extractCaption(line string, pathStart, pathEnd int) string
```

**Algorithm**:

```
1. trimmed := strings.TrimSpace(line)
2. escaped := html.EscapeString(trimmed)
3. if len(escaped) <= maxCaptionLen: return escaped
4. // Line is too long. Extract a window around the path.
   contextRadius := 50
   windowStart := pathStart - contextRadius
   if windowStart < 0: windowStart = 0
   windowEnd := pathEnd + contextRadius
   if windowEnd > len(trimmed): windowEnd = len(trimmed)
5. excerpt := trimmed[windowStart:windowEnd]
6. if windowStart > 0: excerpt = "..." + excerpt
7. if windowEnd < len(trimmed): excerpt = excerpt + "..."
8. return html.EscapeString(excerpt)  // re-escape the excerpt
```

**Note on index mapping**: `pathStart` and `pathEnd` are byte indices into the original `line` (pre-trim). The `extractCaption` function works on the trimmed line, so it adjusts indices by the number of leading whitespace bytes trimmed. However, for simplicity in the MVP, we compute `pathStart`/`pathEnd` against the already-trimmed line in `DetectImagePaths` (which trims each line before regex matching). This avoids index translation entirely.

### 1.12 Function: `resolvePath`

```go
// resolvePath resolves a detected path string to an absolute path.
//
// Resolution rules:
//   - Absolute paths (/foo/bar.png): returned as-is after filepath.Clean
//   - Home-relative (~/ prefix): expanded via os.UserHomeDir()
//   - Relative paths (./foo, ../foo, foo/bar): joined with repoDir
//
// Returns the cleaned absolute path. Does NOT verify the file exists --
// that is the caller's responsibility (via validateImage).
func resolvePath(detected, repoDir string) string
```

**Algorithm**:

```
1. detected = strings.TrimSpace(detected)
2. if strings.HasPrefix(detected, "~/"):
       home, err := os.UserHomeDir()
       if err != nil: return filepath.Clean(detected) // best effort
       return filepath.Clean(filepath.Join(home, detected[2:]))
3. if filepath.IsAbs(detected):
       return filepath.Clean(detected)
4. // Relative path: resolve against repo directory.
   return filepath.Clean(filepath.Join(repoDir, detected))
```

### 1.13 Function: `DetectImagePaths`

```go
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
func DetectImagePaths(lines []string, repoDir string, dedup *ImageDedup, project string) []ImageRef
```

**Algorithm**:

```
 1. var refs []ImageRef
 2. seen := make(map[string]bool)  // local dedup within this call (same path in multiple lines)
 3. for _, line := range lines:
 4.     trimmed := strings.TrimSpace(line)
 5.     if trimmed == "": continue
 6.     matches := imagePathRe.FindAllStringSubmatchIndex(trimmed, -1)
 7.     for _, match := range matches:
 8.         // Group 1 is the path (indices match[2]:match[3])
 9.         // The full match includes the extension, which is outside group 1.
10.         // We need the full matched path including extension.
11.         // Reconstruct: the path is from match[2] to the end of the extension.
12.         // Actually, the regex captures everything up to but excluding the
13.         // extension dot in group 1, then the extension is matched outside.
14.         //
15.         // Revised approach: capture the entire path+extension in one group.
16.         // See regex revision in section 1.5.1.
17.         rawPath := trimmed[match[2]:match[3]]
18.         absPath := resolvePath(rawPath, repoDir)
19.         if seen[absPath]: continue
20.         seen[absPath] = true
21.         mode, _, err := validateImage(absPath)
22.         if err != nil:
23.             logging.Debug("telegram: image validation failed", "path", absPath, "err", err)
24.             continue
25.         info, _ := os.Stat(absPath)  // already validated, Stat won't fail
26.         if dedup != nil && !dedup.ShouldSend(project, absPath, info.ModTime()):
27.             logging.Debug("telegram: image deduplicated", "path", absPath)
28.             continue
29.         caption := extractCaption(trimmed, match[2], match[3])
30.         refs = append(refs, ImageRef{
31.             AbsPath:  absPath,
32.             Caption:  caption,
33.             SendMode: mode,
34.         })
35. return refs
```

**Performance**: For a typical 40-line terminal screen, this runs the compiled regex against each non-empty line. Regex matching on short strings (< 200 chars) is sub-microsecond. The `os.Stat` + `os.Open` + 12-byte read for validation is bounded by the number of matches (typically 0-3 per screen). Total expected cost: < 1ms per `sendTelegramEvent` call.

### 1.14 Revised Regex (captures path + extension in one group)

The regex in section 1.5 captures the path body in group 1 but the extension is matched outside the group. This complicates index extraction. Revised to capture the complete path including extension:

```go
var imagePathRe = regexp.MustCompile(
    `(?:^|[\s"'=(,])` +
    `(` +
        `~?` +
        `(?:\.{0,2}/)?` +
        `[^\s"'<>|*?]+/` +
        `[^\s"'<>|*?/]+` +
        `\.(?i:png|jpe?g|gif|webp|svg)` +
    `)` +
    `(?:[\s"'),:;.\]}]|$)`,
)
```

Now group 1 (`match[2]:match[3]`) contains the full path including the extension. The `pathStart` and `pathEnd` for caption extraction map directly to `match[2]` and `match[3]`.


## 2. Modified File: `internal/telegram/bridge.go`

### 2.1 Extended `Event` struct

Add the `Images` field after the existing `Screen` field:

```go
// Event is sent from the TUI to the Telegram bot when a state transition
// occurs for a session.
type Event struct {
    Project   string      // project name (used for topic lookup)
    SessionID string      // session ID (e.g. "proj" or "proj (2)")
    Kind      EventKind
    Detail    string      // human-readable description from attention detection
    Screen    []string    // current visible terminal lines
    Images    []ImageRef  // detected image files to send before text (may be nil)
}
```

**No other changes to `bridge.go`**: The `shouldSend` function, `screenFingerprint`, and `sentRecord` are unchanged. Image dedup is handled by `ImageDedup` in `image.go`, not by the bridge's text-level dedup. This separation is intentional: text dedup compares screen content fingerprints at a 3-second interval, while image dedup compares file paths + modification times at a 5-minute TTL. Mixing these concerns would couple unrelated dedup strategies.

**Important implication**: When `shouldSend` returns false (text is duplicate or rate-limited), images attached to that event are also dropped. This is correct behavior -- if the screen content hasn't changed, the same image paths would be detected again, and `ImageDedup.ShouldSend` would reject them anyway (same path, same mtime, within TTL). The bridge-level rate limit acts as a coarse first filter; `ImageDedup` is the precise second filter.


## 3. Modified File: `internal/telegram/bot.go`

### 3.1 New Method: `rawMultipartCall`

```go
// rawMultipartCall makes a multipart/form-data POST request to the Telegram
// Bot API. This is the file-upload counterpart to rawAPICall (which uses
// application/json). Used by sendPhotoToTopic and sendDocumentToTopic.
//
// Parameters:
//   - method: Telegram Bot API method name (e.g. "sendPhoto", "sendDocument")
//   - fields: string key-value pairs for non-file form fields
//     (chat_id, message_thread_id, caption, parse_mode)
//   - fileField: the form field name for the file ("photo" or "document")
//   - filePath: absolute path to the file on disk
//
// The entire file is buffered in memory before sending. This is acceptable
// because files are pre-validated to be <= 50MB (documentMaxSize), and typical
// agent screenshots are < 1MB. Streaming via io.Pipe is not worth the
// complexity for this size range.
//
// Error handling mirrors rawAPICall: returns an error for HTTP errors,
// Telegram API errors ({"ok": false}), and file I/O errors.
func (b *Bot) rawMultipartCall(method string, fields map[string]string, fileField, filePath string) error
```

**Algorithm**:

```
 1. f, err := os.Open(filePath)
 2. if err != nil: return fmt.Errorf("open %s: %w", filePath, err)
 3. defer f.Close()
 4.
 5. var buf bytes.Buffer
 6. w := multipart.NewWriter(&buf)
 7.
 8. // Write string fields first (order doesn't matter for multipart).
 9. for key, val := range fields:
10.     if err := w.WriteField(key, val); err != nil:
11.         return fmt.Errorf("write field %s: %w", key, err)
12.
13. // Create the file part.
14. part, err := w.CreateFormFile(fileField, filepath.Base(filePath))
15. if err != nil: return fmt.Errorf("create form file: %w", err)
16.
17. // Copy file contents into the multipart body.
18. if _, err := io.Copy(part, f); err != nil:
19.     return fmt.Errorf("copy file: %w", err)
20.
21. // Finalize the multipart message (writes the closing boundary).
22. if err := w.Close(); err != nil:
23.     return fmt.Errorf("close multipart writer: %w", err)
24.
25. // POST to Telegram.
26. apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s",
27.     url.PathEscape(b.token), method)
28. resp, err := http.Post(apiURL, w.FormDataContentType(), &buf)
29. if err != nil: return fmt.Errorf("API %s: %w", method, err)
30. defer resp.Body.Close()
31.
32. respBody, err := io.ReadAll(resp.Body)
33. if err != nil: return fmt.Errorf("API %s: read body: %w", method, err)
34.
35. if resp.StatusCode != http.StatusOK:
36.     return fmt.Errorf("API %s returned %d: %s", method, resp.StatusCode, string(respBody))
37.
38. // Check Telegram's {"ok": false} error envelope.
39. var result struct {
40.     OK          bool   `json:"ok"`
41.     Description string `json:"description"`
42. }
43. if err := json.Unmarshal(respBody, &result); err == nil && !result.OK:
44.     return fmt.Errorf("API %s: %s", method, result.Description)
45.
46. return nil
```

**New imports required**: `mime/multipart`, `path/filepath` (already imported in bot.go for downloadFile -- confirm), `io` (already imported).

Actually, `mime/multipart` is the only new import. `bytes`, `io`, `fmt`, `net/http`, `net/url`, `encoding/json` are all already imported in `bot.go`.

### 3.2 New Method: `sendPhotoToTopic`

```go
// sendPhotoToTopic uploads a local image file to a Forum Topic using
// Telegram's sendPhoto API. The image gets an inline preview in the chat.
// Use for raster images (PNG, JPEG, GIF, WebP) under 10MB.
func (b *Bot) sendPhotoToTopic(topicID int, filePath, caption string) error {
    fields := map[string]string{
        "chat_id":           strconv.FormatInt(b.cfg.ChatID, 10),
        "message_thread_id": strconv.Itoa(topicID),
        "caption":           caption,
        "parse_mode":        "HTML",
    }
    return b.rawMultipartCall("sendPhoto", fields, "photo", filePath)
}
```

### 3.3 New Method: `sendDocumentToTopic`

```go
// sendDocumentToTopic uploads a local file to a Forum Topic using
// Telegram's sendDocument API. The file is sent at original quality.
// Use for SVG files or images exceeding 10MB.
func (b *Bot) sendDocumentToTopic(topicID int, filePath, caption string) error {
    fields := map[string]string{
        "chat_id":           strconv.FormatInt(b.cfg.ChatID, 10),
        "message_thread_id": strconv.Itoa(topicID),
        "caption":           caption,
        "parse_mode":        "HTML",
    }
    return b.rawMultipartCall("sendDocument", fields, "document", filePath)
}
```

### 3.4 Modified Method: `sendEvent`

The existing `sendEvent` method is modified to send images before text messages. The complete revised method:

```go
func (b *Bot) sendEvent(e Event) {
    topicID := b.state.Get(e.Project)
    if topicID == 0 {
        logging.Debug("telegram: no topic for project", "project", e.Project)
        return
    }

    // ── NEW: Send images first (supplementary context). ──
    //
    // Images are sent before text so users see visual context (screenshots,
    // charts) before the action prompt (permission buttons, question options).
    // Errors are logged and skipped -- image failures must never block the
    // critical-path text message with inline keyboards.
    for _, img := range e.Images {
        var err error
        switch img.SendMode {
        case SendAsPhoto:
            err = b.sendPhotoToTopic(topicID, img.AbsPath, img.Caption)
        case SendAsDocument:
            err = b.sendDocumentToTopic(topicID, img.AbsPath, img.Caption)
        }
        if err != nil {
            logging.Error("telegram: failed to send image",
                "project", e.Project,
                "path", img.AbsPath,
                "mode", img.SendMode,
                "err", err,
            )
            // Continue with remaining images and text. Do not return.
        }
    }

    // ── EXISTING: Send text messages (critical path, unchanged). ──
    var messages []string
    var keyboard *tgbotapi.InlineKeyboardMarkup

    switch e.Kind {
    case EventResponse:
        messages = FormatResponse(e.Project, e.Screen)
    case EventPermission:
        messages = FormatPermission(e.Project, e.Detail, e.Screen)
        kb := PermissionKeyboard(e.Project)
        keyboard = &kb
    case EventQuestion:
        messages = FormatQuestion(e.Project, e.Screen)
        options := ParseQuestionOptions(e.Screen)
        if len(options) > 0 {
            kb := QuestionKeyboard(e.Project, options)
            keyboard = &kb
        }
    case EventAttention:
        messages = FormatAttention(e.Project, e.Detail, e.Screen)
    case EventError:
        messages = FormatError(e.Project, e.Detail, e.Screen)
        kb := ErrorKeyboard(e.Project)
        keyboard = &kb
    case EventDone:
        messages = FormatDone(e.Project, e.Screen)
    }

    if len(messages) == 0 {
        // Even if there are no text messages, we may have sent images above.
        // Record the send if we sent at least one image.
        if len(e.Images) > 0 {
            b.recordSendOK()
        }
        return
    }

    for i, text := range messages {
        var kb *tgbotapi.InlineKeyboardMarkup
        if i == len(messages)-1 {
            kb = keyboard
        }
        if err := b.sendToTopic(topicID, text, kb); err != nil {
            logging.Error("telegram: failed to send message", "project", e.Project, "err", err)
            return
        }
    }

    b.recordSendOK()
    logging.Debug("telegram: sent message",
        "project", e.Project,
        "kind", e.Kind,
        "parts", len(messages),
        "images", len(e.Images),
    )
}
```

**Changes from existing `sendEvent`**:

1. New image-sending loop before the `switch` block (lines with `NEW` comment above).
2. After `if len(messages) == 0`, call `recordSendOK()` if images were sent (otherwise the function returns without recording success even though images were delivered).
3. Added `"images"` field to the final debug log.


## 4. Modified File: `internal/tui/app.go`

### 4.1 New Field on `App`

```go
type App struct {
    // ... existing fields ...

    // imageDedup tracks which images have been sent to Telegram to prevent
    // re-sending the same file on consecutive attention ticks. Initialized
    // lazily on the first sendTelegramEvent call when telegramCh is set.
    imageDedup *telegram.ImageDedup

    // ... existing fields ...
}
```

**Placement**: After the `telegramCh` field (line 111 in the current file), before `onProjectAdded`. This groups Telegram-related fields together.

**Initialization**: Lazy, on first use in `sendTelegramEvent`. This avoids allocating the dedup map when Telegram is not configured:

```go
if a.imageDedup == nil {
    a.imageDedup = telegram.NewImageDedup(telegram.DefaultImageDedupTTL)
}
```

Alternatively, initialize in `NewApp` (or wherever `telegramCh` is set). Lazy initialization is preferred because `telegramCh` may be set after `NewApp` returns (via `SetTelegramChannel`). The first `sendTelegramEvent` call is the earliest point where we know Telegram is configured (because the function returns early if `telegramCh == nil`).

### 4.2 Modified Function: `sendTelegramEvent`

The complete revised function:

```go
// sendTelegramEvent sends an event to the Telegram bridge channel if configured.
// Non-blocking; the bridge handles dedup and rate limiting.
// Screen lines are filtered through the agent adapter's ScreenFilter (if any)
// to extract only the conversation area before sending.
func (a *App) sendTelegramEvent(project, sessionID string, state SessionState, detail string, lines []string) {
    if a.telegramCh == nil {
        return
    }

    kind := stateToEventKind(state)
    if kind < 0 {
        return
    }

    // Filter screen lines through the agent adapter to remove sidebar noise,
    // strip the fixed header row, and remove content-aware chrome lines
    // (status bar, model selector, shortcut hints) that aren't conversation
    // content.
    //
    // Only the top skip from ChromeSkipRows is applied here. The bottom skip
    // is intentionally omitted because FilterChromeLines already handles all
    // bottom chrome (status bar, shortcut hints, "esc interrupt" footer), and
    // applying a fixed bottom skip would strip dialog footers ("enter submit
    // esc dismiss") that ParseQuestionOptions needs to find inline keyboard
    // buttons.
    var repoDir string
    if s := a.mgr.GetSession(sessionID); s != nil {
        repoDir = s.Project.Repo
        lines = agent.FilterScreen(s.Project.Agent, lines)
        top, _ := agent.ChromeSkipRows(s.Project.Agent)
        if top > 0 && top < len(lines) {
            lines = lines[top:]
        }
        lines = agent.FilterChromeLines(s.Project.Agent, lines)
    }

    // Detect image paths in the filtered screen lines.
    var images []telegram.ImageRef
    if repoDir != "" {
        if a.imageDedup == nil {
            a.imageDedup = telegram.NewImageDedup(telegram.DefaultImageDedupTTL)
        }
        images = telegram.DetectImagePaths(lines, repoDir, a.imageDedup, project)
    }

    select {
    case a.telegramCh <- telegram.Event{
        Project:   project,
        SessionID: sessionID,
        Kind:      kind,
        Detail:    detail,
        Screen:    lines,
        Images:    images,
    }:
    default:
        // Channel full -- drop the event (bridge dedup will cover next tick).
    }
}
```

**Changes from existing function**:

1. Captured `repoDir` from session (existing `s.Project.Repo` access, just saved to a variable).
2. Added `imageDedup` lazy initialization.
3. Added `DetectImagePaths` call after line filtering.
4. Added `Images: images` to the `Event` struct literal.

**No changes to callers**: The four call sites in `app.go` (lines 1136, 2410, 2418, 2450) remain unchanged. They pass the same arguments; the image detection is internal to `sendTelegramEvent`.


## 5. Test Specifications

All tests use table-driven structure per project conventions. Tests live in `internal/telegram/image_test.go` (new file) and `internal/telegram/bot_test.go` (additions).

### 5.1 `image_test.go`: Path Detection Regex

```go
// TestImagePathRegex verifies the compiled regex against positive and negative
// patterns. Each test case provides a terminal output line and the expected
// extracted paths (may be empty for negative cases).
func TestImagePathRegex(t *testing.T)
```

**Test table**:

```go
tests := []struct {
    name    string
    line    string
    want    []string // expected matched paths
}{
    // ── Positive cases ──
    {
        name: "absolute path",
        line: "Screenshot saved to /tmp/screenshot.png",
        want: []string{"/tmp/screenshot.png"},
    },
    {
        name: "relative path with dot-slash",
        line: "Created ./docs/arch.svg",
        want: []string{"./docs/arch.svg"},
    },
    {
        name: "relative path with double-dot",
        line: `file: "../output/chart.jpeg"`,
        want: []string{"../output/chart.jpeg"},
    },
    {
        name: "home-relative path",
        line: "Saved to ~/screenshots/login.PNG",
        want: []string{"~/screenshots/login.PNG"},
    },
    {
        name: "multiple paths in one line",
        line: "Compare /a/b.png and /c/d.jpg",
        want: []string{"/a/b.png", "/c/d.jpg"},
    },
    {
        name: "path in single quotes",
        line: "Image at '/tmp/test/output.gif'",
        want: []string{"/tmp/test/output.gif"},
    },
    {
        name: "path in double quotes",
        line: `Wrote "/home/user/project/result.webp"`,
        want: []string{"/home/user/project/result.webp"},
    },
    {
        name: "path after equals sign",
        line: "output=/var/data/chart.png",
        want: []string{"/var/data/chart.png"},
    },
    {
        name: "path at start of line",
        line: "/tmp/screenshot.png saved successfully",
        want: []string{"/tmp/screenshot.png"},
    },
    {
        name: "path with spaces in parent dir (quoted)",
        line: `Saved to "/Users/dev/My Project/output/screen.png"`,
        want: []string{"/Users/dev/My"},
        // Note: regex stops at whitespace. Paths with spaces require
        // quoting in agent output, and the regex matches up to the space.
        // This is a known limitation documented in the HLD.
    },
    {
        name: "jpeg extension",
        line: "Created ./output/photo.jpg",
        want: []string{"./output/photo.jpg"},
    },
    {
        name: "case-insensitive extension JPG",
        line: "Image: /tmp/photo.JPG done",
        want: []string{"/tmp/photo.JPG"},
    },
    {
        name: "case-insensitive extension Png",
        line: "Saved /data/capture.Png to disk",
        want: []string{"/data/capture.Png"},
    },
    {
        name: "path in parentheses",
        line: "image (./output/diagram.svg) created",
        want: []string{"./output/diagram.svg"},
    },
    {
        name: "deeply nested path",
        line: "Created /home/user/projects/myapp/src/assets/images/logo.png",
        want: []string{"/home/user/projects/myapp/src/assets/images/logo.png"},
    },
    {
        name: "bare relative path with subdirectory",
        line: "Output: screenshots/login.png",
        want: []string{"screenshots/login.png"},
    },

    // ── Negative cases ──
    {
        name: "glob pattern with asterisk",
        line: "rm *.png",
        want: nil,
    },
    {
        name: "URL https",
        line: "https://example.com/images/photo.png",
        want: nil,
    },
    {
        name: "URL http",
        line: "http://cdn.example.com/assets/logo.jpg",
        want: nil,
    },
    {
        name: "non-image extension",
        line: "Created /tmp/output.txt",
        want: nil,
    },
    {
        name: "bare filename without directory",
        line: "image.png",
        want: nil,
    },
    {
        name: "glob with question mark",
        line: "ls file?.png",
        want: nil,
    },
    {
        name: "pipe redirect",
        line: "convert input.bmp | output.png",
        want: nil,
    },
    {
        name: "no path at all",
        line: "No image paths here",
        want: nil,
    },
    {
        name: "extension in the middle of a word",
        line: "The loading.png module handles caching",
        want: nil, // no directory separator
    },
    {
        name: "color code not a path",
        line: "color: #png; font: sans",
        want: nil,
    },
    {
        name: "directory path ending with slash",
        line: "cd /tmp/images/",
        want: nil, // no image extension
    },
}
```

**Test body**: For each case, run `imagePathRe.FindAllStringSubmatch(tc.line, -1)`, extract group 1 from each match, and compare against `tc.want`.

### 5.2 `image_test.go`: Path Resolution

```go
// TestResolvePath verifies absolute, relative, and home-relative path resolution.
func TestResolvePath(t *testing.T)
```

**Test table**:

```go
tests := []struct {
    name     string
    detected string
    repoDir  string
    want     string // expected absolute path (after filepath.Clean)
}{
    {
        name:     "absolute path unchanged",
        detected: "/tmp/screenshot.png",
        repoDir:  "/home/user/project",
        want:     "/tmp/screenshot.png",
    },
    {
        name:     "relative dot-slash",
        detected: "./output/chart.png",
        repoDir:  "/home/user/project",
        want:     "/home/user/project/output/chart.png",
    },
    {
        name:     "relative double-dot",
        detected: "../sibling/img.png",
        repoDir:  "/home/user/project",
        want:     "/home/user/sibling/img.png",
    },
    {
        name:     "bare relative",
        detected: "screenshots/login.png",
        repoDir:  "/home/user/project",
        want:     "/home/user/project/screenshots/login.png",
    },
    {
        name:     "home-relative path",
        detected: "~/screenshots/img.png",
        repoDir:  "/home/user/project",
        // want depends on runtime os.UserHomeDir(); test uses a known home dir
        // or skips exact match and checks HasPrefix(home)
    },
    {
        name:     "path with redundant slashes cleaned",
        detected: "./output//chart.png",
        repoDir:  "/home/user/project",
        want:     "/home/user/project/output/chart.png",
    },
}
```

**Note on `~/` test**: The home-relative test case checks that the result has the `os.UserHomeDir()` prefix and ends with `screenshots/img.png`, rather than asserting an exact path (which varies per machine).

### 5.3 `image_test.go`: File Validation

```go
// TestValidateImage uses temporary files with crafted headers to test the
// validation pipeline (magic bytes, size checks, send mode selection).
func TestValidateImage(t *testing.T)
```

**Test table**:

```go
tests := []struct {
    name     string
    filename string        // filename with extension (for temp file)
    content  []byte        // file content (header bytes + optional padding)
    size     int64         // if > 0, override file size by writing this many bytes
    wantMode SendMode
    wantErr  bool
}{
    {
        name:     "valid PNG under 10MB",
        filename: "test.png",
        content:  append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 100)...),
        wantMode: SendAsPhoto,
        wantErr:  false,
    },
    {
        name:     "valid JPEG under 10MB",
        filename: "test.jpg",
        content:  append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 100)...),
        wantMode: SendAsPhoto,
        wantErr:  false,
    },
    {
        name:     "valid GIF87a",
        filename: "test.gif",
        content:  append([]byte("GIF87a"), make([]byte, 100)...),
        wantMode: SendAsPhoto,
        wantErr:  false,
    },
    {
        name:     "valid GIF89a",
        filename: "test.gif",
        content:  append([]byte("GIF89a"), make([]byte, 100)...),
        wantMode: SendAsPhoto,
        wantErr:  false,
    },
    {
        name:     "valid WebP",
        filename: "test.webp",
        content:  append([]byte("RIFF\x00\x00\x00\x00WEBP"), make([]byte, 100)...),
        wantMode: SendAsPhoto,
        wantErr:  false,
    },
    {
        name:     "SVG always document",
        filename: "test.svg",
        content:  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`),
        wantMode: SendAsDocument,
        wantErr:  false,
    },
    {
        name:     "PNG wrong magic bytes",
        filename: "test.png",
        content:  []byte("this is not a PNG file, just text content here"),
        wantErr:  true,
    },
    {
        name:     "zero-byte file",
        filename: "test.png",
        content:  []byte{},
        wantErr:  true,
    },
    {
        name:     "file does not exist",
        filename: "", // signal to test body: do not create a file
        wantErr:  true,
    },
}
```

**Large file tests** (separate, to avoid allocating 10MB+ in the table):

```go
func TestValidateImage_LargePhoto(t *testing.T)
// Creates a temp file with valid PNG header + 11MB of zero padding.
// Expects SendAsDocument (>10MB) and no error.

func TestValidateImage_TooLarge(t *testing.T)
// Creates a temp file with valid PNG header + 51MB of zero padding.
// Expects an error ("file too large").
```

**Test body pattern**: For each case, create a temp file with `os.CreateTemp`, write `content`, close, call `validateImage(path)`, assert `wantMode` and `wantErr`. Use `t.TempDir()` for automatic cleanup.

### 5.4 `image_test.go`: Dedup Logic

```go
// TestImageDedup verifies the dedup state machine: first occurrence passes,
// repeats are blocked, new mtimes pass, TTL expiry resets, and different
// projects are independent.
func TestImageDedup(t *testing.T)
```

**Test table**:

```go
tests := []struct {
    name     string
    setup    func(d *ImageDedup)                    // pre-populate state
    project  string
    absPath  string
    mtime    time.Time
    wantSend bool
}{
    {
        name:     "first occurrence passes",
        setup:    nil,
        project:  "myproject",
        absPath:  "/tmp/screenshot.png",
        mtime:    time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC),
        wantSend: true,
    },
    {
        name: "same path same mtime blocked",
        setup: func(d *ImageDedup) {
            d.ShouldSend("myproject", "/tmp/screenshot.png",
                time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC))
        },
        project:  "myproject",
        absPath:  "/tmp/screenshot.png",
        mtime:    time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC),
        wantSend: false,
    },
    {
        name: "same path new mtime passes",
        setup: func(d *ImageDedup) {
            d.ShouldSend("myproject", "/tmp/screenshot.png",
                time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC))
        },
        project:  "myproject",
        absPath:  "/tmp/screenshot.png",
        mtime:    time.Date(2026, 4, 11, 12, 5, 0, 0, time.UTC), // 5 min later
        wantSend: true,
    },
    {
        name: "different project same path passes",
        setup: func(d *ImageDedup) {
            d.ShouldSend("project-a", "/tmp/screenshot.png",
                time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC))
        },
        project:  "project-b",
        absPath:  "/tmp/screenshot.png",
        mtime:    time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC),
        wantSend: true,
    },
}
```

**TTL expiry test** (separate, requires time manipulation):

```go
func TestImageDedup_TTLExpiry(t *testing.T) {
    // Create dedup with a very short TTL (1ms).
    d := NewImageDedup(1 * time.Millisecond)
    mtime := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)

    // First call: should send.
    if !d.ShouldSend("proj", "/tmp/img.png", mtime) {
        t.Fatal("expected first call to pass")
    }

    // Immediately: should be blocked.
    if d.ShouldSend("proj", "/tmp/img.png", mtime) {
        t.Fatal("expected immediate repeat to be blocked")
    }

    // Wait for TTL to expire.
    time.Sleep(5 * time.Millisecond)

    // After expiry: should send again.
    if !d.ShouldSend("proj", "/tmp/img.png", mtime) {
        t.Fatal("expected post-TTL call to pass")
    }
}
```

**Cleanup test**:

```go
func TestImageDedup_LazyCleanup(t *testing.T) {
    // Create dedup with 1ms TTL, insert 100 entries, wait for expiry,
    // call ShouldSend once (triggers cleanup), verify map size is 1.
    d := NewImageDedup(1 * time.Millisecond)
    base := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
    for i := 0; i < 100; i++ {
        d.ShouldSend("proj", fmt.Sprintf("/tmp/img%d.png", i),
            base.Add(time.Duration(i)*time.Nanosecond))
    }
    time.Sleep(5 * time.Millisecond)
    d.ShouldSend("proj", "/tmp/new.png", base)

    d.mu.Lock()
    count := len(d.sent)
    d.mu.Unlock()
    if count != 1 {
        t.Errorf("expected 1 entry after cleanup, got %d", count)
    }
}
```

### 5.5 `image_test.go`: Caption Extraction

```go
// TestExtractCaption verifies caption generation from terminal lines.
func TestExtractCaption(t *testing.T)
```

**Test table**:

```go
tests := []struct {
    name      string
    line      string
    pathStart int
    pathEnd   int
    want      string // expected caption (HTML-escaped)
}{
    {
        name:      "short line unchanged",
        line:      "Screenshot saved to /tmp/s.png",
        pathStart: 20,
        pathEnd:   30,
        want:      "Screenshot saved to /tmp/s.png",
    },
    {
        name:      "HTML special chars escaped",
        line:      "Saved <img> to /tmp/s.png & done",
        pathStart: 15,
        pathEnd:   25,
        want:      "Saved &lt;img&gt; to /tmp/s.png &amp; done",
    },
    {
        name:      "long line truncated around path",
        line:      strings.Repeat("x", 600) + "/tmp/screenshot.png" + strings.Repeat("y", 600),
        pathStart: 600,
        pathEnd:   619,
        // Expected: "..." + 50 x's + path + 50 y's + "..."
        // Total well under 1024 after escaping.
    },
}
```

**Note**: The long-line test asserts that the result length is <= `maxCaptionLen` and contains the path substring, rather than asserting exact content (which is brittle with ellipsis positioning).

### 5.6 `image_test.go`: Integration -- `DetectImagePaths`

```go
// TestDetectImagePaths_Integration creates real files on disk and verifies
// the full pipeline: regex match -> path resolution -> validation -> dedup.
func TestDetectImagePaths_Integration(t *testing.T)
```

**Setup**: Use `t.TempDir()` as the repo directory. Create valid image files with correct headers.

**Test cases**:

```go
tests := []struct {
    name      string
    lines     []string
    files     map[string][]byte // relative path -> content (created in tempdir)
    project   string
    wantPaths []string // expected AbsPath values in returned refs
}{
    {
        name:  "detects absolute path to existing PNG",
        lines: []string{"Screenshot saved to <TEMPDIR>/output/screen.png"},
        files: map[string][]byte{
            "output/screen.png": pngHeader(200), // helper: 8-byte PNG header + padding
        },
        project:   "proj",
        wantPaths: []string{"<TEMPDIR>/output/screen.png"},
    },
    {
        name:  "detects relative path resolved against repoDir",
        lines: []string{"Created ./output/chart.png"},
        files: map[string][]byte{
            "output/chart.png": pngHeader(200),
        },
        project:   "proj",
        wantPaths: []string{"<TEMPDIR>/output/chart.png"},
    },
    {
        name:    "skips non-existent file",
        lines:   []string{"Saved to ./output/missing.png"},
        files:   nil,
        project: "proj",
        wantPaths: nil,
    },
    {
        name:  "skips file with wrong magic bytes",
        lines: []string{"Saved to ./output/fake.png"},
        files: map[string][]byte{
            "output/fake.png": []byte("not a png file, just text"),
        },
        project:   "proj",
        wantPaths: nil,
    },
    {
        name:  "SVG detected as document",
        lines: []string{"Diagram: ./docs/arch.svg"},
        files: map[string][]byte{
            "docs/arch.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
        },
        project: "proj",
        wantPaths: []string{"<TEMPDIR>/docs/arch.svg"},
        // Also verify: refs[0].SendMode == SendAsDocument
    },
    {
        name:  "dedup blocks second detection of same file",
        // Call DetectImagePaths twice with same lines and shared dedup.
        // Second call should return empty.
        lines: []string{"Screenshot: ./output/screen.png"},
        files: map[string][]byte{
            "output/screen.png": pngHeader(200),
        },
        project:   "proj",
        wantPaths: nil, // second call
    },
    {
        name: "multiple images in one screen",
        lines: []string{
            "Before: ./output/before.png",
            "After: ./output/after.jpg",
        },
        files: map[string][]byte{
            "output/before.png": pngHeader(200),
            "output/after.jpg":  jpegHeader(200),
        },
        project:   "proj",
        wantPaths: []string{"<TEMPDIR>/output/before.png", "<TEMPDIR>/output/after.jpg"},
    },
}
```

**Helper functions** used in tests:

```go
// pngHeader returns a byte slice with a valid 8-byte PNG header followed
// by n bytes of zero padding. This creates a file that passes magic byte
// validation but is not a valid PNG image (which is fine -- Telegram would
// reject it, but we are testing detection, not Telegram's response).
func pngHeader(padding int) []byte {
    header := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
    return append(header, make([]byte, padding)...)
}

// jpegHeader returns a byte slice with a valid 3-byte JPEG SOI header
// followed by n bytes of zero padding.
func jpegHeader(padding int) []byte {
    header := []byte{0xFF, 0xD8, 0xFF}
    return append(header, make([]byte, padding)...)
}
```

### 5.7 `bot_test.go`: Multipart Upload

```go
// TestRawMultipartCall uses httptest.NewServer to verify that rawMultipartCall
// correctly constructs multipart/form-data requests with the expected fields
// and file content.
func TestRawMultipartCall(t *testing.T)
```

**Test setup**:

```go
func TestRawMultipartCall(t *testing.T) {
    // Track what the server received.
    var receivedFields map[string]string
    var receivedFileName string
    var receivedFileContent []byte
    var receivedContentType string

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        receivedContentType = r.Header.Get("Content-Type")

        // Parse the multipart form.
        if err := r.ParseMultipartForm(50 << 20); err != nil {
            w.WriteHeader(http.StatusBadRequest)
            return
        }

        // Collect string fields.
        receivedFields = make(map[string]string)
        for key, values := range r.MultipartForm.Value {
            if len(values) > 0 {
                receivedFields[key] = values[0]
            }
        }

        // Collect the file.
        for _, files := range r.MultipartForm.File {
            if len(files) > 0 {
                receivedFileName = files[0].Filename
                f, _ := files[0].Open()
                receivedFileContent, _ = io.ReadAll(f)
                f.Close()
            }
        }

        // Respond with Telegram-style success.
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"ok": true, "result": {}}`))
    }))
    defer server.Close()

    // Create a Bot that points to the test server instead of api.telegram.org.
    // This requires rawMultipartCall to accept an overridable base URL.
    // See section 5.7.1 for the base URL approach.

    // Create a temp file to upload.
    tmpDir := t.TempDir()
    filePath := filepath.Join(tmpDir, "test.png")
    fileContent := pngHeader(100)
    os.WriteFile(filePath, fileContent, 0o644)

    // Call rawMultipartCall.
    fields := map[string]string{
        "chat_id":           "12345",
        "message_thread_id": "67",
        "caption":           "Test caption",
        "parse_mode":        "HTML",
    }
    err := b.rawMultipartCall("sendPhoto", fields, "photo", filePath)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Verify.
    if !strings.HasPrefix(receivedContentType, "multipart/form-data") {
        t.Errorf("expected multipart content type, got %s", receivedContentType)
    }
    if receivedFields["chat_id"] != "12345" {
        t.Errorf("expected chat_id=12345, got %s", receivedFields["chat_id"])
    }
    if receivedFields["caption"] != "Test caption" {
        t.Errorf("expected caption, got %s", receivedFields["caption"])
    }
    if receivedFileName != "test.png" {
        t.Errorf("expected filename test.png, got %s", receivedFileName)
    }
    if !bytes.Equal(receivedFileContent, fileContent) {
        t.Error("file content mismatch")
    }
}
```

### 5.7.1 Test Infrastructure: Overridable Base URL

To test `rawMultipartCall` and `rawAPICall` against `httptest.NewServer`, the `Bot` struct needs an overridable API base URL. Add an unexported field:

```go
type Bot struct {
    // ... existing fields ...

    // apiBase overrides the Telegram API base URL for testing.
    // When empty (production), the default "https://api.telegram.org/bot<token>"
    // is used. Tests set this to point at httptest.NewServer.
    apiBase string
}
```

Both `rawAPICall` and `rawMultipartCall` compute the URL as:

```go
func (b *Bot) apiURL(method string) string {
    base := b.apiBase
    if base == "" {
        base = fmt.Sprintf("https://api.telegram.org/bot%s", url.PathEscape(b.token))
    }
    return base + "/" + method
}
```

This is a minimal, non-breaking change: the zero value (`""`) preserves existing behavior. Only test code sets `apiBase`.

### 5.7.2 Error Response Test

```go
func TestRawMultipartCall_ErrorResponse(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusBadRequest)
        w.Write([]byte(`{"ok": false, "description": "Bad Request: wrong file type"}`))
    }))
    defer server.Close()

    // ... create Bot with apiBase = server.URL ...
    // ... create temp file, call rawMultipartCall ...

    if err == nil {
        t.Fatal("expected error for 400 response")
    }
    if !strings.Contains(err.Error(), "400") {
        t.Errorf("error should mention status code: %v", err)
    }
}
```

### 5.7.3 File Not Found Test

```go
func TestRawMultipartCall_FileNotFound(t *testing.T) {
    // No httptest server needed -- os.Open will fail before HTTP.
    b := &Bot{}
    err := b.rawMultipartCall("sendPhoto", nil, "photo", "/nonexistent/file.png")
    if err == nil {
        t.Fatal("expected error for nonexistent file")
    }
    if !strings.Contains(err.Error(), "open") {
        t.Errorf("error should mention file open failure: %v", err)
    }
}
```

### 5.8 `bot_test.go`: `sendEvent` with Images

```go
// TestSendEvent_ImagesBeforeText verifies that sendEvent sends images
// before text messages and that image errors do not block text delivery.
func TestSendEvent_ImagesBeforeText(t *testing.T)
```

This test requires mocking the HTTP layer. Using the `apiBase` override from 5.7.1, set up an `httptest.NewServer` that records the sequence of API calls:

```go
func TestSendEvent_ImagesBeforeText(t *testing.T) {
    var callSequence []string // e.g. ["sendPhoto", "sendMessage"]
    var mu sync.Mutex

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract the method from the URL path.
        parts := strings.Split(r.URL.Path, "/")
        method := parts[len(parts)-1]

        mu.Lock()
        callSequence = append(callSequence, method)
        mu.Unlock()

        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"ok": true, "result": {"message_id": 1}}`))
    }))
    defer server.Close()

    // Create Bot with test server.
    state := newTopicState()
    state.Set("myproject", 42)
    b := &Bot{
        cfg:     config.TelegramConfig{ChatID: 12345},
        state:   state,
        apiBase: server.URL,
    }

    // Create a temp image file.
    tmpDir := t.TempDir()
    imgPath := filepath.Join(tmpDir, "test.png")
    os.WriteFile(imgPath, pngHeader(100), 0o644)

    // Send event with one image and screen content.
    b.sendEvent(Event{
        Project: "myproject",
        Kind:    EventResponse,
        Screen:  []string{"Agent completed the task."},
        Images: []ImageRef{{
            AbsPath:  imgPath,
            Caption:  "Screenshot",
            SendMode: SendAsPhoto,
        }},
    })

    // Verify order: sendPhoto before sendMessage.
    if len(callSequence) < 2 {
        t.Fatalf("expected at least 2 API calls, got %d", len(callSequence))
    }
    if callSequence[0] != "sendPhoto" {
        t.Errorf("expected first call to be sendPhoto, got %s", callSequence[0])
    }
    if callSequence[1] != "sendMessage" {
        t.Errorf("expected second call to be sendMessage, got %s", callSequence[1])
    }
}
```

### 5.8.1 Image Error Does Not Block Text

```go
func TestSendEvent_ImageErrorDoesNotBlockText(t *testing.T) {
    var callSequence []string
    var mu sync.Mutex

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        parts := strings.Split(r.URL.Path, "/")
        method := parts[len(parts)-1]

        mu.Lock()
        callSequence = append(callSequence, method)
        mu.Unlock()

        if method == "sendPhoto" {
            // Simulate image upload failure.
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusBadRequest)
            w.Write([]byte(`{"ok": false, "description": "Bad Request: wrong file type"}`))
            return
        }

        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"ok": true, "result": {"message_id": 1}}`))
    }))
    defer server.Close()

    state := newTopicState()
    state.Set("myproject", 42)
    b := &Bot{
        cfg:     config.TelegramConfig{ChatID: 12345},
        state:   state,
        apiBase: server.URL,
    }

    tmpDir := t.TempDir()
    imgPath := filepath.Join(tmpDir, "test.png")
    os.WriteFile(imgPath, pngHeader(100), 0o644)

    b.sendEvent(Event{
        Project: "myproject",
        Kind:    EventPermission,
        Detail:  "Write file: main.go",
        Screen:  []string{"Permission needed"},
        Images: []ImageRef{{
            AbsPath:  imgPath,
            Caption:  "Screenshot",
            SendMode: SendAsPhoto,
        }},
    })

    // Text message should still be sent despite image failure.
    hasMessage := false
    for _, call := range callSequence {
        if call == "sendMessage" {
            hasMessage = true
        }
    }
    if !hasMessage {
        t.Error("sendMessage was not called after image failure")
    }
}
```


## 6. Complete File Inventory

### 6.1 New Files

| File | Lines (est.) | Purpose |
|---|---|---|
| `internal/telegram/image.go` | ~250 | `SendMode`, `ImageRef`, `ImageDedup`, `DetectImagePaths`, `validateImage`, `matchesMagic`, `resolvePath`, `extractCaption`, `imagePathRe`, size/magic constants |
| `internal/telegram/image_test.go` | ~450 | Table-driven tests for regex, resolution, validation, dedup, caption extraction, and the full `DetectImagePaths` integration |

### 6.2 Modified Files

| File | Lines changed (est.) | Changes |
|---|---|---|
| `internal/telegram/bot.go` | ~90 added, ~5 modified | `rawMultipartCall` (~45 lines), `sendPhotoToTopic` (~10 lines), `sendDocumentToTopic` (~10 lines), `apiURL` helper (~7 lines), `apiBase` field (1 line), `sendEvent` modifications (~15 lines changed), new import `mime/multipart` |
| `internal/telegram/bridge.go` | 1 line added | `Images []ImageRef` field on `Event` struct |
| `internal/tui/app.go` | ~10 added, ~3 modified | `imageDedup` field on `App` (1 line), lazy init + `DetectImagePaths` call in `sendTelegramEvent` (~8 lines), `Images` field in `Event` literal (1 line) |
| `internal/telegram/bot_test.go` | ~150 added | `TestRawMultipartCall`, `TestRawMultipartCall_ErrorResponse`, `TestRawMultipartCall_FileNotFound`, `TestSendEvent_ImagesBeforeText`, `TestSendEvent_ImageErrorDoesNotBlockText` |

### 6.3 Unchanged Files

| File | Reason |
|---|---|
| `internal/telegram/handler.go` | Inbound-only. Outbound images do not affect inbound handling. |
| `internal/telegram/formatter.go` | Text formatting unchanged. Image captions constructed in `image.go`. |
| `internal/telegram/bridge.go` (`shouldSend`) | Image dedup handled by `ImageDedup`. Bridge text dedup unchanged. |
| `internal/tui/messages.go` | No new Bubble Tea message types. Image sending is fire-and-forget. |
| `internal/tui/statusbar.go` | No keybinding hint (no manual screenshot in MVP). |


## 7. Import Dependency Map

```
internal/telegram/image.go imports:
    bytes
    fmt
    html
    os
    path/filepath
    regexp
    strconv
    strings
    sync
    time
    github.com/openconductorhq/openconductor/internal/logging

internal/telegram/bot.go adds:
    mime/multipart   (new)
    path/filepath    (already imported? -- check. Not currently imported in bot.go.
                      Actually, filepath is not used in bot.go currently. It is used
                      in handler.go. Need to add it for filepath.Base in rawMultipartCall.)
```

No new external dependencies. All imports are from the Go standard library plus the existing `internal/logging` package.


## 8. Concurrency and Thread Safety

### 8.1 `ImageDedup`

Protected by `sync.Mutex`. All access goes through `ShouldSend`, which acquires the lock. The `App.imageDedup` field is written once (lazy init in `sendTelegramEvent`) and read on every subsequent call. Since `sendTelegramEvent` is called only from the TUI goroutine (Bubble Tea's `Update` loop), there is no data race on the `App.imageDedup` pointer itself. The mutex inside `ImageDedup` protects the `sent` map against future concurrent callers.

### 8.2 `rawMultipartCall`

Stateless -- reads only from `Bot.token`, `Bot.apiBase`, and `Bot.cfg.ChatID`, all of which are immutable after `NewBot`. Uses `http.Post` which creates a new HTTP request per call. No shared mutable state.

### 8.3 File Access

`DetectImagePaths` calls `os.Stat` and `os.Open` on potentially shared files. These are read-only operations. If an agent deletes a file between `DetectImagePaths` (in `sendTelegramEvent` on the TUI goroutine) and `rawMultipartCall` (in `bridgeLoop` on the bridge goroutine), `rawMultipartCall`'s `os.Open` returns an error, which is logged and skipped. This TOCTOU gap is documented in HLD section 9.2 ("File deleted between detection and send").


## 9. Error Taxonomy

Every error in the image pipeline is classified as one of:

| Category | Handling | Log level |
|---|---|---|
| File not found at detection time | Skip, continue scanning | Debug |
| File is directory / not regular | Skip | Debug |
| File is 0 bytes | Skip | Debug |
| File > 50MB | Skip | Warn |
| Magic bytes mismatch | Skip | Debug |
| File cannot be opened (permissions) | Skip | Warn |
| Dedup: already sent | Skip | Debug |
| `rawMultipartCall`: `os.Open` failure | Skip image, continue with next | Error |
| `rawMultipartCall`: HTTP error | Skip image, continue with next | Error |
| `rawMultipartCall`: Telegram API error (`ok: false`) | Skip image, continue with next | Error |
| Text `sendToTopic` failure after images | Return (existing behavior) | Error |

**Invariant**: No error in the image pipeline causes a panic, blocks the text message, or stops processing of remaining images.
