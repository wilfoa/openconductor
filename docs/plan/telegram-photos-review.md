# Architecture Review: LLD for Telegram Photo Sending

**Reviewer**: Architect reviewer
**Date**: 2026-04-11
**Documents reviewed**:
- HLD: `docs/plan/telegram-photos-hld.md`
- LLD: `docs/plan/lld-telegram-photos.md`
- User story: `docs/user-stories/telegram-photo-sending.md`
- Source: `internal/telegram/bot.go`, `internal/telegram/bridge.go`
- Source: `internal/tui/app.go` (lines 81-115, 2459-2562)

**Verdict: Revision Required**

Seven issues found: two must be resolved before implementation, five are advisory.


## 1. LLD-to-HLD Alignment

### 1.1 PASS: Component boundaries

The LLD correctly places detection in `app.go` (TUI layer), types/validation in `image.go` (new file), and transport in `bot.go`. This matches HLD section 2 and the dependency graph in HLD section 2.5. The rationale for TUI-side detection (repo dir access, filesystem freshness, consistency with existing filtering) is correctly reproduced.

### 1.2 PASS: ImageRef simplification is well-justified

The LLD drops `ModTime`, `Size`, and `Format` from `ImageRef` (HLD section 3.1 includes them). The LLD section 1.2 explains why: these fields are consumed transiently during `DetectImagePaths` and are dead weight on the struct once it enters the `Event`. This is a sound simplification. The dedup key is computed and stored in `ImageDedup.sent`, not carried on the ref.

### 1.3 PASS: Event struct extension

The LLD adds `Images []ImageRef` to `Event` in `bridge.go`, matching HLD section 3.3. Verified against the actual `Event` struct in `bridge.go` (line 28-34): the field appends cleanly after `Screen []string`.

### 1.4 PASS: sendEvent modification

The LLD's revised `sendEvent` (section 3.4) correctly inserts an image-sending loop before the existing text-sending `switch`. Error handling is per-image with `continue`, matching HLD section 9.3 ("Never Block Text"). The addition of `recordSendOK()` for image-only events (no text messages) is a good catch that the HLD did not specify.

### 1.5 PASS: Unchanged files list is accurate

Cross-referenced against HLD section 12. `handler.go`, `formatter.go`, `messages.go`, `statusbar.go` are all correctly listed as unchanged, with sound rationale.


## 2. API Contracts

### 2.1 PASS: DetectImagePaths signature

```go
func DetectImagePaths(lines []string, repoDir string, dedup *ImageDedup, project string) []ImageRef
```

Matches the call site in the LLD's `sendTelegramEvent` (section 4.2) and the HLD's dependency graph. The `dedup *ImageDedup` parameter being nilable (for testing) is a practical touch.

### 2.2 PASS: rawMultipartCall signature

```go
func (b *Bot) rawMultipartCall(method string, fields map[string]string, fileField, filePath string) error
```

Parallel to existing `rawAPICall(method string, payload map[string]interface{}) error`. The `fields` type is `map[string]string` (not `map[string]interface{}`), which is correct because multipart form fields are always strings. No implicit JSON marshaling needed.

### 2.3 PASS: sendPhotoToTopic / sendDocumentToTopic

Signatures mirror `sendToTopic(topicID int, text string, keyboard *tgbotapi.InlineKeyboardMarkup)` in style. Both use `strconv.FormatInt(b.cfg.ChatID, 10)` for `chat_id`, matching the existing `sendToTopic` which uses `b.cfg.ChatID` as `int64`. Consistent.

### 2.4 PASS: apiURL helper and test infrastructure

The `apiBase` field on `Bot` (section 5.7.1) is a clean approach for test overrides. The zero value preserves production behavior. Both `rawAPICall` and `rawMultipartCall` would use `b.apiURL(method)`. This requires modifying the existing `rawAPICall` to also use the helper -- the LLD notes this correctly.


## 3. Regex Analysis

### 3.1 ISSUE [MUST FIX]: Shell redirect `> output/dir/file.png` is not excluded

