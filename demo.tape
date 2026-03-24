# OpenConductor demo — run with: vhs demo.tape
# Requires: openconductor binary built (make build), at least one agent installed.

Output assets/demo.gif

Set Shell bash
Set FontFamily "JetBrains Mono"
Set FontSize 14
Set Width 1400
Set Height 750
Set Padding 12
Set Framerate 24
Set PlaybackSpeed 1.5
Set TypingSpeed 50ms
Set WindowBar Colorful
Set WindowBarSize 40
Set CursorBlink false

# Pre-create temp project directories so the form path completion works.
Hide
Type "mkdir -p /tmp/oc-demo-api /tmp/oc-demo-web"
Enter
Sleep 500ms
Type "clear"
Enter
Sleep 300ms
Show

# ── Launch ────────────────────────────────────────────────────────
Type "./openconductor"
Enter
Sleep 2s

# ── Add first project ("api" with Scale persona) ─────────────────
# Sidebar is focused on first launch with no projects.
Type "a"
Sleep 800ms

# Step 1/5: project name.
Type "api"
Sleep 400ms
Enter
Sleep 400ms

# Step 2/5: repo path.
Type "/tmp/oc-demo-api"
Sleep 400ms
Enter
Sleep 400ms

# Step 3/5: agent selection — claude-code (j).
Down
Sleep 300ms
Enter
Sleep 400ms

# Step 4/5: persona — browse the options then pick Scale (j j j).
Sleep 600ms
Down
Sleep 500ms
Down
Sleep 500ms
Down
Sleep 500ms
Enter
Sleep 400ms

# Step 5/5: auto-approve — Scale defaults to Off, accept it.
# Notice the persona pre-selected "Off" for production quality.
Enter
Sleep 3s

# ── Add second project ("web" with Vibe persona) ─────────────────
# Switch focus back to sidebar.
Ctrl+S
Sleep 600ms

Type "a"
Sleep 800ms

Type "web"
Sleep 400ms
Enter
Sleep 400ms

Type "/tmp/oc-demo-web"
Sleep 400ms
Enter
Sleep 400ms

# Agent: claude-code (j).
Down
Sleep 300ms
Enter
Sleep 400ms

# Persona: pick Vibe (j) — fast and loose.
Down
Sleep 500ms
Enter
Sleep 400ms

# Auto-approve: Vibe defaults to Full. Accept it.
# Notice persona pre-selected "Full" for maximum velocity.
Enter
Sleep 3s

# ── Show persona labels in sidebar ──────────────────────────────
# Notice "claude · Scale · idle" and "claude · Vibe · idle" labels.
Ctrl+S
Sleep 1.5s

# ── Change persona on existing project ──────────────────────────
# Press 'p' to open the persona picker on the selected project.
Type "p"
Sleep 800ms

# Browse persona options.
Down
Sleep 400ms
Down
Sleep 400ms

# Select POC.
Enter
Sleep 1s

# Confirm session restart (y).
Type "y"
Sleep 2s

# ── Navigate: switch focus to terminal ────────────────────────────
Ctrl+S
Sleep 1s

# ── Tab switching ─────────────────────────────────────────────────
Ctrl+J
Sleep 2s
Ctrl+K
Sleep 2s

# ── Switch back one more time to show responsiveness ──────────────
Ctrl+J
Sleep 1.5s

# ── Exit ──────────────────────────────────────────────────────────
Ctrl+C
Sleep 600ms
Ctrl+C
Sleep 1s
