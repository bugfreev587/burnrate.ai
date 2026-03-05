#!/bin/bash
# TokenGate Status Line for Claude Code
#
# Displays real-time budget/cost/usage data from TokenGate gateway alongside
# the standard Claude Code session info.
#
# For API_USAGE mode (BYOK): queries the TokenGate gateway for cost/budget data.
# For MONTHLY_SUBSCRIPTION mode: queries Anthropic OAuth usage API directly for
# 5h/7d rate limit windows — no TokenGate API call needed.
#
# Setup:
#   1. Copy this script to ~/.claude/statusline-command.sh
#   2. chmod +x ~/.claude/statusline-command.sh
#   3. Configure in ~/.claude/settings.json:
#      { "statusLine": { "type": "command", "command": "sh ~/.claude/statusline-command.sh" } }
#
# Required environment variables (API_USAGE mode only):
#   ANTHROPIC_BASE_URL  - TokenGate gateway URL (e.g. https://gateway.tokengate.to)
#   ANTHROPIC_API_KEY   - TokenGate API key (tg_xxx) or set via TOKENGATE_API_KEY
#
# Optional environment variables:
#   TOKENGATE_API_KEY          - Explicit TokenGate key (overrides ANTHROPIC_API_KEY)
#   TOKENGATE_STATUSLINE_POLL  - Cache TTL in seconds (default: 60)
#   TOKENGATE_STATUSLINE_BARS  - Progress bar block count (default: 6)
#   TOKENGATE_BILLING_MODE     - Force billing mode: MONTHLY_SUBSCRIPTION | API_USAGE
#   CLAUDE_CODE_OAUTH_TOKEN    - Explicit OAuth token (skips Keychain lookup)

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

sep=" ${dim}|${reset} "

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
    if [ "$pct" -ge 90 ]; then echo "$red"
    elif [ "$pct" -ge 70 ]; then echo "$orange"
    elif [ "$pct" -ge 50 ]; then echo "$yellow"
    else echo "$green"
    fi
}

# Build a dot progress bar: ●●●○○○
# Usage: dot_bar <percent> <num_blocks> <color>
dot_bar() {
    local pct=$1
    local blocks=${2:-6}
    local color=${3:-$green}
    local filled=$(( (pct * blocks + 99) / 100 ))  # round up so >0% shows at least 1
    [ "$pct" -eq 0 ] && filled=0
    [ "$filled" -gt "$blocks" ] && filled=$blocks
    [ "$filled" -lt 0 ] && filled=0
    local empty=$(( blocks - filled ))

    local bar=""
    for (( i=0; i<filled; i++ )); do bar+="●"; done
    for (( i=0; i<empty; i++ )); do bar+="○"; done
    echo "$bar"
}

# Build a bracket progress bar: [■■■■□□□□]
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

# Cross-platform ISO to epoch conversion
iso_to_epoch() {
    local iso_str="$1"
    local epoch
    # GNU date (Linux)
    epoch=$(date -d "${iso_str}" +%s 2>/dev/null)
    if [ -n "$epoch" ]; then echo "$epoch"; return 0; fi
    # BSD date (macOS)
    local stripped="${iso_str%%.*}"
    stripped="${stripped%%Z}"
    stripped="${stripped%%+*}"
    stripped="${stripped%%-[0-9][0-9]:[0-9][0-9]}"
    if [[ "$iso_str" == *"Z"* ]] || [[ "$iso_str" == *"+00:00"* ]] || [[ "$iso_str" == *"-00:00"* ]]; then
        epoch=$(env TZ=UTC date -j -f "%Y-%m-%dT%H:%M:%S" "$stripped" +%s 2>/dev/null)
    else
        epoch=$(date -j -f "%Y-%m-%dT%H:%M:%S" "$stripped" +%s 2>/dev/null)
    fi
    if [ -n "$epoch" ]; then echo "$epoch"; return 0; fi
    return 1
}

