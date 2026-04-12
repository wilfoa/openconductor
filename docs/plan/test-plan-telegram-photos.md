# Test Plan: Telegram Photo Sending

**Feature**: Telegram Photo Sending (US-TGIMG-001)
**Status**: Draft
**Date**: 2026-04-11
**Depends on**:
- User Story: `docs/user-stories/telegram-photo-sending.md`
- HLD: `docs/plan/telegram-photos-hld.md`
- LLD: `docs/plan/lld-telegram-photos.md`

**Test Strategy**: Unit tests per package using Go's `testing` package, table-driven where applicable, following existing codebase patterns (`bot_test.go`, `bridge_test.go`, `handler_test.go`). All filesystem tests use `t.TempDir()` for isolation. HTTP tests use `httptest.NewServer`. No new external test dependencies.

---

## 1. Acceptance Criteria to Test Case Coverage Matrix

This matrix maps every acceptance criterion from the user story (AC-1 through AC-5) to specific test cases in this plan. A criterion is considered covered when at least one test case exercises the behavior it describes. The "LLD Covered" column indicates whether the LLD already specifies a test for this sub-criterion; the "Gap" column flags criteria that required new test cases beyond what the LLD provides.

### AC-1: Automatic Image Detection and Sending

| AC-1 Sub-Criterion | Test Case IDs | LLD Covered | Gap |
|---|---|---|---|
| Detects paths to .png, .jpg, .jpeg, .gif, .webp, .svg files that exist on disk | R01-R15, I01, I02, I07 | Yes | No |
| PNG/JPG/JPEG/GIF/WEBP under 10MB sent via sendPhoto | V01-V05 | Yes | No |
| SVG files sent via sendDocument | V06, E14 | Yes | No |
| Files over 10MB up to 50MB sent via sendDocument | V10 | Yes | No |
| Files over 50MB skipped with log warning | V11 | Yes | No |
| Caption includes file name and surrounding output line | C01-C03 | Yes | No |
| Same image (path+mtime) not sent twice within session | D01-D05, D06, D07 | Yes | No |
| Detection only triggers on output, not agent input | E01 | **No** | **Yes -- added** |

### AC-2: Terminal Screenshot via Keybinding (Deferred in MVP)

Per the user story section 10 ("Resolved Design Decisions"), Flow 2 (manual Ctrl+Shift+T terminal-to-PNG rendering) is deferred from the MVP scope. The HLD confirms this in resolved decision #6. AC-2 acceptance criteria do not apply to the current implementation scope. No test cases are needed.

If AC-2 is re-scoped into a future iteration, the following test areas will be required: PNG renderer output dimensions and pixel content, keybinding registration and dispatch, status bar confirmation messages, no-op behavior when Telegram is unconfigured, and render time performance benchmarks.

### AC-3: Integration with Attention Events

| AC-3 Sub-Criterion | Test Case IDs | LLD Covered | Gap |
|---|---|---|---|
| Images sent before text message on attention events | S01 | Yes | No |
| Inline keyboard attached to text message, not image | S01, E02 | Partial (S01 checks order) | **Yes -- E02 added** |
| Image sends respect existing rate limit | E03 | **No** | **Yes -- added** |
| Multiple images in single event sent (sequential in MVP) | E04 | **No** | **Yes -- added** |

### AC-4: Error Handling and Resilience

| AC-4 Sub-Criterion | Test Case IDs | LLD Covered | Gap |
|---|---|---|---|
| Missing image file logged and skipped without affecting text | S02, I03 | Yes | No |
| Telegram API error on image send retried once then skipped | E05 | **No (MVP has no retry)** | **Yes -- added as negative verification** |
| File magic bytes validated before sending | V07, I04 | Yes | No |
| Regex does not false-positive on common terminal output | R17-R27, E06 | Partial | **Yes -- E06 added** |

### AC-5: Tests

| AC-5 Sub-Criterion | Test Case IDs | LLD Covered | Gap |
|---|---|---|---|
| Unit tests for image path detection regex | R01-R27 | Yes | Extended with additional cases |
| Unit tests for send-mode selection | V01-V11 | Yes | No |
| Unit tests for dedup logic | D01-D07 | Yes | No |
| Unit tests for caption extraction | C01-C03, E11-E13 | Yes | Extended with edge cases |
| Integration test for terminal-to-PNG rendering | N/A (deferred) | N/A | N/A |
| Unit test for rawMultipartCall with mock HTTP server | M01-M03 | Yes | No |

