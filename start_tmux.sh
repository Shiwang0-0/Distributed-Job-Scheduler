#!/usr/bin/env bash
set -e

SESSION="distributed-scheduler"

# Function to check if we're inside tmux
if [ -n "$TMUX" ]; then
  echo "ERROR: You are already inside a tmux session."
  echo "Please run this script from outside tmux, or detach first (Ctrl+b, d)"
  exit 1
fi

# Health check function
wait_for_service() {
  local host=$1
  local port=$2
  local service_name=$3
  local max_attempts=30
  local attempt=0
  
  echo "Waiting for $service_name ($host:$port) to be ready..."
  while ! nc -z "$host" "$port" 2>/dev/null; do
    attempt=$((attempt + 1))
    if [ $attempt -ge $max_attempts ]; then
      echo "ERROR: $service_name failed to start after $max_attempts seconds"
      return 1
    fi
    sleep 1
  done
  echo "$service_name is ready!"
}

# Kill existing session if present
tmux has-session -t "$SESSION" 2>/dev/null && tmux kill-session -t "$SESSION"

# Create main session
tmux new-session -d -s "$SESSION" -n schedulers

# Define all ports
SCHEDULER_PORTS=(8081 8082 8083)
NORMAL_QUEUE_ARR=(7001 7002 7003)
HIGH_QUEUE_ARR=(7101 7102)
EXCHANGE_PORTS=(2001 2002)
WATCHER_PORTS=(9091 9092 9093)
WORKER_PORTS=(10001 10002 10003 10004 10005 10006)

NORMAL_QUEUE_PORTS=$(IFS=,; echo "${NORMAL_QUEUE_ARR[*]}")
HIGH_QUEUE_PORTS=$(IFS=,; echo "${HIGH_QUEUE_ARR[*]}")
EXCHANGE_PORTS_STR=$(IFS=,; echo "${EXCHANGE_PORTS[*]}")
SCHEDULER_PORTS_STR=$(IFS=,; echo "${SCHEDULER_PORTS[*]}")

####################################################
# Window 1: Schedulers (3 separate processes)
####################################################
echo "Starting Schedulers..."
for i in "${!SCHEDULER_PORTS[@]}"; do
  if (( i > 0 )); then
    tmux split-window -h -t "$SESSION:schedulers"
  fi
  tmux send-keys -t "$SESSION:schedulers.$i" "go run cmd/scheduler/main.go --port=${SCHEDULER_PORTS[$i]}" C-m
done
tmux select-layout -t "$SESSION:schedulers" tiled

# Wait for at least one scheduler to be ready
wait_for_service "localhost" "${SCHEDULER_PORTS[0]}" "Scheduler-1"

####################################################
# Window 2: Coordinator (1 separate process)
####################################################
echo "Starting Coordinator..."
tmux new-window -t "$SESSION" -n coordinator
tmux send-keys -t "$SESSION:coordinator" "go run cmd/coordinator/main.go --port=3000 --normal=${NORMAL_QUEUE_PORTS} --high=${HIGH_QUEUE_PORTS}" C-m

# Wait for coordinator to be ready (CRITICAL for workers)
wait_for_service "localhost" "3000" "Coordinator"

####################################################
# Window 3: Normal Queues (3 separate processes)
####################################################
echo "Starting Normal Priority Queues..."
tmux new-window -t "$SESSION" -n normal-queues
for i in "${!NORMAL_QUEUE_ARR[@]}"; do
  if (( i > 0 )); then
    tmux split-window -h -t "$SESSION:normal-queues"
  fi
  tmux send-keys -t "$SESSION:normal-queues.$i" "go run cmd/queue_service/main.go --port=${NORMAL_QUEUE_ARR[$i]} --type=normal --index=$i" C-m
done
tmux select-layout -t "$SESSION:normal-queues" tiled

# Wait for at least one queue to be ready
wait_for_service "localhost" "${NORMAL_QUEUE_ARR[0]}" "Normal-Queue-1"

