# Telegram Photo Sending -- High-Level Design

**Feature**: Detect image file paths in agent terminal output and forward them to the project's Telegram Forum Topic via the Telegram Bot API.

**Status**: Draft
**Date**: 2026-04-11


## 1. Resolved Decisions (from User Story)

These decisions are settled and this design conforms to them:

1. **Detection strategy**: Match ANY file path with image extension (.png, .jpg, .jpeg, .gif, .webp, .svg). No keyword prefix required.
2. **No rate limiting on image sends**: Images are sent immediately when detected. The existing 3-second `minSendInterval` does not apply to images.
3. **Sequential `sendPhoto` calls**: Each image is sent as a separate `sendPhoto` or `sendDocument` call, not `sendMediaGroup`.
4. **Forward files as-is**: OpenConductor sends the agent-created file. No VT100-to-PNG terminal rendering.
5. **Embed monospace font**: For potential future terminal screenshot rendering, not used in MVP.
6. **No manual screenshot keybinding**: `Ctrl+Shift+T` terminal screenshot is deferred.


## 2. Component Architecture

### 2.1 New File: `internal/telegram/image.go`

This file owns all image-related logic: path detection, file validation, deduplication, and caption extraction. It has no dependency on TUI, session, or agent packages -- it operates on plain strings (screen lines) and file paths.

**Responsibilities:**

- `DetectImagePaths(lines []string, repoDir string) []ImageRef` -- scan terminal output for image file references
- `ValidateImageFile(path string) (ImageInfo, error)` -- check existence, size, magic bytes
- Image deduplication state (path+mtime keyed, TTL-based expiry)
- Caption extraction from surrounding output context

**Why a new file instead of extending `bridge.go` or `bot.go`:**

The image detection logic is self-contained and testable in isolation. It needs no access to the Bot struct, HTTP client, or bridge channel. Placing it in `bridge.go` would conflate event routing with content analysis. Placing it in `bot.go` would mix API transport with domain logic. A dedicated file follows the same pattern as `formatter.go` (content transformation isolated from transport).

### 2.2 Modified File: `internal/telegram/bridge.go`

The `Event` struct gains an `Images []ImageRef` field to carry detected image references from the TUI to the bridge loop. The bridge's `shouldSend` logic is unchanged -- image dedup is handled separately in `image.go` because images use a different dedup strategy (path+mtime) than text events (screen fingerprint).

### 2.3 Modified File: `internal/telegram/bot.go`

Two new methods on `Bot`:

- `sendPhotoToTopic(topicID int, filePath string, caption string) error` -- multipart upload via `sendPhoto`
- `sendDocumentToTopic(topicID int, filePath string, caption string) error` -- multipart upload via `sendDocument`

Both use a new private helper `rawMultipartCall(method string, fields map[string]string, fileField string, filePath string) error` that constructs `multipart/form-data` requests. This sits alongside the existing `rawAPICall` (JSON POST) as a parallel transport method.

The `sendEvent` method gains a loop before text sending: for each `ImageRef` in the event, it calls the appropriate photo/document method.

### 2.4 Modified File: `internal/tui/app.go`

`sendTelegramEvent` gains image detection after the existing screen-line filtering. The detected paths are resolved relative to the session's `Project.Repo` directory, validated, deduplicated, and attached to the `Event` struct before it is sent to the bridge channel.

### 2.5 Dependency Graph (image-related additions only)

```
tui/app.go
    |
    +-- telegram.DetectImagePaths(lines, repoDir)   # returns []ImageRef
    +-- telegram.Event{Images: refs}                # enriched event
    |
    v
telegram/bridge.go
    |
    v
telegram/bot.go  -->  sendEvent()
    |
    +-- sendPhotoToTopic()   --> rawMultipartCall("sendPhoto", ...)
    +-- sendDocumentToTopic() --> rawMultipartCall("sendDocument", ...)
    +-- sendToTopic()         --> rawAPICall("sendMessage", ...)  [existing]
```

The new code path introduces no new inter-package dependencies. The `telegram` package gains internal complexity (new file, new methods) but its public surface grows by only two types (`ImageRef`, `ImageDedup`) and one function (`DetectImagePaths`).


## 3. Data Types

### 3.1 ImageRef (in `image.go`)