# Format ISO reset time to compact local time
format_reset_time() {
    local iso_str="$1"
    local style="$2"
    [ -z "$iso_str" ] || [ "$iso_str" = "null" ] && return
    local epoch
    epoch=$(iso_to_epoch "$iso_str")
    [ -z "$epoch" ] && return
    case "$style" in
        time)
            date -j -r "$epoch" +"%l:%M%p" 2>/dev/null | sed 's/^ //' | tr '[:upper:]' '[:lower:]' || \
            date -d "@$epoch" +"%l:%M%P" 2>/dev/null | sed 's/^ //'
            ;;
        datetime)
            date -j -r "$epoch" +"%b %-d, %l:%M%p" 2>/dev/null | sed 's/  / /g; s/^ //' | tr '[:upper:]' '[:lower:]' || \
            date -d "@$epoch" +"%b %-d, %l:%M%P" 2>/dev/null | sed 's/  / /g; s/^ //'
            ;;
        *)
            date -j -r "$epoch" +"%b %-d" 2>/dev/null | tr '[:upper:]' '[:lower:]' || \
            date -d "@$epoch" +"%b %-d" 2>/dev/null
            ;;
    esac
}

# Resolve OAuth token: env var → macOS Keychain → Linux creds → GNOME Keyring
get_oauth_token() {
    if [ -n "$CLAUDE_CODE_OAUTH_TOKEN" ]; then
        echo "$CLAUDE_CODE_OAUTH_TOKEN"; return 0
    fi
    # macOS Keychain
    if command -v security >/dev/null 2>&1; then
        local blob
        blob=$(security find-generic-password -s "Claude Code-credentials" -w 2>/dev/null)
        if [ -n "$blob" ]; then
            local token
            token=$(echo "$blob" | jq -r '.claudeAiOauth.accessToken // empty' 2>/dev/null)
            if [ -n "$token" ] && [ "$token" != "null" ]; then echo "$token"; return 0; fi
        fi
    fi
    # Linux credentials file
    local creds_file="${HOME}/.claude/.credentials.json"
    if [ -f "$creds_file" ]; then
        local token
        token=$(jq -r '.claudeAiOauth.accessToken // empty' "$creds_file" 2>/dev/null)
        if [ -n "$token" ] && [ "$token" != "null" ]; then echo "$token"; return 0; fi
    fi
    # GNOME Keyring
    if command -v secret-tool >/dev/null 2>&1; then
        local blob
        blob=$(timeout 2 secret-tool lookup service "Claude Code-credentials" 2>/dev/null)
        if [ -n "$blob" ]; then
            local token
            token=$(echo "$blob" | jq -r '.claudeAiOauth.accessToken // empty' 2>/dev/null)
            if [ -n "$token" ] && [ "$token" != "null" ]; then echo "$token"; return 0; fi
        fi
    fi
    echo ""
}

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

ctx_pct=0
ctx_remain=100
if [ "$size" -gt 0 ] 2>/dev/null; then
    ctx_pct=$(( current * 100 / size ))
    ctx_remain=$(( 100 - ctx_pct ))
fi

session_cost=$(echo "$input" | jq -r '.cost // 0' | LC_NUMERIC=C awk '{printf "%.4f", $1}')
session_cost_nonzero=false
if LC_NUMERIC=C awk "BEGIN {exit !($session_cost > 0.00005)}"; then
    session_cost_nonzero=true
fi

cwd=$(echo "$input" | jq -r '.cwd // empty')
display_dir=""
if [ -n "$cwd" ]; then
    display_dir="${cwd##*/}"
    # Append git branch if inside a git repo
    git_branch=$(git -C "$cwd" rev-parse --abbrev-ref HEAD 2>/dev/null)
    if [ -n "$git_branch" ]; then
        display_dir="${display_dir}@${git_branch}"
    fi
fi

# ── Detect billing mode ─────────────────────────────────────────────────────
tg_key="${TOKENGATE_API_KEY:-$ANTHROPIC_API_KEY}"
# If key is still empty, extract X-TokenGate-Key value from ANTHROPIC_CUSTOM_HEADERS.
# Supports format: "X-TokenGate-Key:tg_xxx..." or "Header1:v1,X-TokenGate-Key:tg_xxx..."
if [ -z "$tg_key" ] && [ -n "$ANTHROPIC_CUSTOM_HEADERS" ]; then
    tg_key=$(printf '%s' "$ANTHROPIC_CUSTOM_HEADERS" | grep -oE 'X-TokenGate-Key:[^,[:space:]]+' | head -1 | cut -d: -f2-)