####################################################
# Window 4: High Priority Queues (2 separate processes)
####################################################
echo "Starting High Priority Queues..."
tmux new-window -t "$SESSION" -n high-queues
for i in "${!HIGH_QUEUE_ARR[@]}"; do
  if (( i > 0 )); then
    tmux split-window -h -t "$SESSION:high-queues"
  fi
  tmux send-keys -t "$SESSION:high-queues.$i" "go run cmd/queue_service/main.go --port=${HIGH_QUEUE_ARR[$i]} --type=high --index=$i" C-m
done
tmux select-layout -t "$SESSION:high-queues" tiled

# Wait for at least one high priority queue
wait_for_service "localhost" "${HIGH_QUEUE_ARR[0]}" "High-Queue-1"

####################################################
# Window 5: Exchanges (2 separate processes)
####################################################
echo "Starting Exchanges..."
tmux new-window -t "$SESSION" -n exchanges
for i in "${!EXCHANGE_PORTS[@]}"; do
  if (( i > 0 )); then
    tmux split-window -h -t "$SESSION:exchanges"
  fi
  tmux send-keys -t "$SESSION:exchanges.$i" "go run cmd/exchange/main.go --port=${EXCHANGE_PORTS[$i]} --normal=${NORMAL_QUEUE_PORTS} --high=${HIGH_QUEUE_PORTS}" C-m
done
tmux select-layout -t "$SESSION:exchanges" tiled

# Wait for at least one exchange
wait_for_service "localhost" "${EXCHANGE_PORTS[0]}" "Exchange-1"

####################################################
# Window 6: Watchers (3 separate processes)
####################################################
echo "Starting Watchers..."
tmux new-window -t "$SESSION" -n watchers
for i in "${!WATCHER_PORTS[@]}"; do
  if (( i > 0 )); then
    tmux split-window -h -t "$SESSION:watchers"
  fi
  tmux send-keys -t "$SESSION:watchers.$i" "go run cmd/watcher/main.go --port=${WATCHER_PORTS[$i]} --exchanges=${EXCHANGE_PORTS_STR}" C-m
done
tmux select-layout -t "$SESSION:watchers" tiled

# Give watchers a moment to start
sleep 2

####################################################
# Window 7: Workers (6 separate processes)
####################################################
echo "Starting Workers..."
tmux new-window -t "$SESSION" -n workers
WORKER_CONCURRENCY=5
COORDINATOR_PORT=3000

for i in "${!WORKER_PORTS[@]}"; do
  if (( i > 0 )); then
    tmux split-window -h -t "$SESSION:workers"
  fi
  tmux send-keys -t "$SESSION:workers.$i" "go run cmd/worker/main.go --port=${WORKER_PORTS[$i]} --coordinator=${COORDINATOR_PORT} --concurrency=${WORKER_CONCURRENCY}" C-m
done
tmux select-layout -t "$SESSION:workers" tiled

# Give workers time to register
sleep 2

####################################################
# Window 8: Gateway (1 separate process)
####################################################
echo "Starting Gateway..."
tmux new-window -t "$SESSION" -n gateway
tmux send-keys -t "$SESSION:gateway" "go run cmd/gateway/main.go --port=8080 --schedulers=${SCHEDULER_PORTS_STR}" C-m

# Wait for gateway
wait_for_service "localhost" "8080" "Gateway"

####################################################
# Attach to session
####################################################
echo ""
echo "All services started successfully!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "System Overview:"
echo "   • Schedulers: 3 (ports 8081-8083)"
echo "   • Coordinator: 1 (port 3000)"
echo "   • Normal Queues: 3 (ports 7001-7003)"
echo "   • High Priority Queues: 2 (ports 7101-7102)"
echo "   • Exchanges: 2 (ports 2001-2002)"
echo "   • Watchers: 3 (ports 9091-9093)"
echo "   • Workers: 6 (ports 10001-10006)"
echo "   • Gateway: 1 (port 8080)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Gateway API: http://localhost:8080"
echo "Coordinator: http://localhost:3000"
echo ""
echo "Press Ctrl+b, then number to switch windows:"
echo "   0: Schedulers  1: Coordinator  2: Normal Queues  3: High Queues"
echo "   4: Exchanges   5: Watchers     6: Workers        7: Gateway"
echo ""
tmux select-window -t "$SESSION:schedulers"
tmux attach-session -t "$SESSION"