```go
// ImageRef is a validated reference to an image file detected in agent output.
type ImageRef struct {
    AbsPath  string    // absolute path to the image file
    ModTime  time.Time // file modification time (for dedup)
    Size     int64     // file size in bytes
    Format   ImageFormat // png, jpg, gif, webp, svg
    Caption  string    // extracted from surrounding output context
    SendMode SendMode  // photo or document
}
```

### 3.2 ImageFormat and SendMode (in `image.go`)

```go
type ImageFormat int

const (
    FormatPNG  ImageFormat = iota
    FormatJPEG
    FormatGIF
    FormatWebP
    FormatSVG
)

type SendMode int

const (
    SendAsPhoto    SendMode = iota // sendPhoto (raster, <10MB)
    SendAsDocument                 // sendDocument (SVG, or >10MB)
)
```

### 3.3 Extended Event (in `bridge.go`)

```go
type Event struct {
    Project   string
    SessionID string
    Kind      EventKind
    Detail    string
    Screen    []string
    Images    []ImageRef // new: detected image files to send before text
}
```

### 3.4 ImageDedup (in `image.go`)

```go
// ImageDedup tracks which images have already been sent to prevent
// re-sending the same file on consecutive attention ticks.
type ImageDedup struct {
    mu      sync.Mutex
    sent    map[string]time.Time // key: "absPath|mtime" -> when sent
    ttl     time.Duration        // entries expire after this duration
}
```


## 4. Data Flow

### 4.1 Detection Through Sending (Happy Path)

```
Agent writes to PTY: "Screenshot saved to ./screenshots/login.png"
    |
    v
Attention tick fires (every 500ms)
    |
    v
State transition detected (e.g. Working -> Idle)
    |
    v
app.go: sendTelegramEvent(project, sessionID, state, detail, lines)
    |
    +-- Filter screen lines through agent adapter [existing, unchanged]
    |
    +-- [NEW] telegram.DetectImagePaths(filteredLines, session.Project.Repo)
    |       |
    |       +-- Regex scan each line for paths with image extensions
    |       +-- Resolve relative paths against repoDir
    |       +-- os.Stat() each path: exists? readable?
    |       +-- Check magic bytes (PNG header, JPEG SOI, GIF8, etc.)
    |       +-- Check size (<10MB -> photo, 10-50MB -> document, >50MB -> skip)
    |       +-- Dedup check: skip if (absPath, mtime) already sent within TTL
    |       +-- Extract caption from the line containing the path
    |       +-- Return []ImageRef
    |
    +-- Build Event{..., Images: refs}
    |
    +-- Send to telegramCh [existing channel]
    |
    v
bot.go: bridgeLoop receives Event
    |
    v
bot.go: sendEvent(event)
    |
    +-- [NEW] For each image in event.Images:
    |       |
    |       +-- if image.SendMode == SendAsPhoto:
    |       |       sendPhotoToTopic(topicID, image.AbsPath, image.Caption)
    |       |
    |       +-- if image.SendMode == SendAsDocument:
    |       |       sendDocumentToTopic(topicID, image.AbsPath, image.Caption)
    |       |
    |       +-- On error: log and continue (do not abort remaining images or text)
    |
    +-- [EXISTING] For each text message:
    |       sendToTopic(topicID, text, keyboard)
    |
    +-- recordSendOK()
```

### 4.2 Why Detection Runs in the TUI Layer (app.go), Not in bridgeLoop

The detection must run in `sendTelegramEvent` (TUI side), not in `bridgeLoop` (telegram side), for three reasons:

1. **File path resolution needs the repo directory.** `DetectImagePaths` resolves relative paths (e.g. `./screenshots/login.png`) against `session.Project.Repo`. The session object is available in `sendTelegramEvent` (via `a.mgr.GetSession`), but the bridge loop has no access to session state -- it receives opaque `Event` structs.

2. **File validation needs filesystem access at detection time.** `os.Stat`, magic byte checks, and size checks should happen as close to detection as possible, before the event sits in the bridge channel buffer. If detection ran in the bridge loop, the file could be deleted or modified between enqueue and dequeue.

3. **Consistency with existing architecture.** The existing flow already does content transformation in `sendTelegramEvent` (screen filtering, chrome stripping). Adding image detection here follows the established pattern: `sendTelegramEvent` prepares the complete event payload, the bridge transports it, and `sendEvent` delivers it.

The tradeoff is that `sendTelegramEvent` does more work on the TUI goroutine. However, the cost is bounded: regex scanning of ~40 screen lines plus at most a handful of `os.Stat` calls. This is negligible compared to the existing screen filtering work already done in this function.


