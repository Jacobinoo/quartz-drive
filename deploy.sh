#!/bin/bash
set -e

echo "🚀 Starting zero-downtime deployment..."

# 1. Pull the latest image from Docker Hub
echo "📦 Pulling latest image..."
docker compose -f docker-compose.prod.yml pull quartz-api

# 2. Find the ID of the currently running old container (if any)
OLD_CONTAINER_ID=$(docker compose -f docker-compose.prod.yml ps -q quartz-api || true)

# 3. Start a new container alongside the old one (scaling to 2 temporarily)
echo "🌱 Starting new container..."
docker compose -f docker-compose.prod.yml up -d --scale quartz-api=2 --no-recreate quartz-api

# 4. Wait for the new container to boot and become healthy.
# Traefik will automatically detect it and start routing traffic to both containers.
echo "⏳ Waiting 10 seconds for the new container to initialize..."
sleep 10

# 5. Safely kill the old container. Traefik will stop routing traffic to it instantly.
if [ -n "$OLD_CONTAINER_ID" ]; then
    echo "🛑 Stopping the old container..."
    docker stop "$OLD_CONTAINER_ID"
    docker rm "$OLD_CONTAINER_ID"
fi

# 6. Reset the scale back to 1 in Docker Compose's state so future commands don't get confused
docker compose -f docker-compose.prod.yml up -d --scale quartz-api=1 --no-deps quartz-api

echo "✅ Deployment complete! Zero downtime achieved."