---

## 2. Unit Tests: `internal/telegram/image_test.go` (new file)

### 2.1 Path Detection Regex

**Test Function**: `TestImagePathRegex`

| # | Name | Input Line | Expected Matches |
|---|---|---|---|
| R01 | `absolute path` | `Screenshot saved to /tmp/screenshot.png` | `["/tmp/screenshot.png"]` |
| R02 | `relative path with dot-slash` | `Created ./docs/arch.svg` | `["./docs/arch.svg"]` |
| R03 | `relative path with double-dot` | `file: "../output/chart.jpeg"` | `["../output/chart.jpeg"]` |
| R04 | `home-relative path` | `Saved to ~/screenshots/login.PNG` | `["~/screenshots/login.PNG"]` |
| R05 | `multiple paths in one line` | `Compare /a/b.png and /c/d.jpg` | `["/a/b.png", "/c/d.jpg"]` |
| R06 | `path in single quotes` | `Image at '/tmp/test/output.gif'` | `["/tmp/test/output.gif"]` |
| R07 | `path in double quotes` | `Wrote "/home/user/project/result.webp"` | `["/home/user/project/result.webp"]` |
| R08 | `path after equals sign` | `output=/var/data/chart.png` | `["/var/data/chart.png"]` |
| R09 | `path at start of line` | `/tmp/screenshot.png saved successfully` | `["/tmp/screenshot.png"]` |
| R10 | `jpeg extension` | `Created ./output/photo.jpg` | `["./output/photo.jpg"]` |
| R11 | `case-insensitive extension JPG` | `Image: /tmp/photo.JPG done` | `["/tmp/photo.JPG"]` |
| R12 | `case-insensitive extension Png` | `Saved /data/capture.Png to disk` | `["/data/capture.Png"]` |
| R13 | `path in parentheses` | `image (./output/diagram.svg) created` | `["./output/diagram.svg"]` |
| R14 | `deeply nested path` | `Created /home/user/projects/myapp/src/assets/images/logo.png` | `["/home/user/projects/myapp/src/assets/images/logo.png"]` |
| R15 | `bare relative path with subdirectory` | `Output: screenshots/login.png` | `["screenshots/login.png"]` |
| R16 | `path with spaces in parent dir (quoted)` | `Saved to "/Users/dev/My Project/output/screen.png"` | `["/Users/dev/My"]` (known limitation: regex stops at whitespace) |
| R17 | `glob pattern with asterisk` | `rm *.png` | `nil` |
| R18 | `URL https` | `https://example.com/images/photo.png` | `nil` |
| R19 | `URL http` | `http://cdn.example.com/assets/logo.jpg` | `nil` |
| R20 | `non-image extension` | `Created /tmp/output.txt` | `nil` |
| R21 | `bare filename without directory` | `image.png` | `nil` |
| R22 | `glob with question mark` | `ls file?.png` | `nil` |
| R23 | `pipe redirect` | `convert input.bmp \| output.png` | `nil` |
| R24 | `no path at all` | `No image paths here` | `nil` |
| R25 | `extension in the middle of a word` | `The loading.png module handles caching` | `nil` (no directory separator) |
| R26 | `color code not a path` | `color: #png; font: sans` | `nil` |
| R27 | `directory path ending with slash` | `cd /tmp/images/` | `nil` (no image extension) |

### 2.2 Path Resolution

**Test Function**: `TestResolvePath`

| # | Name | Detected Path | Repo Dir | Expected Absolute Path |
|---|---|---|---|---|
| P01 | `absolute path unchanged` | `/tmp/screenshot.png` | `/home/user/project` | `/tmp/screenshot.png` |
| P02 | `relative dot-slash` | `./output/chart.png` | `/home/user/project` | `/home/user/project/output/chart.png` |
| P03 | `relative double-dot` | `../sibling/img.png` | `/home/user/project` | `/home/user/sibling/img.png` |
| P04 | `bare relative` | `screenshots/login.png` | `/home/user/project` | `/home/user/project/screenshots/login.png` |
| P05 | `home-relative path` | `~/screenshots/img.png` | `/home/user/project` | `<os.UserHomeDir()>/screenshots/img.png` (prefix assert) |
| P06 | `path with redundant slashes cleaned` | `./output//chart.png` | `/home/user/project` | `/home/user/project/output/chart.png` |

### 2.3 File Validation

**Test Function**: `TestValidateImage`