fi
tg_base="${ANTHROPIC_BASE_URL:-}"
tg_poll="${TOKENGATE_STATUSLINE_POLL:-60}"
tg_blocks="${TOKENGATE_STATUSLINE_BARS:-6}"
billing_mode="${TOKENGATE_BILLING_MODE:-}"

# Auto-detect billing mode:
#   - Has ANTHROPIC_BASE_URL + key → TokenGate proxy (AUTO)
#   - Has ANTHROPIC_API_KEY that looks like a direct Anthropic key (sk-*) → API_USAGE_DIRECT
#   - No API key, OAuth login only → MONTHLY_SUBSCRIPTION
if [ -z "$billing_mode" ]; then
    if [ -n "$tg_base" ] && [ -n "$tg_key" ]; then
        billing_mode="AUTO"
    elif [ -n "$ANTHROPIC_API_KEY" ] && [[ "$ANTHROPIC_API_KEY" != tg_* ]]; then
        # Direct Anthropic API key (sk-ant-...) — API usage billing, not monthly subscription
        billing_mode="API_USAGE_DIRECT"
    else
        billing_mode="MONTHLY_SUBSCRIPTION"
    fi
fi

# ── Measure visible width (strip ANSI escapes) ──────────────────────────────
visible_len() {
    printf "%b" "$1" | sed $'s/\033\[[0-9;]*m//g' | wc -m | tr -d ' '
}

# Terminal width: status line scripts don't have a real TTY, so tput returns 80.
# Use COLUMNS env var if set, otherwise default to 200 (assume wide terminal).
# Users can override via: export COLUMNS=120 in their shell profile.
cols=${COLUMNS:-200}

# ── Precompute context color ────────────────────────────────────────────────
ctx_color=$(usage_color "$ctx_pct")

# ── Fetch usage data for MONTHLY_SUBSCRIPTION ────────────────────────────────
five_pct="" ; five_reset="" ; five_color="" ; five_bar=""
seven_pct="" ; seven_reset="" ; seven_color="" ; seven_bar=""
extra_part=""

fetch_oauth_usage() {
    local cache_file="/tmp/claude/statusline-usage-cache.json"
    local cache_max_age="${tg_poll}"
    mkdir -p /tmp/claude

    local needs_refresh=true
    usage_data=""

    if [ -f "$cache_file" ]; then
        local cache_mtime=$(stat -c %Y "$cache_file" 2>/dev/null || stat -f %m "$cache_file" 2>/dev/null)
        local now_epoch=$(date +%s)
        local cache_age=$(( now_epoch - cache_mtime ))
        if [ "$cache_age" -lt "$cache_max_age" ]; then
            needs_refresh=false
            usage_data=$(cat "$cache_file" 2>/dev/null)
        fi
    fi

    if $needs_refresh; then
        local token
        token=$(get_oauth_token)
        if [ -n "$token" ] && [ "$token" != "null" ]; then
            local cc_version
            cc_version=$(echo "$input" | jq -r '.version // "2.1.0"')
            local response
            response=$(curl -s --max-time 5 \
                -H "Accept: application/json" \
                -H "Content-Type: application/json" \
                -H "Authorization: Bearer $token" \
                -H "anthropic-beta: oauth-2025-04-20" \
                -H "User-Agent: claude-code/${cc_version}" \
                "https://api.anthropic.com/api/oauth/usage" 2>/dev/null)
            if [ -n "$response" ] && echo "$response" | jq -e '.five_hour' >/dev/null 2>&1; then
                usage_data="$response"
                echo "$response" > "$cache_file"
            fi
        fi
        if [ -z "$usage_data" ] && [ -f "$cache_file" ]; then
            local _stale
            _stale=$(cat "$cache_file" 2>/dev/null)
            if echo "$_stale" | jq -e '.five_hour' >/dev/null 2>&1; then
                usage_data="$_stale"
            fi
        fi
    fi

    if [ -n "$usage_data" ] && echo "$usage_data" | jq -e '.five_hour' >/dev/null 2>&1; then
        five_pct=$(echo "$usage_data" | jq -r '.five_hour.utilization // 0' | awk '{printf "%.0f", $1}')
        five_reset_iso=$(echo "$usage_data" | jq -r '.five_hour.resets_at // empty')
        five_reset=$(format_reset_time "$five_reset_iso" "time")
        five_color=$(usage_color "$five_pct")
        five_bar=$(dot_bar "$five_pct" "$tg_blocks" "$five_color")

        seven_pct=$(echo "$usage_data" | jq -r '.seven_day.utilization // 0' | awk '{printf "%.0f", $1}')
        seven_reset_iso=$(echo "$usage_data" | jq -r '.seven_day.resets_at // empty')
        seven_reset=$(format_reset_time "$seven_reset_iso" "datetime")
        seven_color=$(usage_color "$seven_pct")
        seven_bar=$(dot_bar "$seven_pct" "$tg_blocks" "$seven_color")

        extra_enabled=$(echo "$usage_data" | jq -r '.extra_usage.is_enabled // false')
        if [ "$extra_enabled" = "true" ]; then
            extra_pct=$(echo "$usage_data" | jq -r '.extra_usage.utilization // 0' | awk '{printf "%.0f", $1}')
            extra_used=$(echo "$usage_data" | jq -r '.extra_usage.used_credits // 0' | LC_NUMERIC=C awk '{printf "%.2f", $1/100}')
            extra_limit=$(echo "$usage_data" | jq -r '.extra_usage.monthly_limit // 0' | LC_NUMERIC=C awk '{printf "%.2f", $1/100}')
            if [ -n "$extra_used" ] && [ -n "$extra_limit" ]; then
                extra_color=$(usage_color "$extra_pct")
                extra_part="${white}extra${reset} ${extra_color}\$${extra_used}/\$${extra_limit}${reset}"
            fi
        fi
    fi
}

