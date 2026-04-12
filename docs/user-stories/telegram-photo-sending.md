# User Story: Send Photos/Images from Agents to Telegram

**Story ID:** US-TGIMG-001
**Feature:** Forward agent-created images and terminal screenshots to Telegram Forum Topics
**Priority:** Medium-High
**Epic:** Telegram Bridge
**Status:** Draft
**Author:** Product Manager
**Date:** 2026-04-11

---

## 1. Problem Statement

### The Problem

OpenConductor's Telegram bridge is asymmetric when it comes to images. Users can send photos from Telegram to agents (downloaded to `.openconductor/images/`, path forwarded to the agent's PTY via `HandleInboundMedia`), but agents cannot send images back. All outbound Telegram communication uses `sendMessage` with HTML-formatted text and `<pre>` code blocks for terminal content.

This creates three concrete gaps:

1. **Visual output is invisible remotely.** Agents using Playwright MCP, chart-generation libraries, or image-creation tools produce files that the user can only see by switching to the TUI and navigating to the agent's terminal. A user monitoring agents from their phone via Telegram has no way to see a screenshot the agent just captured, a diagram it generated, or a chart it rendered. They see the agent's text description of the image, but not the image itself.

2. **Terminal context is lossy as text.** When OpenConductor sends screen content to Telegram, it converts the VT100 terminal state to plain text inside `<pre>` blocks. This strips all colors, styles, box-drawing layout, and visual structure that make terminal output scannable. A user reading a permission request on Telegram cannot see the syntax highlighting, diff coloring, or progress indicators that would help them make a quick decision. An actual screenshot of the terminal would preserve this visual context.

