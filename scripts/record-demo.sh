#!/opt/homebrew/bin/bash
# record-demo.sh — Scripted demo for asciinema recording.
#
# Usage:
#   asciinema rec assets/demo.cast -c ./scripts/record-demo.sh --cols 120 --rows 32
#
# Then convert:
#   agg --font-family "JetBrains Mono,Menlo" --font-size 16 assets/demo.cast assets/demo.gif
#
set -e

# Simulated typing — prints characters one at a time with a delay.
simtype() {
    local text="$1"
    local delay="${2:-0.04}"
    for ((i = 0; i < ${#text}; i++)); do
        printf '%s' "${text:$i:1}"
        sleep "$delay"
    done
}

# Send a keystroke to the openconductor process via its PTY.
# We'll use the file descriptor from the coprocess.
sendkey() {
    printf '%s' "$1" >&"${OC[1]}"
}

sendline() {
    sendkey "$1"
    sendkey $'\r'
}

pause() { sleep "${1:-1}"; }

# Pre-create temp dirs.
mkdir -p /tmp/oc-demo-api /tmp/oc-demo-web

clear

# Start openconductor as a coprocess so we can send it keystrokes.
coproc OC { ./openconductor 2>/dev/null; }
OC_PID=$!

# Give it time to start and render.
sleep 2

# ── Add first project ────────────────────────────────────────────
sendkey "a"                    # Open add-project form
pause 0.8
simtype "api" 0.06 >&"${OC[1]}"   # Project name
pause 0.3
sendkey $'\r'                  # Enter → next step
pause 0.3
simtype "/tmp/oc-demo-api" 0.03 >&"${OC[1]}"  # Repo path
pause 0.3
sendkey $'\r'                  # Enter → agent step
pause 0.3
# Navigate to opencode (index 3): Down Down Down
sendkey $'\x1b[B'             # Down
sendkey $'\x1b[B'             # Down
sendkey $'\x1b[B'             # Down
pause 0.3
sendkey $'\r'                  # Enter → persona step
pause 0.3
# Persona: Vibe (Down 1)
sendkey $'\x1b[B'
pause 0.3
sendkey $'\r'                  # Enter → auto-approve (Vibe defaults to Full)
pause 0.3
sendkey $'\r'                  # Enter → confirm
pause 3

# ── Add second project ───────────────────────────────────────────
sendkey $'\x13'               # Ctrl+S → focus sidebar
pause 0.6
sendkey "a"                    # Open form
pause 0.8
simtype "web" 0.06 >&"${OC[1]}"
pause 0.3
sendkey $'\r'
pause 0.3
simtype "/tmp/oc-demo-web" 0.03 >&"${OC[1]}"
pause 0.3
sendkey $'\r'
pause 0.3
sendkey $'\x1b[B'
sendkey $'\x1b[B'
sendkey $'\x1b[B'
pause 0.3
sendkey $'\r'                  # Enter → persona step
pause 0.3
# Persona: POC (Down 2)
sendkey $'\x1b[B'
sendkey $'\x1b[B'
pause 0.3
sendkey $'\r'                  # Enter → auto-approve (POC defaults to Safe)
pause 0.3
sendkey $'\r'                  # Enter → confirm
pause 3

# ── Navigate ─────────────────────────────────────────────────────
sendkey $'\x13'               # Ctrl+S → focus terminal
pause 1

# Switch tabs.
sendkey $'\x0a'               # Ctrl+J → next tab
pause 2
sendkey $'\x0b'               # Ctrl+K → prev tab
pause 2
sendkey $'\x0a'               # Ctrl+J → next tab again
pause 1.5

# ── Exit ─────────────────────────────────────────────────────────
sendkey $'\x03'               # Ctrl+C
pause 0.6
sendkey $'\x03'               # Ctrl+C again → exit
pause 1

wait "$OC_PID" 2>/dev/null || true