| # | Name | Filename | Content | Expected Mode | Expected Error |
|---|---|---|---|---|---|
| V01 | `valid PNG under 10MB` | `test.png` | PNG magic header + 100 bytes padding | `SendAsPhoto` | `false` |
| V02 | `valid JPEG under 10MB` | `test.jpg` | JPEG SOI header + 100 bytes padding | `SendAsPhoto` | `false` |
| V03 | `valid GIF87a` | `test.gif` | `GIF87a` + 100 bytes | `SendAsPhoto` | `false` |
| V04 | `valid GIF89a` | `test.gif` | `GIF89a` + 100 bytes | `SendAsPhoto` | `false` |
| V05 | `valid WebP` | `test.webp` | `RIFF\x00\x00\x00\x00WEBP` + 100 bytes | `SendAsPhoto` | `false` |
| V06 | `SVG always document` | `test.svg` | `<svg xmlns=...><rect/></svg>` | `SendAsDocument` | `false` |
| V07 | `PNG wrong magic bytes` | `test.png` | `this is not a PNG file` | N/A | `true` |
| V08 | `zero-byte file` | `test.png` | `[]byte{}` | N/A | `true` |
| V09 | `file does not exist` | (no file created) | N/A | N/A | `true` |

**Separate large-file test functions** (avoid large allocations in table):

| # | Test Function | Description | Size | Expected |
|---|---|---|---|---|
| V10 | `TestValidateImage_LargePhoto` | Valid PNG header + 11MB padding | 11MB | `SendAsDocument`, no error |
| V11 | `TestValidateImage_TooLarge` | Valid PNG header + 51MB padding | 51MB | Error ("file too large") |

### 2.4 Dedup Logic

**Test Function**: `TestImageDedup`

| # | Name | Setup | Project | Path | Mtime | Expected |
|---|---|---|---|---|---|---|
| D01 | `first occurrence passes` | none | `myproject` | `/tmp/screenshot.png` | `2026-04-11T12:00:00Z` | `true` (send) |
| D02 | `same path same mtime blocked` | Mark sent | `myproject` | `/tmp/screenshot.png` | `2026-04-11T12:00:00Z` | `false` (blocked) |
| D03 | `same path new mtime passes` | Mark sent at T12:00 | `myproject` | `/tmp/screenshot.png` | `2026-04-11T12:05:00Z` | `true` (send) |
| D04 | `different project same path passes` | Mark sent for project-a | `project-b` | `/tmp/screenshot.png` | `2026-04-11T12:00:00Z` | `true` (send) |
| D05 | `different path same project passes` | Mark sent for /tmp/screenshot.png | `myproject` | `/tmp/other.png` | `2026-04-11T12:00:00Z` | `true` (send) |

**Separate TTL and cleanup test functions:**

| # | Test Function | Description |
|---|---|---|
| D06 | `TestImageDedup_TTLExpiry` | Create dedup with 1ms TTL. First call passes, immediate repeat blocked, sleep 5ms, post-TTL call passes. |
| D07 | `TestImageDedup_LazyCleanup` | Insert 100 entries with 1ms TTL, sleep 5ms, call ShouldSend once, verify map size is 1. |

### 2.5 Caption Extraction

**Test Function**: `TestExtractCaption`

| # | Name | Line | pathStart | pathEnd | Expected |
|---|---|---|---|---|---|
| C01 | `short line unchanged` | `Screenshot saved to /tmp/s.png` | 20 | 30 | `Screenshot saved to /tmp/s.png` |
| C02 | `HTML special chars escaped` | `Saved <img> to /tmp/s.png & done` | 15 | 25 | `Saved &lt;img&gt; to /tmp/s.png &amp; done` |
| C03 | `long line truncated around path` | 600 x chars + `/tmp/screenshot.png` + 600 y chars | 600 | 619 | Length <= 1024, contains path substring |

### 2.6 Integration: DetectImagePaths

**Test Function**: `TestDetectImagePaths_Integration`

Uses `t.TempDir()` as repo directory with real files created on disk.