if [ "$billing_mode" = "MONTHLY_SUBSCRIPTION" ]; then
    fetch_oauth_usage
fi

# ── Assemble output with adaptive density ────────────────────────────────────
# Strategy: try 3 levels — full → compact → minimal
#   Level 1 (full):    " | " spacious separators, dot bars
#   Level 2 (compact): "|" tight separators, no dot bars
#   Level 3 (minimal): "|" tight separators, no dot bars, no reset times

build_output() {
    local level=$1   # 1=full, 2=compact, 3=minimal
    local s          # separator

    if [ "$level" -le 1 ]; then
        s=" ${dim}|${reset} "
    else
        s="${dim}|${reset}"
    fi

    local o=""
    o+="${blue}${model_name}${reset}"
    [ -n "$display_dir" ] && o+="${s}${cyan}${display_dir}${reset}"
    o+="${s}${orange}${used_tokens}/${total_tokens}${reset}"
    o+="${s}${ctx_color}${ctx_pct}% used${reset}"
    o+="${s}${dim}${ctx_remain}% remain${reset}"

    # Monthly subscription rate windows
    if [ -n "$five_pct" ]; then
        if [ "$level" -le 2 ]; then
            # With dot bars
            o+="${s}${white}5h${reset} ${five_color}${five_bar}${reset} ${five_color}${five_pct}%${reset}"
        else
            # No dot bars
            o+="${s}${white}5h${reset} ${five_color}${five_pct}%${reset}"
        fi
        [ -n "$five_reset" ] && o+=" ${dim}@${five_reset}${reset}"
    fi

    if [ -n "$seven_pct" ]; then
        if [ "$level" -le 2 ]; then
            o+="${s}${white}7d${reset} ${seven_color}${seven_bar}${reset} ${seven_color}${seven_pct}%${reset}"
        else
            o+="${s}${white}7d${reset} ${seven_color}${seven_pct}%${reset}"
        fi
        [ -n "$seven_reset" ] && o+=" ${dim}@${seven_reset}${reset}"
    fi

    [ -n "$extra_part" ] && o+="${s}${extra_part}"

    # Session cost — shown when billing is per-token (cost > 0) and no
    # TokenGate gateway is providing its own cost breakdown.
    if $session_cost_nonzero && [ "${#tg_parts[@]}" -eq 0 ]; then
        cost_disp=$(LC_NUMERIC=C awk "BEGIN {printf \"\$%.4f\", $session_cost}")
        o+="${s}${magenta}${cost_disp} session${reset}"
    fi

    echo "$o"
}

