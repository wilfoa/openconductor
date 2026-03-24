#!/usr/bin/env bash
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

# ── Add first project ("api" with Scale persona) ──────────────────
sendkey "a"                    # Open add-project form
pause 0.8
simtype "api" 0.06 >&"${OC[1]}"   # Step 1: Project name
pause 0.3
sendkey $'\r'                  # Enter → step 2
pause 0.3
simtype "/tmp/oc-demo-api" 0.03 >&"${OC[1]}"  # Step 2: Repo path
pause 0.3
sendkey $'\r'                  # Enter → step 3 (agent)
pause 0.3
# Agent: claude-code (Down once)
sendkey $'\x1b[B'             # Down
pause 0.3
sendkey $'\r'                  # Enter → step 4 (persona)
pause 0.3
# Persona: browse to Scale (Down Down Down)
sendkey $'\x1b[B'             # Down → Vibe
pause 0.5
sendkey $'\x1b[B'             # Down → POC
pause 0.5
sendkey $'\x1b[B'             # Down → Scale
pause 0.5
sendkey $'\r'                  # Enter → step 5 (auto-approve)
pause 0.3
# Auto-approve: Scale defaults to Off. Accept.
sendkey $'\r'                  # Enter → confirm
pause 3

# ── Add second project ("web" with Vibe persona) ──────────────────
sendkey $'\x13'               # Ctrl+S → focus sidebar
pause 0.6
sendkey "a"                    # Open form
pause 0.8
simtype "web" 0.06 >&"${OC[1]}"   # Step 1: name
pause 0.3
sendkey $'\r'
pause 0.3
simtype "/tmp/oc-demo-web" 0.03 >&"${OC[1]}"  # Step 2: path
pause 0.3
sendkey $'\r'
pause 0.3
# Agent: claude-code
sendkey $'\x1b[B'
pause 0.3
sendkey $'\r'                  # Enter → persona step
pause 0.3
# Persona: Vibe (Down once)
sendkey $'\x1b[B'             # Down → Vibe
pause 0.5
sendkey $'\r'                  # Enter → auto-approve
pause 0.3
# Auto-approve: Vibe defaults to Full. Accept.
sendkey $'\r'
pause 3

# ── Show persona labels in sidebar ─────────────────────────────────
sendkey $'\x13'               # Ctrl+S → focus sidebar
pause 1.5

# ── Change persona on selected project ─────────────────────────────
sendkey "p"                    # Open persona picker
pause 0.8
sendkey $'\x1b[B'             # Down → Vibe
pause 0.4
sendkey $'\x1b[B'             # Down → POC
pause 0.4
sendkey $'\r'                  # Select POC
pause 1
sendkey "y"                    # Confirm session restart
pause 2

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