| # | Name | Lines | Files Created | Project | Expected |
|---|---|---|---|---|---|
| I01 | `detects absolute path to existing PNG` | `["Screenshot saved to <TMPDIR>/output/screen.png"]` | `output/screen.png` (valid PNG) | `proj` | `["<TMPDIR>/output/screen.png"]` |
| I02 | `detects relative path resolved against repoDir` | `["Created ./output/chart.png"]` | `output/chart.png` (valid PNG) | `proj` | `["<TMPDIR>/output/chart.png"]` |
| I03 | `skips non-existent file` | `["Saved to ./output/missing.png"]` | none | `proj` | `nil` |
| I04 | `skips file with wrong magic bytes` | `["Saved to ./output/fake.png"]` | `output/fake.png` (text) | `proj` | `nil` |
| I05 | `SVG detected as document` | `["Diagram: ./docs/arch.svg"]` | `docs/arch.svg` (SVG content) | `proj` | path returned, `SendMode == SendAsDocument` |
| I06 | `dedup blocks second detection` | `["Screenshot: ./output/screen.png"]` (called twice) | `output/screen.png` (valid PNG) | `proj` | First call returns ref, second returns `nil` |
| I07 | `multiple images in one screen` | Two lines with different image paths | Both files valid | `proj` | Both paths returned |

---

## 3. Unit Tests: `internal/telegram/bot_test.go` (additions)

### 3.1 Multipart Upload

| # | Test Function | Description | Expected |
|---|---|---|---|
| M01 | `TestRawMultipartCall` | POST temp PNG to httptest.NewServer. Verify Content-Type, all form fields, file content, filename. | Success, no error |
| M02 | `TestRawMultipartCall_ErrorResponse` | Server returns HTTP 400 with Telegram error JSON. | Error containing "400" |
| M03 | `TestRawMultipartCall_FileNotFound` | Call with path `/nonexistent/file.png`. | Error containing "open" |

### 3.2 sendEvent with Images

| # | Test Function | Description | Expected |
|---|---|---|---|
| S01 | `TestSendEvent_ImagesBeforeText` | Event with one `SendAsPhoto` image + screen content. Record API call sequence. | `sendPhoto` before `sendMessage` |
| S02 | `TestSendEvent_ImageErrorDoesNotBlockText` | Server returns 400 for `sendPhoto`, 200 for `sendMessage`. | `sendMessage` called despite image failure |

### 3.3 Test Infrastructure

| # | Description |
|---|---|
| T01 | Add `apiBase` field to `Bot` struct (empty = production URL, non-empty = test server URL) |
| T02 | Add `apiURL(method string) string` helper used by both `rawAPICall` and `rawMultipartCall` |

---

## 4. Edge Case Tests Not Covered in the LLD

These test cases address gaps identified during the acceptance criteria cross-reference and risk analysis.

### 4.1 image_test.go additions

| # | Test Function | Description | AC |
|---|---|---|---|
| E01 | `TestDetectImagePaths_OnlyScreenContent` | Call `DetectImagePaths` with lines containing no image paths. Verify no images returned even if image files exist in repoDir. Validates detection is output-driven, not filesystem-driven. | AC-1 |
| E06 | `TestDetectImagePaths_FalsePositivePrevention` | Lines with paths matching regex in `ls -la` and git log format where valid image files exist at those paths. Verify images ARE returned (documenting that os.Stat + magic bytes is the filter, not context awareness). | AC-4 |
| E07 | `TestValidateImage_Symlink` | Create valid PNG + symlink to it. Validate the symlink path succeeds with `SendAsPhoto`. | AC-1 |
| E08 | `TestDetectImagePaths_FileTruncatedBetweenCalls` | Create valid PNG, detect it, truncate to 0 bytes, detect again with fresh dedup. Second detection returns nil (0-byte check). | AC-4 |
| E09 | `TestImageDedup_ConcurrentAccess` | 100 goroutines call `ShouldSend` with same args. Exactly 1 returns true. Run with `-race`. | AC-5 |
| E10 | `TestDetectImagePaths_EmptyAndWhitespaceLines` | Lines: `["", "   ", "\t"]`. No matches, no panics. | AC-1 |
| E11 | `TestExtractCaption_ExactlyMaxLen` | Line is exactly 1024 chars after escaping. Returned as-is. | AC-1 |
| E12 | `TestExtractCaption_PathAtLineStart` | Path starts at index 0. No negative index issues on truncation. | AC-1 |
| E13 | `TestExtractCaption_PathAtLineEnd` | Path ends at last char. No out-of-bounds on truncation. | AC-1 |

### 4.2 bot_test.go additions