# ═════════════════════════════════════════════════════════════════════════════
# API_USAGE / AUTO MODE — append TokenGate data to build_output
# ═════════════════════════════════════════════════════════════════════════════
tg_parts=()
if [ "$billing_mode" != "MONTHLY_SUBSCRIPTION" ] && [ -n "$tg_key" ] && [ -n "$tg_base" ]; then
    cache_file="/tmp/claude/tokengate-statusline-cache.json"
    mkdir -p /tmp/claude

    needs_refresh=true
    tg_data=""

    if [ -f "$cache_file" ]; then
        cache_mtime=$(stat -c %Y "$cache_file" 2>/dev/null || stat -f %m "$cache_file" 2>/dev/null)
        now_epoch=$(date +%s)
        cache_age=$(( now_epoch - cache_mtime ))
        if [ "$cache_age" -lt "$tg_poll" ]; then
            needs_refresh=false
            tg_data=$(cat "$cache_file" 2>/dev/null)
        fi
    fi

    if $needs_refresh; then
        response=$(curl -s --max-time 2 \
            -H "X-TokenGate-Key: ${tg_key}" \
            -H "Accept: application/json" \
            "${tg_base}/v1/statusline" 2>/dev/null)
        if [ -n "$response" ] && echo "$response" | jq -e '.ok' >/dev/null 2>&1; then
            tg_data="$response"
            echo "$response" > "$cache_file"
        fi
        if [ -z "$tg_data" ] && [ -f "$cache_file" ]; then
            tg_data=$(cat "$cache_file" 2>/dev/null)
        fi
    fi

    if [ -n "$tg_data" ] && echo "$tg_data" | jq -e '.ok' >/dev/null 2>&1; then
        tg_billing=$(echo "$tg_data" | jq -r '.billing_mode // ""')

        if [ "$tg_billing" = "API_USAGE" ]; then
            cost_today=$(echo "$tg_data" | jq -r '.cost.today // "0.0000"')
            cost_display=$(echo "$cost_today" | awk '{printf "$%.2f", $1}')
            tg_parts+=("${magenta}${cost_display} today${reset}")

            for period in monthly daily weekly; do
                budget=$(echo "$tg_data" | jq -r ".budgets.${period} // empty")
                if [ -n "$budget" ] && [ "$budget" != "null" ]; then
                    b_pct=$(echo "$budget" | jq -r '.percent // 0' | awk '{printf "%.0f", $1}')
                    b_used=$(echo "$budget" | jq -r '.used // "0"' | awk '{printf "$%.0f", $1}')
                    b_limit=$(echo "$budget" | jq -r '.limit // "0"' | awk '{printf "$%.0f", $1}')
                    b_color=$(usage_color "$b_pct")
                    b_bar=$(dot_bar "$b_pct" "$tg_blocks" "$b_color")
                    label=$(echo "$period" | sed 's/monthly/month/;s/daily/day/;s/weekly/week/')
                    tg_parts+=("${white}${label}${reset} ${b_color}${b_bar}${reset} ${orange}${b_used}/${b_limit}${reset} ${b_color}${b_pct}%${reset}")
                fi
            done
        elif [ "$tg_billing" = "MONTHLY_SUBSCRIPTION" ]; then
            # TokenGate proxy with monthly subscription — fetch OAuth rate limits only
            fetch_oauth_usage
        fi
    elif [ -n "$tg_key" ]; then
        tg_parts+=("${dim}TokenGate: unavailable${reset}")
    fi
fi

# ── Build with adaptive density and append TokenGate parts ───────────────────

assemble() {
    local level=$1
    local base
    base=$(build_output "$level")
    if [ "${#tg_parts[@]}" -gt 0 ]; then
        local s
        if [ "$level" -le 1 ]; then s=" ${dim}|${reset} "; else s="${dim}|${reset}"; fi
        for part in "${tg_parts[@]}"; do
            base+="${s}${part}"
        done
    fi
    echo "$base"
}

# Try each density level until it fits
out=$(assemble 1)
if [ "$(visible_len "$out")" -gt "$cols" ]; then
    out=$(assemble 2)
fi
if [ "$(visible_len "$out")" -gt "$cols" ]; then
    out=$(assemble 3)
fi

printf "%b" "$out"
exit 0
