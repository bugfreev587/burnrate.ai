#!/bin/bash
# TokenGate Status Line for Claude Code
#
# Displays real-time budget/cost/usage data from TokenGate gateway alongside
# the standard Claude Code session info.
#
# Setup:
#   1. Copy this script to ~/.claude/tokengate-statusline.sh
#   2. chmod +x ~/.claude/tokengate-statusline.sh
#   3. Configure in ~/.claude/settings.json:
#      { "statusLine": { "type": "command", "command": "~/.claude/tokengate-statusline.sh" } }
#
# Required environment variables:
#   ANTHROPIC_BASE_URL  - TokenGate gateway URL (e.g. https://gateway.tokengate.to)
#   ANTHROPIC_API_KEY   - TokenGate API key (tg_xxx) or set via TOKENGATE_API_KEY
#
# Optional environment variables:
#   TOKENGATE_API_KEY          - Explicit TokenGate key (overrides ANTHROPIC_API_KEY)
#   TOKENGATE_STATUSLINE_POLL  - Cache TTL in seconds (default: 5)
#   TOKENGATE_STATUSLINE_BARS  - Progress bar block count (default: 8)

set -f  # disable globbing

input=$(cat)

if [ -z "$input" ]; then
    printf "Claude"
    exit 0
fi

# ── ANSI colors ──────────────────────────────────────────────────────────────
blue='\033[38;2;0;153;255m'
orange='\033[38;2;255;176;85m'
green='\033[38;2;0;160;0m'
cyan='\033[38;2;46;149;153m'
red='\033[38;2;255;85;85m'
yellow='\033[38;2;230;200;0m'
white='\033[38;2;220;220;220m'
magenta='\033[38;2;200;120;255m'
dim='\033[2m'
reset='\033[0m'

# ── Helpers ──────────────────────────────────────────────────────────────────
format_tokens() {
    local num=$1
    if [ "$num" -ge 1000000 ]; then
        awk "BEGIN {printf \"%.1fm\", $num / 1000000}"
    elif [ "$num" -ge 1000 ]; then
        awk "BEGIN {printf \"%.0fk\", $num / 1000}"
    else
        printf "%d" "$num"
    fi
}

usage_color() {
    local pct=$1
    if [ "$pct" -ge 85 ]; then echo "$red"
    elif [ "$pct" -ge 60 ]; then echo "$orange"
    elif [ "$pct" -ge 40 ]; then echo "$yellow"
    else echo "$green"
    fi
}

# Build a progress bar: [■■■■□□□□]
# Usage: progress_bar <percent> <num_blocks> <color>
progress_bar() {
    local pct=$1
    local blocks=${2:-8}
    local color=${3:-$green}
    local filled=$(( pct * blocks / 100 ))
    [ "$filled" -gt "$blocks" ] && filled=$blocks
    [ "$filled" -lt 0 ] && filled=0
    local empty=$(( blocks - filled ))

    local bar="["
    for (( i=0; i<filled; i++ )); do bar+="■"; done
    for (( i=0; i<empty; i++ )); do bar+="□"; done
    bar+="]"
    echo "$bar"
}

sep=" ${dim}|${reset} "

# ── Standard Claude Code info ────────────────────────────────────────────────
model_name=$(echo "$input" | jq -r '.model.display_name // "Claude"')

size=$(echo "$input" | jq -r '.context_window.context_window_size // 200000')
[ "$size" -eq 0 ] 2>/dev/null && size=200000

input_tokens=$(echo "$input" | jq -r '.context_window.current_usage.input_tokens // 0')
cache_create=$(echo "$input" | jq -r '.context_window.current_usage.cache_creation_input_tokens // 0')
cache_read=$(echo "$input" | jq -r '.context_window.current_usage.cache_read_input_tokens // 0')
current=$(( input_tokens + cache_create + cache_read ))

used_tokens=$(format_tokens $current)
total_tokens=$(format_tokens $size)

out=""
out+="${blue}${model_name}${reset}"

cwd=$(echo "$input" | jq -r '.cwd // empty')
if [ -n "$cwd" ]; then
    display_dir="${cwd##*/}"
    out+="${sep}${cyan}${display_dir}${reset}"
fi

out+="${sep}${orange}${used_tokens}/${total_tokens}${reset}"

# ── Context window utilization (always available from Claude Code input) ─────
ctx_pct=0
ctx_remain=100
if [ "$size" -gt 0 ] 2>/dev/null; then
    ctx_pct=$(( current * 100 / size ))
    ctx_remain=$(( 100 - ctx_pct ))
fi

# ── TokenGate budget/cost data ───────────────────────────────────────────────
tg_key="${TOKENGATE_API_KEY:-$ANTHROPIC_API_KEY}"
tg_base="${ANTHROPIC_BASE_URL:-}"
tg_poll="${TOKENGATE_STATUSLINE_POLL:-5}"
tg_blocks="${TOKENGATE_STATUSLINE_BARS:-8}"

