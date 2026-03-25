# OpenConductor demo — run with: vhs demo.tape
# Prerequisites: ./scripts/demo-setup.sh && make build
# Requires: claude (Pro/Max auth), opencode installed

Output assets/demo.gif

Set Shell bash
Set FontFamily "Menlo"
Set FontSize 14
Set Width 1400
Set Height 800
Set Padding 16
Set Framerate 30
Set PlaybackSpeed 1.0
Set TypingSpeed 35ms
Set WindowBar Colorful
Set WindowBarSize 40
Set CursorBlink false

Hide
Type "bash ./scripts/demo-setup.sh && rm -f /tmp/demo-test.txt /tmp/hello.txt"
Enter
Sleep 1s
Type "OC_CONFIG_PATH=/tmp/oc-demo-config.yaml OC_STATE_PATH=/tmp/oc-demo-state.json OC_LOG_DIR=/tmp/oc-demo-logs ./openconductor"
Enter
Sleep 3s
Show

# Tab order after setup: saas-app(1) home-infra(2) local-poc(3) billing-api(4)
# Ctrl+K = forward (1→2→3→4→1), Ctrl+J = backward

# ══════════════════════════════════════════════════════════════════
# ADD PROJECTS: OC, CC, OC, CC
# ══════════════════════════════════════════════════════════════════

# 1. saas-app — OpenCode + Vibe persona + minimax
Type "a"
Sleep 700ms
Type "saas-app"
Sleep 200ms
Enter
Sleep 300ms
Type "/tmp/oc-demo/saas-app"
Sleep 200ms
Enter
Sleep 2s
Enter
Sleep 2s
# Persona: Vibe (Down 1)
Down
Sleep 500ms
Enter
Sleep 2s
# Auto-approve: Vibe defaults to Full
Enter
Sleep 2s
Ctrl+P
Sleep 1500ms
Enter
Sleep 1500ms
Type "minimax"
Sleep 1s
Enter
Sleep 1s
Ctrl+S
Sleep 300ms

# 2. home-infra — Claude Code + Scale persona
Type "a"
Sleep 700ms
Type "home-infra"
Sleep 200ms
Enter
Sleep 300ms
Type "/tmp/oc-demo/home-infra"
Sleep 200ms
Enter
Sleep 2s
Down
Sleep 2s
Enter
Sleep 2s
# Persona: Scale (Down 3)
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Enter
Sleep 2s
# Auto-approve: Scale defaults to Off
Enter
Sleep 4s
Enter
Sleep 4s
Ctrl+S
Sleep 300ms

# 3. local-poc — OpenCode + POC persona + minimax
Type "a"
Sleep 700ms
Type "local-poc"
Sleep 200ms
Enter
Sleep 300ms
Type "/tmp/oc-demo/local-poc"
Sleep 200ms
Enter
Sleep 2s
Enter
Sleep 2s
# Persona: POC (Down 2)
Down
Sleep 400ms
Down
Sleep 400ms
Enter
Sleep 2s
# Auto-approve: POC defaults to Safe
Enter
Sleep 2s
Ctrl+P
Sleep 1500ms
Enter
Sleep 1500ms
Type "minimax"
Sleep 1s
Enter
Sleep 1s
Ctrl+S
Sleep 300ms

# 4. billing-api — Claude Code + Scale persona
Type "a"
Sleep 700ms
Type "billing-api"
Sleep 200ms
Enter
Sleep 300ms
Type "/tmp/oc-demo/billing-api"
Sleep 200ms
Enter
Sleep 2s
Down
Sleep 2s
Enter
Sleep 2s
# Persona: Scale (Down 3)
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Enter
Sleep 2s
# Auto-approve: Scale defaults to Off
Enter
Sleep 4s
Enter
Sleep 4s
# Active: billing-api (4) / Claude Code

# ══════════════════════════════════════════════════════════════════
# TRIGGER: Claude Code question (billing-api) + answer it
# ══════════════════════════════════════════════════════════════════

Type "Before doing anything, ask me: which testing framework do you want? Options: 1. Jest 2. Vitest 3. Playwright. Wait for my answer."
Enter
Sleep 15s

# Answer "2" (Vitest) — we're still on billing-api tab
Type "2"
Sleep 500ms
Enter
Sleep 5s

# ══════════════════════════════════════════════════════════════════
# TRIGGER: OpenCode question (saas-app)
# From billing-api(4), Ctrl+J×3 → saas-app(1)
# ══════════════════════════════════════════════════════════════════

Ctrl+J
Sleep 1s
Ctrl+J
Sleep 1s
Ctrl+J
Sleep 1s

Type "Before writing any code, ask me which testing framework and CSS approach I prefer. Show numbered options for each."
Enter
Sleep 15s

# Answer Testing Framework (Enter = first option)
Enter
Sleep 2s
# Answer CSS Approach (Enter = first option)
Enter
Sleep 2s
# Confirm
Enter
Sleep 3s

# ══════════════════════════════════════════════════════════════════
# TRIGGER: OpenCode permission (local-poc) — write to /tmp
# From saas-app(1), Ctrl+K×2 → local-poc(3)
# ══════════════════════════════════════════════════════════════════

Ctrl+K
Sleep 1s
Ctrl+K
Sleep 1s

Type "Create a new file at /tmp/demo-test.txt containing hello world"
Enter
Sleep 15s

# Approve: Tab to "Allow always", Enter to confirm
Tab
Sleep 1s
Enter
Sleep 5s

# ══════════════════════════════════════════════════════════════════
# Send normal prompt to home-infra (Claude Code)
# From local-poc(3), Ctrl+J → home-infra(2)
# ══════════════════════════════════════════════════════════════════

Ctrl+J
Sleep 1s

Type "Create a main.tf with a VPC and public subnet"
Enter
Sleep 10s

# ══════════════════════════════════════════════════════════════════
# Cycle all tabs to show results
# ══════════════════════════════════════════════════════════════════

Ctrl+J
Sleep 3s
Ctrl+K
Sleep 3s
Ctrl+K
Sleep 3s
Ctrl+K
Sleep 3s

# ══════════════════════════════════════════════════════════════════
# Agent switch with 's' on saas-app
# From billing-api(4), Ctrl+J×3 → saas-app(1)
# ══════════════════════════════════════════════════════════════════

Ctrl+J
Sleep 1s
Ctrl+J
Sleep 1s
Ctrl+J
Sleep 1s

Ctrl+S
Sleep 400ms
Up
Sleep 300ms
Up
Sleep 300ms
Up
Sleep 300ms
Type "s"
Sleep 4s
Enter
Sleep 4s

# ══════════════════════════════════════════════════════════════════
# Sidebar navigation + scrollback
# ══════════════════════════════════════════════════════════════════

Ctrl+S
Sleep 300ms
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms
Up
Sleep 400ms
Up
Sleep 400ms
Up
Sleep 400ms
Ctrl+S
Sleep 300ms

PageUp
Sleep 1s
PageUp
Sleep 1s
PageDown
Sleep 1s
PageDown
Sleep 1s

# ══════════════════════════════════════════════════════════════════
# Exit
# ══════════════════════════════════════════════════════════════════

Ctrl+C
Sleep 500ms
Ctrl+C
Sleep 1s
