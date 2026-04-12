# Implementation Plan: Send Photos from Agents to Telegram

**Feature**: Detect image files in agent output and forward them to the project's Telegram Forum Topic.

**Date**: 2026-04-11
**Status**: Ready for implementation

---

## Document Index

| Document | Purpose |
|----------|---------|
| [User Story](../user-stories/telegram-photo-sending.md) | Problem statement, user flows, acceptance criteria |
| [High-Level Design](telegram-photos-hld.md) | Architecture, data flow, component boundaries |
| [Low-Level Design](lld-telegram-photos.md) | Types, functions, algorithms, regex, test specs |
| [Architect Review](telegram-photos-review.md) | Review with 2 must-fix issues |
| [Test Plan](test-plan-telegram-photos.md) | 81 test cases, coverage matrix |

---

## Feature Summary

When an agent writes a file path with an image extension to its terminal output, OpenConductor detects it, validates the file, and sends it to the project's Telegram Forum Topic via `sendPhoto` or `sendDocument`. Images are sent before text so users see visual context before action prompts.

**Supported formats**: PNG, JPG, JPEG, GIF, WEBP (as photo), SVG (as document)
**Size limits**: ≤10MB raster → sendPhoto, 10-50MB → sendDocument, >50MB → skip
**Detection**: Two-layer — L1 regex on screen lines + L2 LLM extraction from classifier
**Dedup**: Keyed on `project|absPath|mtime`, 5-minute TTL

---

## Resolved Design Decisions

1. **Detection in app.go** (sendTelegramEvent), not telegram package — needs repo path for resolution
2. **No rate limiting on images** — send immediately, Telegram allows ~30 msg/s
3. **Sequential sendPhoto calls** — not sendMediaGroup
4. **Embed monospace font** — for potential future terminal screenshot rendering
5. **No VT100 renderer in MVP** — agents produce screenshots, OC just forwards files
6. **Images before text** — visual context before action buttons
7. **Per-image error isolation** — image failure never blocks text delivery
8. **413 fallback** — sendPhoto failure falls back to sendDocument (from architect review)
9. **Shell redirect filtering** — post-match check for `>` before path (from architect review)
10. **Two-layer detection (L1 regex + L2 LLM)** — regex catches explicit paths; LLM classifier extracts paths from natural language agent output when attention detection already fires

---

## Architecture

```
Agent PTY output
    ↓
sendTelegramEvent (app.go)
    ↓ filter screen lines (existing)
    ↓ L1: DetectImagePaths (regex) — regex → resolve → validate → dedup
    ↓ attach ImageRef[] to Event
    ↓
    ↓ In parallel (existing attention path):
    ↓ L2: Classifier.Classify (when triggered)
    ↓   → extended prompt asks LLM to also extract image file paths
    ↓   → ClassifyResult gains ImagePaths []string field
    ↓   → resolved + validated + deduped same as L1
    ↓   → merged into Event.Images (union with L1 results)
    ↓
bridge channel (existing)
    ↓
bridgeLoop → sendEvent (bot.go)
    ↓ for each image: sendPhotoToTopic or sendDocumentToTopic
    ↓ then: sendToTopic text message (existing)
    ↓
Telegram Bot API
    sendPhoto (multipart/form-data)
    sendDocument (multipart/form-data)
```

### Two-Layer Detection

**L1 — Regex (always runs):** Fast pattern matching on screen lines. Catches explicit paths like `/Users/dev/screenshots/login.png`. Zero latency, no API cost.

**L2 — LLM (piggybacks on attention classifier):** The existing `Classifier.Classify()` already sends screen lines to an LLM. Extend the prompt to also ask: "If the agent created or references any image files, list their paths." The LLM can catch natural language references that regex misses, e.g.:
- "I've saved the architecture diagram" (no path in output, but agent created a file)
- "Here's the screenshot" (path only visible in scrollback, not current screen)
- "The chart has been generated at the project root" (implicit path)

The LLM extraction runs only when the classifier fires (already throttled to 1 call per 5s with backoff). No additional API calls — it's a prompt extension on an existing call.

---

## Implementation Order

### Step 1: Image types and detection (`internal/telegram/image.go` — new file)

