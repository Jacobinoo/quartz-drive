#!/bin/bash
set -e

echo "🚀 Starting advanced zero-downtime deployment..."

# Load environment variables
set -a
source .env.production
set +a

echo "🔐 Logging into Docker Hub..."
echo "$DOCKERHUB_TOKEN" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin

# 1. Pull the latest image
echo "📦 Pulling latest image..."
docker compose -f docker-compose.prod.yml pull quartz-server

# 2. Find the ID of the currently running old API container
OLD_SERVER_ID=$(docker compose -f docker-compose.prod.yml ps -q quartz-server || true)

# 3. Start a new API container alongside the old one
echo "🌱 Starting new quartz-server container..."
docker compose -f docker-compose.prod.yml up -d --scale quartz-server=2 --no-recreate quartz-server

# Wait a brief moment for the new container to be registered by the daemon
sleep 2

# Find the ID of the NEW container (the one not matching OLD_ID)
ALL_SERVER_IDS=$(docker compose -f docker-compose.prod.yml ps -q quartz-server)
NEW_SERVER_ID=$(echo "$ALL_SERVER_IDS" | grep -v "$OLD_SERVER_ID" | head -n 1)

# If no old container existed, then ALL_SERVER_IDS is just the new one
if [ -z "$OLD_SERVER_ID" ]; then
    NEW_SERVER_ID=$ALL_SERVER_IDS
fi

echo "🔍 Monitoring health of new quartz-server ($NEW_SERVER_ID)..."

# 4. The Smart 60-second Health Loop
TIMEOUT=60
SUCCESS=false

for ((i=1; i<=TIMEOUT; i++)); do
    SERVER_HEALTH=$(docker inspect -f '{{.State.Health.Status}}' "$NEW_SERVER_ID" 2>/dev/null || echo "error")
    
    echo "   [$i/$TIMEOUT] quartz-server: $SERVER_HEALTH"
    
    if [ "$SERVER_HEALTH" = "healthy" ]; then
        SUCCESS=true
        break
    fi
    sleep 1
done

# 5. Success / Rollback Logic
if [ "$SUCCESS" = true ]; then
    echo "✅ New container is healthy!"
    
    # Safely kill old container
    if [ -n "$OLD_SERVER_ID" ]; then
        echo "🛑 Stopping old quartz-server..."
        docker stop "$OLD_SERVER_ID"
        docker rm "$OLD_SERVER_ID"
    fi
    
    # Reset scale back to 1
    docker compose -f docker-compose.prod.yml up -d --scale quartz-server=1 --no-deps quartz-server
    echo "🎉 Deployment complete! Zero downtime achieved."

else
    echo "❌ ERROR: New container failed to become healthy within $TIMEOUT seconds!"
    echo "🔄 Rolling back..."
    
    # Kill the broken new container
    if [ -n "$NEW_SERVER_ID" ]; then
        docker rm -f "$NEW_SERVER_ID"
    fi
    
    # Reset scale state
    docker compose -f docker-compose.prod.yml up -d --scale quartz-server=1 --no-deps quartz-server
    
    echo "🚨 Rollback complete. The old containers are still running untouched."
    exit 1
fi