The HLD's regex (section 6.1) excludes `>` from the path character class (`[^\s"'<>|*?]`), which prevents `> output.png` from matching. However, the user story (section 5, edge cases) specifically calls for excluding redirections.

The LLD's revised regex (section 1.14):
```
`(?:^|[\s"'=(,])` + `(~?(?:\.{0,2}/)?[^\s"'<>|*?]+/[^\s"'<>|*?/]+\.(?i:png|jpe?g|gif|webp|svg))` + `(?:[\s"'),:;.\]}]|$)`
```

Consider the input: `echo data > ./output/file.png`

The `>` character is followed by a space, then `./output/file.png`. The left boundary `[\s"'=(,]` matches the space after `>`. The path `./output/file.png` matches the capture group. The `>` character itself is not in the path, so it passes the `[^\s"'<>|*?]` character class.

Result: `./output/file.png` IS matched, which is a false positive. The agent typed a redirect command; the file may not exist yet. The `os.Stat` check will likely catch this (the file may not exist before the command runs), but if the file already exists from a prior run, it would be sent.

The HLD's original regex had `>` in the right boundary exclusion but not the left boundary context. The LLD's regex inherits this gap.

**Recommendation**: Either add `>` to the left boundary character set (so `>` preceding a space does not match), or check for `>` or `>>` as the preceding non-whitespace token during post-match filtering. A simple approach: after regex matching, check if the character before the left boundary whitespace is `>` and skip the match.

### 3.2 ISSUE [ADVISORY]: URL exclusion relies on `:` not being in the left boundary set

The LLD claims (section 1.5, point 2): "https://example.com/image.png does not match because `//example.com/image.png` starts after `https:` which leaves `:` as the preceding character."

This is correct for `https://`, but consider `ftp://host/path/to/image.png` -- same mechanism, so it works. However, consider a bare URL without scheme like `//cdn.example.com/images/logo.png` (protocol-relative URL). The left boundary `[\s"'=(,]` could match a preceding space, and `//cdn.example.com/images/logo.png` starts with `/` which is a valid absolute path prefix. The `os.Stat` call would fail (path does not exist locally), so this is a self-correcting false positive, but it is worth noting in a comment.

### 3.3 PASS: Test coverage of regex edge cases

The test table in LLD section 5.1 has 16 positive cases and 11 negative cases. This covers: absolute, relative, home-relative, quoted, parenthesized, deeply nested, bare relative with subdirectory, multiple paths, and all extension variants. Negative cases cover globs, URLs, bare filenames, pipes, extension-in-word, and color codes. Good coverage.

### 3.4 ISSUE [ADVISORY]: Spaces in paths are a known limitation but the test documents a wrong match

Test case "path with spaces in parent dir (quoted)" expects `want: []string{"/Users/dev/My"}`. This path would fail `os.Stat` (truncated), so the false positive is self-correcting. However, this means the regex silently drops the second half of the real path. If the directory `/Users/dev/My` happens to exist on the machine, `validateImage` would fail because it is a directory, not a file. This is handled (skip directories). Acceptable, but the test comment should note the functional impact: paths with spaces are NOT supported, not just partially supported.


## 4. Multipart Upload Implementation

### 4.1 PASS: Buffer-based approach is appropriate

The LLD correctly justifies buffering vs streaming (section 3.1 comment). Files are bounded at 50MB. Agent screenshots are typically under 1MB. The `io.Copy(part, f)` pattern is correct for Go's `multipart.Writer`.

### 4.2 PASS: Content-Type and boundary handling

`w.FormDataContentType()` returns the correct `Content-Type` header including the boundary string. This is the standard Go pattern.

### 4.3 ISSUE [ADVISORY]: `http.Post` does not use a shared client with timeout

The existing `rawAPICall` in `bot.go` (line 740) uses `http.Post` (the default client, no timeout). The LLD's `rawMultipartCall` also uses `http.Post`. For consistency this is fine, but both lack a timeout. A 50MB upload on a slow connection could block the `bridgeLoop` goroutine indefinitely.

The `pollLoop` uses a dedicated `http.Client{Timeout: 40 * time.Second}` (line 360), showing awareness of timeouts in the codebase. The upload path does not. This is a pre-existing issue, not introduced by the LLD, but worth noting for a follow-up: use a shared `http.Client` on the `Bot` struct with a reasonable timeout (e.g., 120s for uploads).

### 4.4 PASS: Error response parsing

The `rawMultipartCall` response parsing (checking HTTP status, then `{"ok": false}`) mirrors `rawAPICall` exactly. Consistent.

### 4.5 ISSUE [ADVISORY]: Missing import `path/filepath` in bot.go

The LLD notes in section 7 that `path/filepath` is not currently imported in `bot.go` and needs to be added for `filepath.Base(filePath)` in `rawMultipartCall`. Confirmed: grep for `path/filepath` in `bot.go` returns no matches. The LLD is aware of this and documents it. Minor, but easy to miss during implementation.


## 5. Error Handling vs User Story Requirements

### 5.1 ISSUE [MUST FIX]: User story requires retry on send failure; LLD has no retry

User story section 5 ("Telegram API" edge cases):
- "sendPhoto fails (network error): Retry once after 2s. If still failing, log error and continue with text-only event"
- "sendPhoto returns 413 (file too large): Fall back to sendDocument. If that also fails, log and skip"

HLD section 9.2 states: "No retry in MVP."

The user story section 10 ("Resolved Design Decisions") settles this: it was a decision to defer retry and use sequential sends. However, section 5's error table still says "retry once after 2s." The user story has an internal contradiction between sections 5 and 10.

The LLD aligns with the HLD ("No retry in MVP") but contradicts the user story's error table. The 413-to-sendDocument fallback is also absent from the LLD -- when `sendPhoto` fails, the LLD logs and skips regardless of the HTTP status code.

**Recommendation**: Either (a) add the 413 fallback to `sendEvent`'s image loop (check the error string for "413" and retry with `sendDocumentToTopic`), or (b) explicitly document in the LLD that the 413 fallback is deferred along with retry logic, and update the user story's error table to match. Option (b) is acceptable if the HLD's "No retry in MVP" decision takes precedence. But the 413 fallback is cheap to implement (3-5 lines) and should be included.

### 5.2 PASS: "Never block text" invariant

The LLD enforces this via per-image `if err != nil { log; continue }` in the image loop, followed by the unchanged text-sending code. Test `TestSendEvent_ImageErrorDoesNotBlockText` (section 5.8.1) verifies this directly.

### 5.3 PASS: File-deleted-between-detection-and-send

Documented in HLD section 9.2, LLD section 8, and the concurrency analysis (section 8.3). The TOCTOU gap is acknowledged and handled: `rawMultipartCall`'s `os.Open` returns an error, which is logged and skipped.


## 6. Dedup Logic

### 6.1 PASS: Thread safety

`ImageDedup` uses `sync.Mutex`. All access is through `ShouldSend`, which locks, cleans up, checks, records, and unlocks. The LLD correctly notes that `sendTelegramEvent` runs on the TUI goroutine (Bubble Tea's `Update` loop), so concurrent access is unlikely today, but the mutex protects against future callers.

### 6.2 PASS: Key format

`project + "|" + absPath + "|" + modTimeUnixNano` -- the separator `|` is not a valid filename character on macOS/Linux. The key correctly encodes project isolation (same path in different projects gets sent to each topic) and mtime sensitivity (regenerated file gets re-sent).

### 6.3 PASS: Lazy cleanup

Cleanup runs on every `ShouldSend` call, which is driven by the 500ms attention tick. With a 5-minute TTL and ~100 bytes per entry, the map is bounded at ~50 entries for a Playwright-heavy session. The O(n) scan is negligible.

### 6.4 PASS: Bridge shouldSend interaction

The LLD section 2 correctly identifies that when `bridge.shouldSend` returns false (text dedup/rate limit), images are also dropped. It argues this is correct because `ImageDedup.ShouldSend` would reject the same images anyway (same path, same mtime, within TTL). This is sound reasoning. The only edge case: an image appears in a new event with different text content but the same image path. In this case, `shouldSend` passes (different screen fingerprint), and `ImageDedup` rejects the image (already sent). This is correct behavior -- the text is sent, the image is not re-sent.

### 6.5 PASS: Test coverage

Tests in section 5.4 cover: first occurrence, same path/mtime blocked, new mtime passes, different project passes, TTL expiry, and lazy cleanup verification (map size after expiry). Comprehensive.


## 7. Conflicts with Existing Code

### 7.1 PASS: No changes to existing function signatures

`sendEvent`, `sendToTopic`, `rawAPICall` retain their existing signatures. The new `rawMultipartCall` is additive. The `Event` struct gains a field (backward-compatible zero value: `nil` slice).

### 7.2 PASS: No new external dependencies

All imports are from Go's standard library (`mime/multipart`, `path/filepath`, `html`, `regexp`) plus the existing `internal/logging` package.

### 7.3 PASS: Existing callers of sendTelegramEvent are unchanged

The LLD correctly notes (section 4.2) that the four call sites in `app.go` (lines 1136, 2410, 2418, 2450) pass the same arguments. Image detection is internal to `sendTelegramEvent`. Verified against the actual source.

### 7.4 PASS: No Bubble Tea message types needed

Images are fire-and-forget within the existing `sendTelegramEvent` -> channel -> `bridgeLoop` -> `sendEvent` flow. No new `tea.Msg` types are needed.


## 8. Test Coverage Assessment

### 8.1 PASS: Unit test structure

Table-driven tests throughout, per project conventions. Five test files/functions for `image_test.go`, three for `bot_test.go`. The integration test (`TestDetectImagePaths_Integration`) creates real files on disk using `t.TempDir()`, which is the correct approach for filesystem-dependent tests.

### 8.2 PASS: Test helper functions

`pngHeader(padding int)` and `jpegHeader(padding int)` are well-designed: they produce files that pass magic byte validation without being valid images. This correctly tests the detection pipeline without depending on image decoding.

### 8.3 ADVISORY: Missing test for `extractCaption` with HTML entities in paths

The caption tests verify HTML escaping of `<img>` and `&` in surrounding text, but do not test a path that itself contains HTML-special characters (unlikely on macOS/Linux but theoretically possible with `&` in filenames). Low priority.

### 8.4 ADVISORY: Missing test for `apiURL` helper

The `apiURL` helper (section 5.7.1) is simple but untested. It would be tested indirectly via `TestRawMultipartCall`, but an explicit unit test would catch regressions if the helper is modified.


## 9. Discrepancies Between User Story and HLD/LLD

These are not LLD defects but should be reconciled before implementation:

| Topic | User Story | HLD | LLD |
|-------|-----------|-----|-----|
| Rate limiting on images | "Each image send counts against the per-project rate limit (existing 3s minSendInterval)" (section 4.6) | "No rate limiting on images. Images are sent immediately." (section 1, decision 2) | Follows HLD: no image-specific rate limit. bridge.shouldSend applies to the Event as a whole. |
| sendMediaGroup for multi-image events | "Telegram's sendMediaGroup API can send up to 10 images in one call" (section 4.6) | "Sequential sendPhoto calls, not sendMediaGroup" (section 1, decision 3) | Follows HLD: sequential calls. |
| Path restriction to repo dir | "Only paths within the project's repo directory (or absolute paths)" (section 3, Flow 1 alternative paths) | "Path is NOT restricted to be within the repo directory" (section 6.2) | Follows HLD: no restriction. |
| File-not-found retry | "Image is queued for a single retry on the next tick" (section 3, Flow 1 alternative paths) | No retry on detection failure | No retry on detection failure |
| Cap of 5 images per event | "Cap at 5 images per event" (section 5, Deduplication) | No cap mentioned | No cap mentioned |

The HLD's "Resolved Decisions" section (section 1) explicitly settles several of these. The user story predates the HLD and contains proposals that were later narrowed. The LLD correctly follows the HLD, not the pre-decision user story text.

**Recommendation**: Update the user story to mark sections overridden by HLD decisions, or add a "Superseded by HLD" note in the user story's header.


## Summary of Required Actions

### Must Fix (2)

1. **Regex: shell redirect false positive** (section 3.1). Add post-match filtering or left-boundary adjustment to exclude paths preceded by `>` or `>>`. Add a test case: `echo data > ./output/file.png` should produce no match.

2. **Reconcile retry/fallback behavior** (section 5.1). Either implement the 413-to-sendDocument fallback (cheap, 3-5 lines in `sendEvent`'s image loop), or explicitly document its deferral in the LLD and update the user story error table.

### Advisory (5)

3. Spaces in paths: clarify the test comment to state that paths with spaces are unsupported, not partially supported (section 3.4).

4. `http.Post` timeout: note as follow-up to use a shared `http.Client` with timeout for upload calls (section 4.3).

5. Add `path/filepath` import to `bot.go` -- already documented in the LLD, just flagging to prevent implementation oversight (section 4.5).

6. Protocol-relative URLs (`//cdn.example.com/...`): add a comment in the regex documentation noting this is handled by `os.Stat` failure (section 3.2).

7. Update the user story header to note that HLD resolved decisions supersede conflicting user story text (section 9).
