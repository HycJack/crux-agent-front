#!/bin/bash
set -e

# Hermes Chat - Start both backend and frontend

DIR="$(cd "$(dirname "$0")" && pwd)"

# Load env if exists
[ -f "$DIR/.env" ] && export $(grep -v '^#' "$DIR/.env" | xargs)

echo "=== Starting Hermes Chat ==="
echo "Backend: http://localhost:${PORT:-8080}"
echo "Frontend: http://localhost:3000"
echo ""

# Build & start backend
echo "[1/2] Starting backend..."
cd "$DIR/backend"
go build -o ../hermes-chat-backend . 2>&1 | tail -1
cd "$DIR"
./hermes-chat-backend &
BACKEND_PID=$!

# Start frontend
echo "[2/2] Starting frontend..."
cd "$DIR/frontend"
npx vite --host &
FRONTEND_PID=$!

# Trap for cleanup
cleanup() {
    echo "Shutting down..."
    kill $BACKEND_PID $FRONTEND_PID 2>/dev/null || true
    wait
}
trap cleanup EXIT INT TERM

echo ""
echo "Both services started. Press Ctrl+C to stop."
wait