## 5. Multipart Upload

### 5.1 Why rawAPICall Cannot Be Reused

The existing `rawAPICall` serializes the payload as `application/json`. Telegram's `sendPhoto` and `sendDocument` require the file to be uploaded as `multipart/form-data` when sending a local file (as opposed to a URL or `file_id` reference). These are fundamentally different HTTP content types.

### 5.2 rawMultipartCall Design

```go
func (b *Bot) rawMultipartCall(method string, fields map[string]string, fileField, filePath string) error
```

**Parameters:**
- `method`: Telegram API method name (e.g. `"sendPhoto"`, `"sendDocument"`)
- `fields`: String key-value pairs for non-file form fields (chat_id, message_thread_id, caption, parse_mode)
- `fileField`: The form field name for the file (`"photo"` or `"document"`)
- `filePath`: Absolute path to the file on disk

**Implementation outline:**

1. Open the file (`os.Open`)
2. Create a `multipart.Writer` backed by a `bytes.Buffer`
3. Write each entry in `fields` via `writer.WriteField`
4. Create a form file via `writer.CreateFormFile(fileField, filepath.Base(filePath))`
5. Copy the file contents into the form file writer
6. Close the multipart writer
7. POST to `https://api.telegram.org/bot<token>/<method>` with `Content-Type: multipart/form-data; boundary=...`
8. Parse the response: check HTTP status and `{"ok": true/false}` the same way `rawAPICall` does

**Why a buffer, not streaming:**

