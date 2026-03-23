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

# ── Hidden: setup + launch ───────────────────────────────────────
Hide
Type "bash ./scripts/demo-setup.sh"
Enter
Sleep 1s
Type "OC_CONFIG_PATH=/tmp/oc-demo-config.yaml OC_STATE_PATH=/tmp/oc-demo-state.json OC_LOG_DIR=/tmp/oc-demo-logs ./openconductor"
Enter
Sleep 3s
Show

# Focus: SIDEBAR
# Pattern: opencode, claude, opencode, claude

# ══════════════════════════════════════════════════════════════════
# 1. saas-app — OpenCode + minimax
# ══════════════════════════════════════════════════════════════════

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
# Agent: opencode (default, just Enter)
Enter
Sleep 2s
# Auto-approve: Off (default)
Enter
Sleep 2s
# TERMINAL — pick minimax model via Ctrl+P → Switch model
Ctrl+P
Sleep 700ms
Enter
Sleep 700ms
Type "minimax"
Sleep 700ms
Enter
Sleep 1s
# → SIDEBAR for next
Ctrl+S
Sleep 300ms

# ══════════════════════════════════════════════════════════════════
# 2. home-infra — Claude Code (selected in form with j)
# ══════════════════════════════════════════════════════════════════

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
# Agent: press j to select claude-code
Down
Sleep 2s
Enter
Sleep 2s
# Auto-approve: Off
Enter
Sleep 4s
# Trust folder prompt
Enter
Sleep 4s
# TERMINAL (home-infra / claude) → SIDEBAR
Ctrl+S
Sleep 300ms

# ══════════════════════════════════════════════════════════════════
# 3. local-poc — OpenCode + minimax
# ══════════════════════════════════════════════════════════════════

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
# Agent: opencode (default)
Enter
Sleep 2s
# Auto-approve: Off
Enter
Sleep 2s
# TERMINAL — pick minimax
Ctrl+P
Sleep 700ms
Enter
Sleep 700ms
Type "minimax"
Sleep 700ms
Enter
Sleep 1s
# → SIDEBAR
Ctrl+S
Sleep 300ms

# ══════════════════════════════════════════════════════════════════
# 4. billing-api — Claude Code (selected in form with j)
# ══════════════════════════════════════════════════════════════════

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
# Agent: press j to select claude-code
Down
Sleep 2s
Enter
Sleep 2s
# Auto-approve: Off
Enter
Sleep 4s
# Trust folder
Enter
Sleep 4s
# TERMINAL (billing-api / claude)

# ══════════════════════════════════════════════════════════════════
# Send prompts to all 4 agents
# ══════════════════════════════════════════════════════════════════

# billing-api (claude) — create file to show working state
Type "Create a Dockerfile for this Go service"
Enter
Sleep 3s

# saas-app (opencode + minimax)
Ctrl+K
Sleep 1s
Type "What is the tech stack of this project?"
Enter
Sleep 3s

# home-infra (claude)
Ctrl+K
Sleep 1s
Type "Create a main.tf with a VPC and public subnet"
Enter
Sleep 3s

# local-poc (opencode + minimax)
Ctrl+K
Sleep 1s
Type "What does the analyze function in main.py do?"
Enter
Sleep 3s

# ══════════════════════════════════════════════════════════════════
# Cycle through tabs — show working/attention badges
# ══════════════════════════════════════════════════════════════════

Ctrl+J
Sleep 2s
Ctrl+J
Sleep 2s
Ctrl+J
Sleep 2s
Ctrl+J
Sleep 3s

# Let agents finish
Sleep 6s

# Show results
Ctrl+K
Sleep 2s
Ctrl+K
Sleep 2s
Ctrl+K
Sleep 2s

# ══════════════════════════════════════════════════════════════════
# Show 's' agent switch on saas-app
# ══════════════════════════════════════════════════════════════════

# Navigate to saas-app tab
Ctrl+K
Sleep 1s

# Switch to sidebar, select saas-app, press s to switch agent
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
# Trust folder for new claude session
Enter
Sleep 4s

# ══════════════════════════════════════════════════════════════════
# Sidebar navigation + scrollback
# ══════════════════════════════════════════════════════════════════

Ctrl+S
Sleep 400ms
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
Sleep 400ms

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