if [ -n "$tg_key" ] && [ -n "$tg_base" ]; then
    cache_file="/tmp/claude/tokengate-statusline-cache.json"
    mkdir -p /tmp/claude

    needs_refresh=true
    tg_data=""

    # Check cache
    if [ -f "$cache_file" ]; then
        cache_mtime=$(stat -c %Y "$cache_file" 2>/dev/null || stat -f %m "$cache_file" 2>/dev/null)
        now_epoch=$(date +%s)
        cache_age=$(( now_epoch - cache_mtime ))
        if [ "$cache_age" -lt "$tg_poll" ]; then
            needs_refresh=false
            tg_data=$(cat "$cache_file" 2>/dev/null)
        fi
    fi

    # Fetch from TokenGate gateway
    if $needs_refresh; then
        response=$(curl -s --max-time 2 \
            -H "X-TokenGate-Key: ${tg_key}" \
            -H "Accept: application/json" \
            "${tg_base}/v1/statusline" 2>/dev/null)

        if [ -n "$response" ] && echo "$response" | jq -e '.ok' >/dev/null 2>&1; then
            tg_data="$response"
            echo "$response" > "$cache_file"
        fi

        # Fall back to stale cache
        if [ -z "$tg_data" ] && [ -f "$cache_file" ]; then
            tg_data=$(cat "$cache_file" 2>/dev/null)
        fi
    fi

    # Render TokenGate data
    if [ -n "$tg_data" ] && echo "$tg_data" | jq -e '.ok' >/dev/null 2>&1; then
        billing_mode=$(echo "$tg_data" | jq -r '.billing_mode // ""')

        if [ "$billing_mode" = "API_USAGE" ]; then
            # ── API Usage mode: show cost + budget bars ──────────────────
            cost_today=$(echo "$tg_data" | jq -r '.cost.today // "0.0000"')
            cost_display=$(echo "$cost_today" | awk '{printf "$%.2f", $1}')
            out+="${sep}${magenta}${cost_display} today${reset}"

            # Monthly budget
            monthly=$(echo "$tg_data" | jq -r '.budgets.monthly // empty')
            if [ -n "$monthly" ] && [ "$monthly" != "null" ]; then
                m_pct=$(echo "$monthly" | jq -r '.percent // 0' | awk '{printf "%.0f", $1}')
                m_used=$(echo "$monthly" | jq -r '.used // "0"' | awk '{printf "$%.0f", $1}')
                m_limit=$(echo "$monthly" | jq -r '.limit // "0"' | awk '{printf "$%.0f", $1}')
                m_color=$(usage_color "$m_pct")
                m_bar=$(progress_bar "$m_pct" "$tg_blocks")
                out+="${sep}${cyan}Month${reset} ${orange}${m_used}/${m_limit}${reset} ${m_color}${m_bar}${reset} ${m_color}${m_pct}%${reset}"
            fi

            # Daily budget
            daily=$(echo "$tg_data" | jq -r '.budgets.daily // empty')
            if [ -n "$daily" ] && [ "$daily" != "null" ]; then
                d_pct=$(echo "$daily" | jq -r '.percent // 0' | awk '{printf "%.0f", $1}')
                d_used=$(echo "$daily" | jq -r '.used // "0"' | awk '{printf "$%.0f", $1}')
                d_limit=$(echo "$daily" | jq -r '.limit // "0"' | awk '{printf "$%.0f", $1}')
                d_color=$(usage_color "$d_pct")
                d_bar=$(progress_bar "$d_pct" "$tg_blocks")
                out+="${sep}${cyan}Day${reset} ${orange}${d_used}/${d_limit}${reset} ${d_color}${d_bar}${reset} ${d_color}${d_pct}%${reset}"
            fi

            # Weekly budget (if present)
            weekly=$(echo "$tg_data" | jq -r '.budgets.weekly // empty')
            if [ -n "$weekly" ] && [ "$weekly" != "null" ]; then
                w_pct=$(echo "$weekly" | jq -r '.percent // 0' | awk '{printf "%.0f", $1}')
                w_used=$(echo "$weekly" | jq -r '.used // "0"' | awk '{printf "$%.0f", $1}')
                w_limit=$(echo "$weekly" | jq -r '.limit // "0"' | awk '{printf "$%.0f", $1}')
                w_color=$(usage_color "$w_pct")
                w_bar=$(progress_bar "$w_pct" "$tg_blocks")
                out+="${sep}${cyan}Week${reset} ${orange}${w_used}/${w_limit}${reset} ${w_color}${w_bar}${reset} ${w_color}${w_pct}%${reset}"
            fi
        else
            # ── Monthly Subscription mode: show context window usage ─────
            ctx_color=$(usage_color "$ctx_pct")
            ctx_bar=$(progress_bar "$ctx_pct" "$tg_blocks" "$ctx_color")
            out+="${sep}${ctx_color}${ctx_bar}${reset} ${ctx_color}${ctx_pct}% used${reset} ${dim}${ctx_remain}% remain${reset}"
        fi
    elif [ -n "$tg_key" ]; then
        out+="${sep}${dim}TokenGate: unavailable${reset}"
    fi
fi

printf "%b" "$out"
exit 0