Image files in this use case are bounded at 50MB (Telegram's document limit). In practice, agent screenshots and charts are typically under 1MB. Buffering the entire multipart body in memory is simpler and avoids the complexity of `io.Pipe` + goroutine streaming. If future use cases require very large files, streaming can be added as an optimization.

### 5.3 sendPhotoToTopic / sendDocumentToTopic

```go
func (b *Bot) sendPhotoToTopic(topicID int, filePath, caption string) error {
    fields := map[string]string{
        "chat_id":           strconv.FormatInt(b.cfg.ChatID, 10),
        "message_thread_id": strconv.Itoa(topicID),
        "caption":           caption,
        "parse_mode":        "HTML",
    }
    return b.rawMultipartCall("sendPhoto", fields, "photo", filePath)
}

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

These mirror the signature style of `sendToTopic` (topicID, content, optional extras) and follow the same error handling pattern: return the error to the caller, which logs and continues.


## 6. Image Detection

### 6.1 Regex Pattern

```go
var imagePathRe = regexp.MustCompile(
    `(?:^|[\s"'=(])` +                           // boundary: start of line or whitespace/quote/paren
    `(` +
        `/[^\s"'<>|*?]+` +                        // absolute path
        `|` +
        `\.{0,2}/[^\s"'<>|*?]+` +                 // relative path (./foo, ../foo, foo/bar)
    `)` +
    `\.(?i:png|jpe?g|gif|webp|svg)` +             // image extension (case-insensitive)
    `(?:[\s"'),:;]|$)`,                           // trailing boundary
)
```

**Design choices:**

- **No keyword prefix**: Per resolved decision, we match any path with an image extension. This maximizes recall at the cost of potential false positives from `ls` output or log lines. The file-existence check (`os.Stat`) acts as a natural filter: paths that don't exist on disk are silently dropped.
- **Case-insensitive extension**: Agents may produce `.PNG` or `.Jpg` files.
- **Exclusion of shell metacharacters** (`<>|*?`): Prevents matching glob patterns like `*.png` or redirections like `> output.png` (which indicate a command, not a completed file).

### 6.2 Path Resolution

```
Detected path             Resolution
--------------------------+------------------------------------------
/abs/path/to/img.png      Used as-is
./relative/img.png        filepath.Join(repoDir, detected)
../sibling/img.png        filepath.Join(repoDir, detected)
img.png                   filepath.Join(repoDir, detected)
~/screenshots/img.png     os.UserHomeDir() + rest
```

After resolution, the path is cleaned (`filepath.Clean`) and checked with `os.Stat`. The path is NOT restricted to be within the repo directory -- agents legitimately create files in `/tmp`, home directories, and other locations. The `os.Stat` check is sufficient to confirm the file exists and is accessible.

### 6.3 Caption Extraction

The caption is the line containing the detected path, HTML-escaped and truncated to Telegram's 1024-character caption limit. If the line is very long (agent dumped a JSON blob containing a path), it is trimmed to the path and 50 characters of surrounding context on each side.

```go
func extractCaption(line string, pathStart, pathEnd int) string
```


## 7. Image Deduplication

### 7.1 Strategy

Dedup key: `absPath + "|" + modTime.UnixNano()`

This means:
- Same path, same mtime: deduplicated (agent output repeats the same reference)
- Same path, new mtime: re-sent (agent regenerated the file)
- Different path: independent

### 7.2 Location

The `ImageDedup` struct is created once per `App` and passed to `sendTelegramEvent` calls. It is NOT stored in the bridge or bot -- it lives on the TUI side because dedup decisions must happen before the event enters the channel (to avoid buffering duplicate image data).

Concretely, the `App` struct gains a field:

```go
imageDedup *telegram.ImageDedup
```

Initialized in `NewApp` (or lazily on first Telegram event).

### 7.3 TTL and Cleanup

Entries expire after 5 minutes. Cleanup runs lazily: on each `DetectImagePaths` call, entries older than TTL are purged. No background goroutine needed -- the attention tick (every 500ms) drives cleanup naturally.

The 5-minute TTL balances two concerns:
- Too short: agent output scrolls back into view and the same image is re-sent.
- Too long: memory grows unboundedly for sessions producing many images. At ~100 bytes per entry (path string + timestamp), even 1000 images over 5 minutes is 100KB -- negligible.

### 7.4 Per-Session vs Global Dedup

Dedup is global across all sessions, not per-session. If two sessions for different projects reference the same absolute path, the image is sent to each project's topic (different Forum Topics) -- the dedup key includes the project name:

```
key = project + "|" + absPath + "|" + modTime.UnixNano()
```

This ensures each project's Forum Topic gets the image independently, while preventing the same project from sending the same image twice.


## 8. File Validation

### 8.1 Validation Pipeline

```
os.Stat(path)
    |
    +-- not exists / not regular file -> skip (log debug, no error to user)
    |
    v
Check file size
    |
    +-- 0 bytes -> skip (file being created, not yet written)
    +-- > 50MB -> skip (log warning: "image too large for Telegram")
    |
    v
Read first 16 bytes -> check magic number
    |
    +-- PNG:  \x89PNG\r\n\x1a\n
    +-- JPEG: \xFF\xD8\xFF
    +-- GIF:  GIF87a or GIF89a
    +-- WebP: RIFF....WEBP
    +-- SVG:  skip magic check (text file, extension is sufficient)
    +-- No match -> skip (log debug: "file extension does not match content")
    |
    v
Determine SendMode
    |
    +-- SVG -> SendAsDocument (Telegram cannot render SVG inline)
    +-- Size > 10MB -> SendAsDocument
    +-- Raster image <= 10MB -> SendAsPhoto
```

### 8.2 Why Magic Bytes

Agent output may contain paths that coincidentally end in `.png` but are not image files (e.g., a text file named `notes.png`, or a partially-written binary). Magic byte validation prevents sending garbage to Telegram, which would result in an API error (Telegram validates uploads server-side, but the error message is opaque).

The cost is one `os.Open` + 16-byte read per candidate, which is negligible.

SVG files are exempt from magic byte checks because SVG is a text format with variable headers (XML declaration, DOCTYPE, bare `<svg>` tag). Extension-only matching is acceptable here -- if the file is not valid SVG, Telegram's `sendDocument` will still succeed (it sends any file as a document).


## 9. Error Handling

### 9.1 Detection Errors

| Scenario | Handling |
|---|---|
| Path does not exist | Skip silently (debug log). Common: agent printed a path it will create later. |
| Path is a directory | Skip silently. |
| Path is unreadable (permissions) | Skip with warning log. |
| Magic bytes mismatch | Skip with debug log. |
| File is 0 bytes | Skip silently. File may be in the process of being written. |
| Regex produces false positive | os.Stat filters it out. No user-visible effect. |

### 9.2 Send Errors

| Scenario | Handling |
|---|---|
| `sendPhoto` network error | Log error, skip this image, continue with remaining images and text. No retry in MVP. |
| `sendPhoto` returns HTTP 413 (too large) | Should not occur (size is pre-checked). If it does, fall back to `sendDocument`. If that also fails, log and skip. |
| `sendPhoto` returns HTTP 400 (bad file) | Log error with Telegram's error description, skip image. |
| `sendDocument` fails | Log error, skip. Do not block the text message. |
| File deleted between detection and send | `rawMultipartCall` gets `os.Open` error. Log and skip. |
| Bot unhealthy | The entire `sendEvent` path is skipped (existing behavior). Images are not sent. |

### 9.3 Principle: Never Block Text

Image send failures must never prevent the text message (with inline keyboards for permissions/questions) from being delivered. The text message is the critical path for remote agent management. Images are supplementary context.

The implementation enforces this by sending images in a loop with per-image error handling before the existing text-sending loop:

```go
func (b *Bot) sendEvent(e Event) {
    topicID := b.state.Get(e.Project)
    if topicID == 0 { return }

    // Send images first (supplementary, errors are non-fatal).
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
                "project", e.Project, "path", img.AbsPath, "err", err)
        }
    }

    // Send text messages (critical path, existing logic unchanged).
    // ... existing code ...
}
```


## 10. Ordering: Images Before Text

Images are sent before the text message within `sendEvent`. This ordering is intentional: when a user receives a Telegram notification for a permission request that references a screenshot, they see the visual context (the image) before the action prompt (the text with inline keyboard buttons).

If image sending is slow (large file, slow upload), the text message is delayed. This is acceptable because:
- Agent-created images are typically small (screenshots < 1MB, charts < 500KB)
- The delay is bounded by Telegram's upload speed, not by OpenConductor
- Sending text first would mean the user sees the action prompt before the context, which is worse UX

If this becomes a problem in practice, a future optimization could send images and text concurrently using goroutines. This is not worth the complexity in the MVP.


## 11. Modified `sendTelegramEvent` in app.go

The function gains approximately 10 lines of new code between the existing screen filtering and the channel send:

```go
func (a *App) sendTelegramEvent(project, sessionID string, state SessionState, detail string, lines []string) {
    if a.telegramCh == nil {
        return
    }

    kind := stateToEventKind(state)
    if kind < 0 {
        return
    }

    // [EXISTING] Filter screen lines through agent adapter.
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

    // [NEW] Detect image paths in the filtered screen lines.
    var images []telegram.ImageRef
    if repoDir != "" {
        images = telegram.DetectImagePaths(lines, repoDir, a.imageDedup, project)
    }

    select {
    case a.telegramCh <- telegram.Event{
        Project:   project,
        SessionID: sessionID,
        Kind:      kind,
        Detail:    detail,
        Screen:    lines,
        Images:    images,   // new field
    }:
    default:
    }
}
```

The new code is minimal and follows the existing structure. `DetectImagePaths` is a pure function (plus dedup side effect) that does not interact with TUI state.


## 12. File Inventory

### New Files

| File | Purpose |
|---|---|
| `internal/telegram/image.go` | `ImageRef`, `ImageFormat`, `SendMode`, `ImageDedup`, `DetectImagePaths`, `ValidateImageFile`, caption extraction, image path regex, magic byte checks |
| `internal/telegram/image_test.go` | Tests for path detection, validation, dedup, caption extraction |

### Modified Files

| File | Changes |
|---|---|
| `internal/telegram/bot.go` | Add `rawMultipartCall`, `sendPhotoToTopic`, `sendDocumentToTopic`. Modify `sendEvent` to send images before text. |
| `internal/telegram/bridge.go` | Add `Images []ImageRef` field to `Event` struct. |
| `internal/tui/app.go` | Add `imageDedup` field to `App`. Modify `sendTelegramEvent` to call `DetectImagePaths` and populate `Event.Images`. Initialize `imageDedup` in constructor. |

### Unchanged Files

| File | Why unchanged |
|---|---|
| `internal/telegram/handler.go` | Inbound path only. Outbound images do not affect inbound message handling. |
| `internal/telegram/formatter.go` | Text formatting is unchanged. Image captions are constructed in `image.go`. |
| `internal/telegram/bridge.go` (shouldSend) | Image dedup is handled separately in `ImageDedup`. The bridge's text dedup (`screenFingerprint`) continues to work on `Screen` lines, which are unaffected by image detection. |
| `internal/tui/messages.go` | No new Bubble Tea message types needed. Image sending is fire-and-forget within the existing event flow. |
| `internal/tui/statusbar.go` | No keybinding hint needed (no manual screenshot in MVP). |


## 13. Testing Strategy

### 13.1 Unit Tests in `image_test.go`

**Path detection (table-driven):**

| Input line | Expected matches |
|---|---|
| `Screenshot saved to /tmp/screenshot.png` | `/tmp/screenshot.png` |
| `Created ./docs/arch.svg` | `./docs/arch.svg` |
| `file: "../output/chart.jpeg"` | `../output/chart.jpeg` |
| `~/screenshots/login.PNG` | `~/screenshots/login.PNG` |
| `Multiple: /a/b.png and /c/d.jpg` | `/a/b.png`, `/c/d.jpg` |
| `rm *.png` (glob) | no match (contains `*`) |
| `> output.png` (redirect) | no match (contains `>`) |
| `https://example.com/image.png` | no match (URL, not local path) |
| `color: #png; font: sans` | no match |
| `No image paths here` | no match |