| # | Test Function | Description | AC |
|---|---|---|---|
| E02 | `TestSendEvent_KeyboardOnTextNotImage` | EventPermission with image. Verify `sendPhoto` request has no `reply_markup`, `sendMessage` request has `reply_markup`. | AC-3 |
| E04 | `TestSendEvent_MultipleImagesSequential` | Event with 3 images (photo, document, photo) + text. Verify call sequence matches send modes and all precede `sendMessage`. | AC-3 |
| E05 | `TestSendEvent_NoRetryOnImageFailure` | Server returns 500 for `sendPhoto`. Verify `sendPhoto` called exactly once (no retry in MVP). | AC-4 |
| E14 | `TestSendEvent_SVGSentAsDocument` | Event with SVG image. Verify API method is `sendDocument` and file field is `document`. | AC-1 |
| E15 | `TestSendEvent_ImagesOnlyNoText` | Event where text formatter returns empty but Images populated. Verify images sent and `recordSendOK` called. | AC-3 |
| E16 | `TestSendEvent_NoTopicSkipsImages` | Event for project with no topic. Verify zero API calls. | AC-4 |

### 4.3 bridge_test.go addition

| # | Test Function | Description | AC |
|---|---|---|---|
| E03 | `TestBridge_ImageEventsRespectShouldSend` | Verify that when `shouldSend` returns false (rate-limited), the entire event including images is dropped. | AC-3 |

---

## 5. Test Summary

### Test Count by Area

| Area | File | Test Cases | LLD Specified | New (This Plan) |
|---|---|---|---|---|
| Path detection regex | `image_test.go` | 27 | 25 | 2 |
| Path resolution | `image_test.go` | 6 | 6 | 0 |
| File validation | `image_test.go` | 11 | 10 | 1 |
| Dedup logic | `image_test.go` | 7 | 7 | 0 |
| Caption extraction | `image_test.go` | 6 | 3 | 3 |
| DetectImagePaths integration | `image_test.go` | 11 | 7 | 4 |
| Multipart upload | `bot_test.go` | 3 | 3 | 0 |
| sendEvent integration | `bot_test.go` | 8 | 2 | 6 |
| Bridge rate limiting | `bridge_test.go` | 1 | 0 | 1 |
| Concurrency | `image_test.go` | 1 | 0 | 1 |
| **Total** | | **81** | **63** | **18** |

### Acceptance Criteria Coverage Summary

| Criterion | Status | Notes |
|---|---|---|
| AC-1 | **Fully Covered** | All 8 sub-criteria mapped to tests. E01 fills the output-only gap. |
| AC-2 | **Deferred** | Not in MVP scope. No tests needed now. |
| AC-3 | **Fully Covered** | E02, E03, E04 fill gaps for keyboard placement, rate limiting, and multi-image behavior. |
| AC-4 | **Fully Covered** | E05 documents no-retry MVP behavior. E06 documents false positive residual risk. |
| AC-5 | **Fully Covered** | All 6 sub-criteria mapped. Terminal-to-PNG deferred with AC-2. |

### Risks Addressed by New Test Cases

| Risk | Test Case | Mitigation |
|---|---|---|
| Inline keyboard attached to image instead of text | E02 | Verify reply_markup absent on sendPhoto, present on sendMessage |
| Images bypass bridge rate limiting | E03 | Verify shouldSend gates the entire event |
| Silent retry loop on API failure | E05 | Verify single call, documenting MVP behavior |
| Symlink traversal broken | E07 | Verify os.Stat follows symlinks |
| Data race on concurrent dedup | E09 | 100-goroutine race under -race flag |
| Panic on empty input lines | E10 | Graceful handling of edge-case input |
| Caption index out of bounds | E12, E13 | Path at line boundaries tested |
| Images sent but recordSendOK not called | E15 | Verify images-only event path |
| Images sent to project with no topic | E16 | Verify early return before any API calls |

---

## 6. Non-Functional Verification

These items are verified during code review and manual testing, not automated unit tests.

| # | Aspect | Verification Method |
|---|---|---|
| NF01 | `DetectImagePaths` completes in < 1ms for 40-line screen | Add `t.Logf` timing in integration test; review in CI output |
| NF02 | No new external dependencies introduced | Review `go.mod` diff |
| NF03 | SPDX license header on all new files | Code review checklist |
| NF04 | Structured logging uses `logging.Debug` / `logging.Error` consistently | Code review checklist |
| NF05 | No panics possible in the image pipeline | Review all slice operations, regex index access, nil checks |
| NF06 | Memory bounded: dedup map capped by TTL cleanup | Verified by D07 |
| NF07 | rawMultipartCall buffers at most 50MB | Verified by documentMaxSize pre-check ordering in code review |
