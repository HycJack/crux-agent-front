#!/bin/bash
set -euo pipefail

# Crux Chat - Start both backend and frontend

DIR="$(cd "$(dirname "$0")" && pwd)"

# Load env if exists
if [ -f "$DIR/.env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "$DIR/.env"
    set +a
fi

# The backend refuses to start without JWT_SECRET — fail early with a useful message
# instead of letting it die with a bare log.Fatalf.
if [ -z "${JWT_SECRET:-}" ]; then
    echo "ERROR: JWT_SECRET is not set."
    echo ""
    echo "The backend will refuse to start without it. Generate one with:"
    echo "    openssl rand -hex 32"
    echo ""
    echo "Then add it to $DIR/.env (see .env.example):"
    echo "    JWT_SECRET=<paste-value>"
    exit 1
fi

# The backend listens on LISTEN_ADDR (e.g. :8080), not PORT.
LISTEN_ADDR="${LISTEN_ADDR:-:8080}"
BACKEND_PORT="${LISTEN_ADDR##*:}"

echo "=== Starting Crux Chat ==="
echo "Backend:  http://localhost:${BACKEND_PORT}"
echo "Frontend: http://localhost:3000"
echo ""

# Build & start backend
echo "[1/2] Building and starting backend..."
cd "$DIR/backend"
# NOTE: go build must not be piped — with `set -o pipefail` a failure here aborts
# before we try to exec a binary that was never built.
if ! go build -o "$DIR/hermes-chat-backend" .; then
    echo ""
    echo "ERROR: backend build failed (see output above)."
    exit 1
fi
cd "$DIR"
./hermes-chat-backend &
BACKEND_PID=$!

# Wait for the backend to come up so the frontend proxy isn't racing it
for _ in $(seq 1 30); do
    if curl -fsS "http://localhost:${BACKEND_PORT}/api/health" >/dev/null 2>&1; then
        echo "Backend healthy."
        break
    fi
    sleep 0.5
done

# Start frontend
echo "[2/2] Starting frontend..."
cd "$DIR/frontend"
if [ ! -d node_modules ]; then
    echo "Installing frontend dependencies (first run)..."
    npm install
fi
npx vite --host &
FRONTEND_PID=$!

# Trap for cleanup
cleanup() {
    echo ""
    echo "Shutting down..."
    kill $BACKEND_PID $FRONTEND_PID 2>/dev/null || true
    wait
}
trap cleanup EXIT INT TERM

echo ""
echo "Both services started. Press Ctrl+C to stop."
wait