**Validation (table-driven):**

| Scenario | Expected |
|---|---|
| Valid PNG file, 500KB | SendAsPhoto |
| Valid JPEG file, 15MB | SendAsDocument (>10MB) |
| Valid SVG file, 2KB | SendAsDocument (SVG always document) |
| File > 50MB | Skipped |
| File does not exist | Skipped |
| File with .png extension, wrong magic bytes | Skipped |
| Zero-byte file | Skipped |

**Dedup (table-driven):**

| Scenario | Expected |
|---|---|
| First occurrence of path+mtime | Not deduped (sent) |
| Same path+mtime within TTL | Deduped (skipped) |
| Same path, new mtime | Not deduped (re-sent) |
| Same path+mtime after TTL expiry | Not deduped (re-sent) |
| Same path, different project | Not deduped (each project gets it) |

**Caption extraction:**

| Input line | Expected caption |
|---|---|
| `Screenshot saved to /tmp/s.png` | `Screenshot saved to /tmp/s.png` |
| Very long line (>1024 chars) with path in middle | Path + 50 chars context each side, truncated |

### 13.2 Unit Tests for `rawMultipartCall` in `bot_test.go`

Use `httptest.NewServer` to verify:
- Correct Content-Type header (`multipart/form-data; boundary=...`)
- All form fields are present
- File content is correctly transmitted
- Error responses are correctly parsed

