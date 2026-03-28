#!/bin/bash
# Records the OpenConductor demo via asciinema.
# Run: asciinema rec --cols 160 --rows 45 -c "./scripts/demo-record.sh" /tmp/demo.cast
set -e

bash ./scripts/demo-setup.sh >/dev/null 2>&1

send() { sleep "${2:-0.3}"; printf '%s' "$1"; }
sendln() { send "$1" "${2:-0.3}"; printf '\r'; }
key() { sleep "${2:-0.3}"; printf '%s' "$1"; }
ctrl_s() { sleep "${2:-0.3}"; printf '\x13'; }
ctrl_k() { sleep "${2:-0.3}"; printf '\x0b'; }
ctrl_j() { sleep "${2:-0.3}"; printf '\x0a'; }
ctrl_p() { sleep "${2:-0.3}"; printf '\x10'; }
ctrl_c() { sleep "${2:-0.3}"; printf '\x03'; }
enter() { sleep "${2:-0.3}"; printf '\r'; }
down() { sleep "${2:-0.3}"; printf '\x1b[B'; }
up() { sleep "${2:-0.3}"; printf '\x1b[A'; }
pgup() { sleep "${2:-0.3}"; printf '\x1b[5~'; }
pgdn() { sleep "${2:-0.3}"; printf '\x1b[6~'; }
marker() { printf '\x1b]1337;SetMark\x07'; } # asciinema marker

export OC_CONFIG_PATH=/tmp/oc-demo-config.yaml
export OC_STATE_PATH=/tmp/oc-demo-state.json
export OC_LOG_DIR=/tmp/oc-demo-logs

# Start OpenConductor in background, feed it keystrokes
./openconductor &
OC_PID=$!
sleep 3

# === 1. saas-app — OpenCode + minimax ===
printf 'a'; sleep 0.7
printf 'saas-app'; sleep 0.2; enter
sleep 0.3; printf '/tmp/oc-demo/saas-app'; sleep 0.2; enter
sleep 2; enter  # agent: opencode
sleep 2; enter  # auto-approve: off
sleep 2
# Pick minimax model
ctrl_p; sleep 0.7; enter; sleep 0.7
printf 'minimax'; sleep 0.7; enter
sleep 1
ctrl_s 0.3

# === 2. home-infra — Claude Code ===
printf 'a'; sleep 0.7
printf 'home-infra'; sleep 0.2; enter
sleep 0.3; printf '/tmp/oc-demo/home-infra'; sleep 0.2; enter
sleep 2; down; sleep 2; enter  # agent: claude-code
sleep 2; enter  # auto-approve
sleep 4; enter  # trust folder
sleep 4
ctrl_s 0.3

# === 3. local-poc — OpenCode + minimax ===
printf 'a'; sleep 0.7
printf 'local-poc'; sleep 0.2; enter
sleep 0.3; printf '/tmp/oc-demo/local-poc'; sleep 0.2; enter
sleep 2; enter  # agent: opencode
sleep 2; enter  # auto-approve
sleep 2
ctrl_p; sleep 0.7; enter; sleep 0.7
printf 'minimax'; sleep 0.7; enter
sleep 1
ctrl_s 0.3

# === 4. billing-api — Claude Code ===
printf 'a'; sleep 0.7
printf 'billing-api'; sleep 0.2; enter
sleep 0.3; printf '/tmp/oc-demo/billing-api'; sleep 0.2; enter
sleep 2; down; sleep 2; enter  # agent: claude-code
sleep 2; enter  # auto-approve
sleep 4; enter  # trust folder
sleep 4

# === Send prompts ===
printf 'Create a Dockerfile for this Go service'; enter
sleep 3

ctrl_k 1
printf 'What is the tech stack of this project?'; enter
sleep 3

ctrl_k 1
printf 'Create a main.tf with a VPC and public subnet'; enter
sleep 3

ctrl_k 1
printf 'What does the analyze function in main.py do?'; enter
sleep 3

# === Cycle tabs ===
ctrl_j 2; ctrl_j 2; ctrl_j 2; ctrl_j 3
sleep 6
ctrl_k 2; ctrl_k 2; ctrl_k 2

# === Agent switch on saas-app ===
ctrl_k 1
ctrl_s 0.4
up 0.3; up 0.3; up 0.3
printf 's'; sleep 4; enter; sleep 4

# === Navigation ===
ctrl_s 0.4
down 0.4; down 0.4; down 0.4
up 0.4; up 0.4; up 0.4
ctrl_s 0.4

pgup 1; pgup 1; pgdn 1; pgdn 1

# === Exit ===
ctrl_c 0.5; ctrl_c 1

wait $OC_PID 2>/dev/null
