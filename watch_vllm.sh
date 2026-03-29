#!/usr/bin/env bash
set -u

METRICS_URL="${1:-http://127.0.0.1:8000/metrics}"
INTERVAL="${INTERVAL:-5}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

get_metric_value() {
  local metrics="$1"
  local pattern="$2"
  printf '%s\n' "$metrics" | awk -v pat="$pattern" '$0 ~ pat {print $NF; exit}'
}

sum_metric_values() {
  local metrics="$1"
  local pattern="$2"
  printf '%s\n' "$metrics" | awk -v pat="$pattern" '$0 ~ pat {sum += $NF} END {print sum+0}'
}

safe_div() {
  awk -v a="${1:-0}" -v b="${2:-0}" 'BEGIN { if (b == 0) print 0; else printf "%.6f", a/b }'
}

fmt_num() {
  awk -v x="${1:-0}" 'BEGIN { printf "%.3f", x }'
}

echo "Watching vLLM SRE metrics from: $METRICS_URL"
echo "Refresh interval: ${INTERVAL}s"
echo

prev_prompt_total=""
prev_gen_total=""
prev_http_total=""
prev_http_4xx=""
prev_http_5xx=""
prev_stop=""
prev_length=""
prev_abort=""
prev_error=""
prev_ts=""

while true; do
  ts_human="$(date '+%F %T')"
  ts_epoch="$(date +%s)"

  metrics="$(curl -s --max-time 3 "$METRICS_URL")"
  if [ -z "$metrics" ]; then
    echo "[$ts_human] ERROR: cannot fetch metrics from $METRICS_URL"
    echo "------------------------------------------------------------"
    sleep "$INTERVAL"
    continue
  fi

  # Core state
  running="$(get_metric_value "$metrics" '^vllm:num_requests_running\{')"
  waiting="$(get_metric_value "$metrics" '^vllm:num_requests_waiting\{')"
  kv_usage="$(get_metric_value "$metrics" '^vllm:kv_cache_usage_perc\{')"
  preemptions="$(get_metric_value "$metrics" '^vllm:num_preemptions_total\{')"

  # Latency
  ttft_count="$(get_metric_value "$metrics" '^vllm:time_to_first_token_seconds_count\{')"
  ttft_sum="$(get_metric_value "$metrics" '^vllm:time_to_first_token_seconds_sum\{')"
  e2e_count="$(get_metric_value "$metrics" '^vllm:e2e_request_latency_seconds_count\{')"
  e2e_sum="$(get_metric_value "$metrics" '^vllm:e2e_request_latency_seconds_sum\{')"

  avg_ttft="$(safe_div "${ttft_sum:-0}" "${ttft_count:-0}")"
  avg_e2e="$(safe_div "${e2e_sum:-0}" "${e2e_count:-0}")"

  # Tokens
  prompt_total="$(get_metric_value "$metrics" '^vllm:prompt_tokens_total\{')"
  gen_total="$(get_metric_value "$metrics" '^vllm:generation_tokens_total\{')"

  # Request outcomes
  success_stop="$(get_metric_value "$metrics" '^vllm:request_success_total\{.*finished_reason="stop"')"
  success_length="$(get_metric_value "$metrics" '^vllm:request_success_total\{.*finished_reason="length"')"
  success_abort="$(get_metric_value "$metrics" '^vllm:request_success_total\{.*finished_reason="abort"')"
  success_error="$(get_metric_value "$metrics" '^vllm:request_success_total\{.*finished_reason="error"')"

  total_finished="$(awk -v a="${success_stop:-0}" -v b="${success_length:-0}" -v c="${success_abort:-0}" -v d="${success_error:-0}" 'BEGIN { printf "%.0f", a+b+c+d }')"
  length_rate="$(safe_div "${success_length:-0}" "$total_finished")"
  length_rate_pct="$(awk -v x="$length_rate" 'BEGIN { printf "%.2f%%", x*100 }')"

  # Prefix cache
  prefix_q="$(get_metric_value "$metrics" '^vllm:prefix_cache_queries_total\{')"
  prefix_h="$(get_metric_value "$metrics" '^vllm:prefix_cache_hits_total\{')"
  prefix_hit_rate="$(safe_div "${prefix_h:-0}" "${prefix_q:-0}")"
  prefix_hit_rate_pct="$(awk -v x="$prefix_hit_rate" 'BEGIN { printf "%.2f%%", x*100 }')"

  # HTTP totals
  http_total="$(sum_metric_values "$metrics" '^http_requests_total\{')"
  http_4xx="$(sum_metric_values "$metrics" '^http_requests_total\{.*status="4xx"')"
  http_5xx="$(sum_metric_values "$metrics" '^http_requests_total\{.*status="5xx"')"

  # Deltas / rates
  elapsed="$INTERVAL"
  if [ -n "${prev_ts:-}" ]; then
    elapsed=$((ts_epoch - prev_ts))
    if [ "$elapsed" -le 0 ]; then
      elapsed="$INTERVAL"
    fi
  fi

  if [ -n "${prev_prompt_total:-}" ]; then
    prompt_tps="$(awk -v cur="${prompt_total:-0}" -v prev="${prev_prompt_total:-0}" -v s="$elapsed" 'BEGIN { printf "%.3f", (cur-prev)/s }')"
    gen_tps="$(awk -v cur="${gen_total:-0}" -v prev="${prev_gen_total:-0}" -v s="$elapsed" 'BEGIN { printf "%.3f", (cur-prev)/s }')"
    rps="$(awk -v cur="${http_total:-0}" -v prev="${prev_http_total:-0}" -v s="$elapsed" 'BEGIN { printf "%.3f", (cur-prev)/s }')"

    d_4xx="$(awk -v cur="${http_4xx:-0}" -v prev="${prev_http_4xx:-0}" 'BEGIN { printf "%.0f", cur-prev }')"
    d_5xx="$(awk -v cur="${http_5xx:-0}" -v prev="${prev_http_5xx:-0}" 'BEGIN { printf "%.0f", cur-prev }')"

    d_stop="$(awk -v cur="${success_stop:-0}" -v prev="${prev_stop:-0}" 'BEGIN { printf "%.0f", cur-prev }')"
    d_length="$(awk -v cur="${success_length:-0}" -v prev="${prev_length:-0}" 'BEGIN { printf "%.0f", cur-prev }')"
    d_abort="$(awk -v cur="${success_abort:-0}" -v prev="${prev_abort:-0}" 'BEGIN { printf "%.0f", cur-prev }')"
    d_error="$(awk -v cur="${success_error:-0}" -v prev="${prev_error:-0}" 'BEGIN { printf "%.0f", cur-prev }')"

    interval_total="$(awk -v a="$d_stop" -v b="$d_length" -v c="$d_abort" -v d="$d_error" -v e="$d_4xx" -v f="$d_5xx" 'BEGIN { printf "%.0f", a+b+c+d+e+f }')"
    err_rate="$(safe_div "$(awk -v a="$d_error" -v b="$d_5xx" 'BEGIN { print a+b }')" "$interval_total")"
    err_rate_pct="$(awk -v x="$err_rate" 'BEGIN { printf "%.2f%%", x*100 }')"
  else
    prompt_tps="0.000"
    gen_tps="0.000"
    rps="0.000"
    d_4xx="0"
    d_5xx="0"
    d_stop="0"
    d_length="0"
    d_abort="0"
    d_error="0"
    err_rate_pct="0.00%"
  fi

  # Health status
  status="OK"
  color="$GREEN"
  reasons=""

  if awk "BEGIN {exit !(${waiting:-0} > 0)}"; then
    status="WARN"
    color="$YELLOW"
    reasons="${reasons} queue"
  fi

  if awk "BEGIN {exit !(${avg_ttft:-0} > 1.5)}"; then
    status="WARN"
    color="$YELLOW"
    reasons="${reasons} slow_ttft"
  fi

  if awk "BEGIN {exit !(${avg_e2e:-0} > 6.0)}"; then
    status="WARN"
    color="$YELLOW"
    reasons="${reasons} slow_e2e"
  fi

  if awk "BEGIN {exit !(${kv_usage:-0} > 0.85)}"; then
    status="CRIT"
    color="$RED"
    reasons="${reasons} kv_pressure"
  fi

  if awk "BEGIN {exit !(${preemptions:-0} > 0)}"; then
    status="CRIT"
    color="$RED"
    reasons="${reasons} preemption"
  fi

  if [ "${d_5xx:-0}" -gt 0 ] || [ "${d_error:-0}" -gt 0 ]; then
    status="CRIT"
    color="$RED"
    reasons="${reasons} errors"
  fi

  if [ "${d_4xx:-0}" -gt 0 ] && [ "$status" = "OK" ]; then
    status="WARN"
    color="$YELLOW"
    reasons="${reasons} client_4xx"
  fi

  clear
  echo "------------------------------------------------------------"
  echo "[$ts_human]"
  echo -e "status=${color}${status}${NC}${reasons}"
  echo
  echo -e "${CYAN}Traffic${NC}"
  echo "rps=$rps prompt_tps=$prompt_tps gen_tps=$gen_tps"
  echo
  echo -e "${CYAN}Latency${NC}"
  echo "avg_ttft=$(fmt_num "$avg_ttft")s avg_e2e=$(fmt_num "$avg_e2e")s"
  echo
  echo -e "${CYAN}Saturation${NC}"
  echo "running=$running waiting=$waiting kv_usage=$kv_usage preemptions=$preemptions"
  echo
  echo -e "${CYAN}Errors / Outcomes${NC}"
  echo "http_4xx_delta=$d_4xx http_5xx_delta=$d_5xx error_rate=$err_rate_pct"
  echo "finished_delta(stop/length/abort/error)=$d_stop/$d_length/$d_abort/$d_error"
  echo "finished_total(stop/length/abort/error)=$success_stop/$success_length/$success_abort/$success_error"
  echo "length_stop_rate=$length_rate_pct"
  echo
  echo -e "${CYAN}Efficiency${NC}"
  echo "prefix_cache_hit_rate=$prefix_hit_rate_pct ($prefix_h/$prefix_q)"
  echo "prompt_total=$prompt_total generation_total=$gen_total"
  echo "http_total=$http_total"
  echo "------------------------------------------------------------"

  prev_prompt_total="${prompt_total:-0}"
  prev_gen_total="${gen_total:-0}"
  prev_http_total="${http_total:-0}"
  prev_http_4xx="${http_4xx:-0}"
  prev_http_5xx="${http_5xx:-0}"
  prev_stop="${success_stop:-0}"
  prev_length="${success_length:-0}"
  prev_abort="${success_abort:-0}"
  prev_error="${success_error:-0}"
  prev_ts="$ts_epoch"

  sleep "$INTERVAL"
done