3. **No manual share action.** Even when a user is actively using the TUI and wants to share the current terminal view with a collaborator via Telegram (or save it to the project's Forum Topic as a record), there is no mechanism to do so. The only outbound path is automatic state-transition events, which send text-only.

### Why Now

- The inbound image pipeline already exists end-to-end: `HandleInboundMedia` downloads files, saves to disk, formats paths for agents. The outbound direction is the natural complement.
- The raw API infrastructure in `bot.go` (`rawAPICall`) already supports arbitrary Telegram Bot API methods -- adding `sendPhoto` and `sendDocument` requires no architectural changes.
- Agent tool ecosystems are expanding. Claude Code with Playwright MCP, code-generation tools that produce SVGs and PNGs, and diagram-as-code tools all produce image artifacts that users need to see remotely.
- The `vt10x.Terminal` exposes `Cell(col, row)` with full glyph data (character, foreground, background, attributes). This is sufficient to render pixel-accurate terminal screenshots to PNG without any third-party rendering dependency.

### What Success Looks Like

An agent captures a browser screenshot via Playwright. OpenConductor detects the new image file in the agent's output, and within seconds the screenshot appears in the project's Telegram Forum Topic with a caption like "Browser screenshot: login page after form submission." The user sees the actual image on their phone, taps to zoom in, and replies with feedback -- all without touching the TUI.

Separately, a user working in the TUI presses a keybinding, and a pixel-rendered screenshot of the current terminal view is sent to the project's Telegram topic, preserving colors, layout, and visual structure.

---

## 2. User Personas Affected

### Primary: The Remote Monitor

- Manages 3-8 agents from their phone via Telegram while away from the workstation
- Currently receives text-only notifications that lack visual context for decisions
- Needs to see screenshots, diagrams, and rendered output that agents produce
- Frustrated by having to return to the TUI just to view an image the agent mentioned

### Secondary: The Visual Builder

- Works on frontend, UI, or data-visualization projects where agent output is inherently visual
- Uses Playwright MCP for browser automation and screenshot capture
- Generates charts, diagrams, or design mockups through agent commands
- Wants visual artifacts to appear in the project's Telegram topic as a running log

### Tertiary: The Team Collaborator

- Uses Telegram Forum Topics as a shared record of agent work for the team
- Wants to manually send terminal screenshots to document progress, errors, or interesting states
- Currently copies and pastes text, losing all visual formatting

---

## 3. User Flows

### Flow 1: Agent Creates an Image -- Auto-Detected and Sent to Telegram

**Trigger:** An agent creates or downloads an image file as part of its work (e.g., Playwright screenshot, generated chart, saved diagram).

**Detection strategy:** Parse agent PTY output for file paths referencing image files. When the agent writes to its PTY, the output often contains lines like:

```
Screenshot saved to /Users/dev/project/screenshots/login.png
Created diagram: ./docs/architecture.svg
Image saved: output/chart.png
```

The detection layer scans recent terminal output (the lines sent to Telegram in the next `EventResponse` or attention event) for patterns matching image file paths with known extensions.

**Preconditions:**
- Telegram bridge is enabled and healthy (`Bot.IsHealthy()`)
- The project has an active Forum Topic (`topicState.Get(project) != 0`)
- The agent session is running

**Happy path:**

1. Agent runs a tool that produces an image file (e.g., `screenshot.png`)
2. Agent writes output to PTY mentioning the file path
3. On the next attention tick or state transition, `sendTelegramEvent` is called with the screen lines
4. The new image detection layer scans the screen lines (and/or recent scrollback) for file path patterns matching supported image extensions: `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, `.svg`
5. For each detected path:
   a. Resolve the path relative to the project's repo directory
   b. Verify the file exists and is readable
   c. Check file size (under 10MB for photo, under 50MB for document)
   d. Determine send mode: raster images (PNG, JPG, JPEG, GIF, WEBP) under 10MB are sent as photos (`sendPhoto`); SVG files and anything over 10MB are sent as documents (`sendDocument`)
   e. Extract a caption from the surrounding terminal output (the line containing the path, or a configurable number of context lines)
   f. Send to the project's Forum Topic with `message_thread_id`
6. The text message for the event is sent as usual (no change to existing behavior)
7. The sent image path is recorded to prevent duplicate sends on subsequent ticks

**Alternative paths:**

- **File not found:** Agent output references a path that does not exist (yet). The image is queued for a single retry on the next tick. If still missing, it is dropped and logged.
- **File too large:** Files exceeding 50MB are skipped with a warning logged. The text event includes a note: "(Image too large for Telegram: 52.3MB)"
- **Multiple images in one event:** All detected images are sent sequentially, each with its own caption. Rate limiting applies (3s minimum between sends to the same topic, matching existing `minSendInterval`).
- **SVG files:** Sent as documents (Telegram cannot render SVG as photos). The caption notes the format.
- **Agent writes image to a path outside the repo:** Only paths within the project's repo directory (or absolute paths the process has access to) are followed. Paths outside the repo are logged but not sent, to avoid leaking unrelated files.

### Flow 2: User Manually Triggers "Send Terminal Screenshot" from TUI

**Trigger:** User presses a keybinding while viewing a terminal tab.

**Keybinding:** `Ctrl+Shift+T` (mnemonic: Telegram). Falls through to the PTY if no Telegram bridge is configured.

**Preconditions:**
- Telegram bridge is enabled and the bot is healthy
- Focus is on the terminal (not the sidebar or a form)
- An active session exists for the current tab

**Happy path:**

1. User presses `Ctrl+Shift+T`
2. The TUI captures the current terminal state:
   a. Read all cells from `vt10x.Terminal` via `Cell(col, row)` for the full grid
   b. Render to a PNG image using the terminal's color scheme (foreground, background, attributes per cell)
   c. Include scrollback content if the user is currently scrolled up (render the visible viewport)
3. The PNG is saved to a temporary file (e.g., `/tmp/openconductor-screenshot-<sessionID>-<timestamp>.png`)
4. The screenshot is sent to the project's Forum Topic via `sendPhoto` with caption: `"Terminal screenshot -- <project name> (<timestamp>)"`
5. A brief status bar message confirms: "Screenshot sent to Telegram"
6. The temporary file is cleaned up after successful send

**Alternative paths:**

- **No Telegram configured:** The keybinding is a no-op. The status bar shows: "Telegram not configured"
- **Bot unhealthy:** Status bar shows: "Telegram bot is not connected"
- **Send fails:** Status bar shows: "Screenshot send failed: <error>". The temporary file is retained for manual retry.
- **User is in scrollback mode:** The screenshot captures the scrollback viewport (what the user is currently seeing), not the live terminal bottom.

### Flow 3: Image Path in Agent Output Sent Alongside Attention Notification

**Trigger:** An attention event (permission request, question, error, done) fires, and the screen content or recent output contains a reference to an image file.

**Preconditions:**
- Same as Flow 1
- An attention state transition has occurred (`sendTelegramEvent` is called)

**Happy path:**

1. Attention detection fires (e.g., `StateNeedsPermission`)
2. `sendTelegramEvent` is called with the current screen lines
3. Before sending the text message, the image detection layer scans the screen lines
4. If image paths are found:
   a. The image(s) are sent first, each as a separate `sendPhoto`/`sendDocument` call
   b. The text message (with inline keyboard for permissions/questions) is sent after the images
   c. This ordering ensures the user sees the visual context before the action prompt
5. If no images are found, behavior is identical to today

**Alternative paths:**

- **Image from a previous event:** The deduplication set (paths already sent for this session) prevents re-sending images that appeared in earlier events. The set is keyed by absolute file path + modification time.
- **Permission request for an image-related tool:** When the permission detail mentions an image operation (e.g., "Read file: ./screenshots/result.png"), the referenced image is proactively attached to the permission message.

---

## 4. Technical Design Notes

### 4.1 Image Detection Strategy

Two complementary strategies, both lightweight:

**Strategy A: Output path scanning (primary)**

Scan terminal screen lines for file paths with image extensions. Use a regex pattern:

```
(?:^|[\s"'=(])(/[^\s"']+\.(?:png|jpg|jpeg|gif|webp|svg))|(?:^|[\s"'=(])(\.{0,2}/[^\s"']+\.(?:png|jpg|jpeg|gif|webp|svg))
```

This catches absolute paths (`/Users/dev/project/screenshot.png`) and relative paths (`./output/chart.png`, `../images/diagram.svg`). Relative paths are resolved against the project's repo directory.

**Strategy B: Filesystem watcher (future enhancement, not in MVP)**

Use `fsnotify` to watch common output directories (configurable per project). More reliable for agents that write files without printing the path, but adds complexity and is deferred to a follow-up iteration.

### 4.2 Telegram API Calls

**`sendPhoto`** (for raster images under 10MB):
```json
{
  "chat_id": <chat_id>,
  "message_thread_id": <topic_id>,
  "photo": "<multipart file upload>",
  "caption": "<caption text>",
  "parse_mode": "HTML"
}
```

**`sendDocument`** (for SVG, large files, or when original quality is needed):
```json
{
  "chat_id": <chat_id>,
  "message_thread_id": <topic_id>,
  "document": "<multipart file upload>",
  "caption": "<caption text>",
  "parse_mode": "HTML"
}
```

Both require `multipart/form-data` encoding (not the JSON `rawAPICall` used today). A new `rawMultipartCall` method on `Bot` will handle file uploads.

### 4.3 Terminal Screenshot Rendering

Render the `vt10x.Terminal` grid to a PNG image:

- Read each cell via `Cell(col, row)` to get `vt10x.Glyph` (char, fg, bg, attributes)
- Use a monospace font (embedded, e.g., JetBrains Mono or similar) rendered via Go's `image` and `golang.org/x/image/font` packages
- Map the 16 ANSI colors + 256 extended colors to RGB values matching the TUI's color scheme (from `styles.go`)
- Support bold (brighter color or bold font weight), italic, underline, reverse video
- Default cell size: 8x16 pixels (standard monospace at ~14pt), configurable
- Output resolution: `width * 8` x `height * 16` pixels (e.g., 120x40 terminal = 960x640 PNG)
- Render only the visible viewport (or scrollback viewport if scrolled up)

This is a self-contained rendering pipeline with no external process dependencies.

### 4.4 New Files and Changes

| File | Change |
|------|--------|
| `internal/telegram/bot.go` | Add `sendPhotoToTopic`, `sendDocumentToTopic`, `rawMultipartCall` methods |
| `internal/telegram/bridge.go` | Add `ImagePath` field to `Event` struct; add image dedup set |
| `internal/telegram/image.go` | New file: image detection (path scanning), file validation, send-mode selection |
| `internal/telegram/screenshot.go` | New file: VT100-to-PNG renderer using `vt10x.Glyph` data |
| `internal/tui/app.go` | Handle `Ctrl+Shift+T` keybinding; call screenshot+send flow |
| `internal/tui/messages.go` | Add `ScreenshotSentMsg`, `ScreenshotFailedMsg` |
| `internal/tui/statusbar.go` | Add "Ctrl+Shift+T: screenshot to Telegram" hint when Telegram is active |

### 4.5 Integration with Existing Notification Flow

The image sending integrates at the `sendTelegramEvent` layer in `app.go` (line ~2463). The flow becomes:

1. `sendTelegramEvent` is called (existing trigger, no change)
2. Screen lines are filtered through `ScreenFilter` and `ChromeLineFilter` (existing, no change)
3. **New:** Image detection scans the filtered lines for file paths
4. **New:** Detected images are queued on the `Event` struct (new `Images []ImageRef` field)
5. Event is sent to the bridge channel (existing)
6. `bridgeLoop` processes the event:
   a. **New:** For each image in `Event.Images`, call `sendPhotoToTopic` or `sendDocumentToTopic`
   b. Send the text message (existing)
   c. Attach inline keyboard to the last text message (existing, no change)

### 4.6 Rate Limiting and Ordering

- Images are sent before the text message so users see visual context first
- Each image send counts against the per-project rate limit (existing 3s `minSendInterval`)
- When multiple images are detected in a single event, they are batched: Telegram's `sendMediaGroup` API can send up to 10 images in one call, displayed as an album. This is preferred over sequential `sendPhoto` calls for multi-image events.
- If rate limiting causes image sends to be delayed, the text message waits for all images to complete (preserving ordering)

---

## 5. Edge Cases and Error Scenarios

### File System

| Scenario | Handling |
|----------|----------|
| Image file deleted before send | Log warning, skip image, send text event normally |
| Image file is being written (partial) | Detect via file size stability: wait 500ms, re-check size. If unchanged, proceed. If still changing, retry on next tick |
| Symlinked image file | Follow symlinks (`os.Stat`, not `os.Lstat`) but only within the repo directory |
| Path with spaces or special characters | Regex handles quoted paths (`"path with spaces/img.png"`) and unquoted paths up to whitespace |
| Path references home dir (`~/screenshots/img.png`) | Expand `~` to `os.UserHomeDir()` before resolution |
| Binary file with image extension | Validate file magic bytes (PNG: `\x89PNG`, JPEG: `\xFF\xD8\xFF`, GIF: `GIF8`) before sending |

### Telegram API

| Scenario | Handling |
|----------|----------|
| `sendPhoto` fails (network error) | Retry once after 2s. If still failing, log error and continue with text-only event |
| `sendPhoto` returns 413 (file too large) | Fall back to `sendDocument`. If that also fails, log and skip |
| Bot token revoked mid-send | Existing `IsHealthy()` check catches this; sends are skipped when unhealthy |
| Topic deleted | Existing topic lookup returns 0; event is dropped with log |
| Rate limit hit (HTTP 429) | Parse `retry_after` from response, sleep, then retry. Existing `backoff` helper can be reused |

### Terminal Screenshots

| Scenario | Handling |
|----------|----------|
| Terminal not yet initialized (no VT) | Status bar: "No terminal content to screenshot" |
| Very large terminal (e.g., 300x80) | Cap screenshot at 4096x4096 pixels. Resize proportionally if exceeded |
| Non-Latin characters / emoji in terminal | Render via Unicode-aware font. Fall back to replacement character for unsupported glyphs |
| Screenshot during rapid output | Lock `terminalModel.mu` for the duration of the cell read (already done via `RLock` in `GetScreenLines`) |
| Temp file write fails | Return error to status bar; do not attempt send |

### Deduplication

| Scenario | Handling |
|----------|----------|
| Same image path appears in consecutive events | Dedup by `(absolute_path, mtime)` tuple. Same path with new mtime is re-sent (agent regenerated the file) |
| Agent output repeats the same path across ticks | Dedup set holds entries for 5 minutes, then expires. Prevents memory leak for long sessions |
| Agent creates many images rapidly | Cap at 5 images per event. Additional images are queued for the next event cycle |

---

## 6. Success KPIs

### Primary Metrics

| KPI | Target | Measurement |
|-----|--------|-------------|
| Image delivery rate | >95% of detected images successfully sent to Telegram | `images_sent / images_detected` logged per session |
| Detection accuracy | >90% of agent-created images detected from output | Manual audit of 50 sessions comparing agent output paths to sent images |
| End-to-end latency | <5s from image file creation to Telegram delivery | Timestamp diff between file `mtime` and Telegram API response |
| Screenshot render time | <500ms for a 120x40 terminal | Instrumented timing in screenshot renderer |

### Secondary Metrics

| KPI | Target | Measurement |
|-----|--------|-------------|
| Manual screenshot usage | >20% of active Telegram users use `Ctrl+Shift+T` within first week | Keybinding event counter in structured log |
| False positive rate | <5% of detected "images" are not actual image paths | Log review of detection misses and false positives |
| User engagement lift | >15% increase in Telegram thread replies after image sends | Compare reply rate on image-containing events vs text-only events |
| Error rate | <1% of send attempts result in unrecoverable errors | Error counter in structured log |

### Guardrail Metrics

| KPI | Threshold | Action |
|-----|-----------|--------|
| Telegram API errors | >10% of sends fail | Alert; investigate API changes or rate limit issues |
| Memory usage increase | >50MB per session from image buffering | Optimize dedup set expiry and image caching |
| Screenshot PNG file size | >2MB average | Reduce default cell size or add compression level config |

---

## 7. Acceptance Criteria

### AC-1: Automatic Image Detection and Sending

- [ ] When an agent writes a line containing a path to a `.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`, or `.svg` file that exists on disk, the image is sent to the project's Telegram Forum Topic
- [ ] PNG, JPG, JPEG, GIF, and WEBP files under 10MB are sent via `sendPhoto` (compressed, with preview)
- [ ] SVG files and any file over 10MB (up to 50MB) are sent via `sendDocument` (original quality)
- [ ] Files over 50MB are skipped with a structured log warning
- [ ] The image caption includes the file name and the surrounding output line
- [ ] The same image file (same path + modification time) is not sent twice within the same session
- [ ] Image detection does not trigger on paths in agent input (only output/screen content)

### AC-2: Terminal Screenshot via Keybinding

- [ ] Pressing `Ctrl+Shift+T` when the terminal is focused and Telegram is configured captures the current terminal viewport as a PNG
- [ ] The PNG preserves terminal colors (16 ANSI + 256 extended), bold, and reverse video attributes
- [ ] The screenshot is sent to the active project's Forum Topic with caption including project name and timestamp
- [ ] A status bar message confirms successful send or displays the error
- [ ] The keybinding is a no-op when Telegram is not configured (no error, no status message)
- [ ] The keybinding hint appears in the status bar when Telegram is active
- [ ] Screenshot capture completes in under 500ms for a 120x40 terminal

### AC-3: Integration with Attention Events

- [ ] When an attention event (permission, question, error, done) includes screen lines referencing an image, the image is sent before the text message
- [ ] The inline keyboard (permission buttons, question options) is attached to the text message, not the image
- [ ] Image sends respect the existing 3-second per-project rate limit
- [ ] When multiple images are detected in a single event, they are sent as a media group (album)

### AC-4: Error Handling and Resilience

- [ ] A missing image file (deleted between detection and send) is logged and skipped without affecting the text event
- [ ] A Telegram API error on image send is retried once, then logged and skipped
- [ ] File magic bytes are validated before sending (prevent sending non-image files with image extensions)
- [ ] The image detection regex does not cause false positives on common terminal output patterns (e.g., file paths in `ls -la` output that happen to end in `.png`)

### AC-5: Tests

- [ ] Unit tests for image path detection regex (positive and negative cases)
- [ ] Unit tests for send-mode selection (photo vs document based on format and size)
- [ ] Unit tests for deduplication logic (same path, same path + new mtime, expiry)
- [ ] Unit tests for caption extraction from surrounding output lines
- [ ] Integration test for terminal-to-PNG rendering (verify output dimensions, non-empty pixels)
- [ ] Unit test for `rawMultipartCall` with mock HTTP server

---

## 8. Out of Scope (Future Iterations)

- **Filesystem watcher (`fsnotify`)**: More reliable detection for agents that create images without printing paths. Adds complexity; deferred to follow-up.
- **Headless mode image sending**: Sending images without a running TUI. Requires headless session management (separate epic).
- **Image editing/annotation in Telegram**: Telegram's built-in photo editor could annotate images before sending back to the agent. Requires inbound image pipeline changes.
- **Video/GIF recording**: Recording terminal sessions as animated GIFs or short videos. Significant rendering complexity.
- **Configurable image quality/resolution**: User-configurable screenshot resolution, font, color scheme. Start with sensible defaults.
- **Thumbnail generation**: Creating smaller preview images for very large screenshots. Telegram handles resizing.
- **Per-project image sending toggle**: Config to enable/disable image detection per project. Start with global on/off.

---

## 9. Implementation Order

### Phase 1: Foundation (MVP)

1. Add `rawMultipartCall` to `bot.go` for file uploads
2. Add `sendPhotoToTopic` and `sendDocumentToTopic` methods
3. Implement image path detection in new `image.go`
4. Add `Images []ImageRef` to `Event` struct
5. Wire detection into `sendTelegramEvent` in `app.go`
6. Wire sending into `bridgeLoop` in `bot.go`
7. Add deduplication logic
8. Tests for all new code

### Phase 2: Terminal Screenshots

1. Build VT100-to-PNG renderer in `screenshot.go`
2. Add `Ctrl+Shift+T` keybinding in `app.go`
3. Add status bar hint and confirmation messages
4. Tests for renderer

### Phase 3: Polish

1. Add `sendMediaGroup` support for multi-image events
2. File magic byte validation
3. Retry logic for failed sends
4. Structured logging for all image operations (detection, send, skip, error)
5. Documentation update

---

## 10. Resolved Design Decisions

1. **Font embedding:** Embed a monospace font in the binary for terminal screenshot rendering.
2. **Detection strategy:** Match any file path with an image extension — no keyword prefix required.
3. **No rate limiting on images.** Images are sent immediately. Telegram's bot API allows ~30 msg/s; the 3-second text interval is unrelated.
4. **Sequential sends.** Use individual `sendPhoto` calls, not `sendMediaGroup`. Keeps message ordering clear.
5. **Screenshot colors are the agent's concern.** OpenConductor forwards image files as-is. When an agent (Claude Code via Playwright, etc.) produces a screenshot, it controls the rendering. OpenConductor does not render VT100 state to images — it detects and forwards files the agent creates.

**Scope simplification:** Flow 2 (manual `Ctrl+Shift+T` terminal-to-PNG rendering) is deferred. The MVP is: detect image files from agent output and forward them to Telegram. The agent already produces screenshots via its tools.