### 13.3 Integration Test for sendEvent with Images

Mock `rawMultipartCall` (via interface or test override) to verify that `sendEvent` sends images before text, handles image errors without blocking text, and logs appropriately.


## 14. Risks and Mitigations

### 14.1 False Positive Detection

**Risk:** The regex matches paths in `ls` output, git log, or conversation text that happen to end in image extensions.

**Mitigation:** The `os.Stat` + magic byte check pipeline eliminates false positives for non-existent files and misnamed files. For files that genuinely exist and are valid images but were not just created by the agent, the dedup TTL (5 minutes) limits the blast radius: the image is sent at most once per 5 minutes. In practice, paths that appear in agent output and also exist on disk as valid images are overwhelmingly files the agent just created or referenced intentionally.

**Residual risk:** Low. If a false positive image is sent, it is supplementary context next to the text message. The user sees an unexpected image in the thread, which is a minor inconvenience, not a functional failure.

### 14.2 Large File Buffering

**Risk:** `rawMultipartCall` buffers the entire file in memory before sending.

**Mitigation:** The 50MB size cap limits worst-case memory usage. Typical images (< 1MB) use negligible memory. If multiple large images are detected in one event (unlikely), they are processed sequentially, not concurrently, so peak memory is bounded by the single largest file plus multipart overhead.

### 14.3 Telegram API Rate Limits

**Risk:** Sending many images rapidly could hit Telegram's bot rate limits (~30 messages/second per chat, with lower limits per-topic).

**Mitigation:** The MVP does not add rate limiting for images (per resolved decision). In practice, agent-created images are infrequent (a Playwright session might produce 1-3 screenshots per task). If rate limiting becomes needed, it can be added to `sendEvent`'s image loop with a per-topic counter -- the architecture supports this without structural changes.

### 14.4 Path Traversal / Security

**Risk:** A malicious agent could print a path like `/etc/shadow.png` and trick OpenConductor into uploading sensitive files.

**Mitigation:** The magic byte check prevents uploading non-image files with image extensions. For actual image files at sensitive paths (unlikely but possible), this is within the threat model: the user has configured OpenConductor to run this agent and forward its output to Telegram. The agent already has PTY access and can run arbitrary commands. Image forwarding does not expand the attack surface beyond what the agent already has. No path restriction is applied beyond existence and format validation.