- `SendMode` enum (SendAsPhoto, SendAsDocument)
- `ImageRef` struct (AbsPath, Caption, SendMode)
- `ImageDedup` struct with thread-safe map, `ShouldSend()`, lazy TTL cleanup
- `imagePathRe` compiled regex with post-match `>` redirect filtering
- `resolvePath()` — absolute, ~/relative, ./relative
- `validateImage()` — os.Stat → size check → magic bytes → send mode
- `matchesMagic()` — PNG, JPEG, GIF, WebP header checks (SVG exempt)
- `extractCaption()` — context from surrounding line, HTML-escaped, 1024 char limit
- `DetectImagePaths()` — entry point: regex → resolve → validate → dedup → []ImageRef

### Step 2: Multipart upload (`internal/telegram/bot.go` — modifications)

- `rawMultipartCall(method, fields, fileField, filePath)` — multipart/form-data POST
- `sendPhotoToTopic(topicID, filePath, caption)` — wraps rawMultipartCall for sendPhoto
- `sendDocumentToTopic(topicID, filePath, caption)` — wraps rawMultipartCall for sendDocument

### Step 3: Event wiring (`internal/telegram/bridge.go` + `bot.go`)

- Add `Images []ImageRef` field to `Event` struct
- Modify `sendEvent()` to iterate images before text, with 413 photo→document fallback
- Per-image error logging, never blocks text delivery

### Step 4: LLM image extraction (`internal/attention/classifier.go`)

- Extend `buildPrompt()` to include: "If the agent created or references any image files (.png, .jpg, .svg, etc.), list their file paths on separate lines after 'Images:'. If none, write 'Images: none'."
- Add `ImagePaths []string` field to a new `ClassifyResult` struct (replace the bare string return)
- Add `parseImagePaths(response string) []string` — extract paths from LLM response after "Images:" marker
- Update `Classify()` return type from `string` to `ClassifyResult`
- Update all callers of `Classify()` (detector.go, tests)

### Step 5: Detection integration (`internal/tui/app.go`)

- Add `imageDedup *telegram.ImageDedup` field to App (lazy init)
- In `sendTelegramEvent`: call `DetectImagePaths(filteredLines, project.Repo)` (L1 regex)
- When attention classifier returns `ClassifyResult.ImagePaths`, resolve + validate + dedup those too (L2 LLM)
- Merge L1 + L2 results (deduplicated by absolute path) into `Event.Images`

### Step 6: Tests (`internal/telegram/image_test.go` + `bot_test.go` + `classifier_test.go`)

- 81 test cases total (from test plan)
- Regex: 25 positive/negative cases
- Validation: 10 cases with crafted magic bytes
- Dedup: 7 state machine cases
- Multipart: httptest server
- sendEvent ordering: images before text
- Edge cases: symlinks, TOCTOU, concurrent access, no-topic

---

## Files Changed

| File | Change | Lines |
|------|--------|-------|
| `internal/telegram/image.go` | **New** — detection, validation, dedup, types | ~250 |
| `internal/telegram/image_test.go` | **New** — 60+ test cases | ~500 |
| `internal/telegram/bot.go` | Add multipart upload + photo/document send + sendEvent image loop | ~90 |
| `internal/telegram/bot_test.go` | Add multipart + sendEvent tests | ~100 |
| `internal/telegram/bridge.go` | Add `Images` field to Event | ~1 |
| `internal/attention/classifier.go` | Extend prompt for image paths, ClassifyResult struct | ~40 |
| `internal/attention/detector.go` | Update Classify caller for new return type | ~5 |
| `internal/tui/app.go` | Add imageDedup, call DetectImagePaths + merge LLM paths | ~20 |

---

## Test Coverage

| Area | Count |
|------|-------|
| Regex detection | 25 |
| Path resolution | 6 |
| File validation | 10 |
| Dedup logic | 7 |
| Caption extraction | 3 |
| DetectImagePaths integration | 7 |
| Multipart upload | 3 |
| sendEvent ordering | 2 |
| Edge cases (from QA review) | 18 |
| **Total** | **81** |

---

## Must-Fix Items from Architect Review

1. **Shell redirect false positive**: Add post-match check — if the character before the path (after whitespace stripping) is `>`, skip the match. Prevents `echo data > ./output/file.png` from triggering detection.

2. **413 photo→document fallback**: In `sendEvent`'s image loop, if `sendPhotoToTopic` returns an error containing "413" or "Request Entity Too Large", retry with `sendDocumentToTopic`. Handles images near the 10MB boundary that Telegram rejects as photos.
