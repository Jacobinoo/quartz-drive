#!/bin/bash
set -e

# Detect active environment
# Default to blue (8080) if not found
ACTIVE_UPSTREAM=$(grep -o "127.0.0.1:[0-9]*" /etc/nginx/conf.d/quartz-upstream.conf || echo "127.0.0.1:8080")

if [[ "$ACTIVE_UPSTREAM" == *"8080"* ]]; then
    TARGET_ENV="green"
    TARGET_PORT=8081
    ACTIVE_ENV="blue"
else
    TARGET_ENV="blue"
    TARGET_PORT=8080
    ACTIVE_ENV="green"
fi

echo "Active environment is $ACTIVE_ENV. Deploying to $TARGET_ENV ($TARGET_PORT)..."

# 1. Stop target service
sudo systemctl stop quartz-$TARGET_ENV || true

# 2. Move binaries into place
sudo mv /home/$USER/deploy_temp/build/quartz-account /opt/quartz/quartz-$TARGET_ENV
sudo chmod +x /opt/quartz/quartz-$TARGET_ENV
sudo chown quartz:quartz /opt/quartz/quartz-$TARGET_ENV

sudo mv /home/$USER/deploy_temp/internal/bindings/opaque_rust/target/release/libopaque_rust.so /usr/lib/
sudo ldconfig

# Clean up temp
rm -rf /home/$USER/deploy_temp

# 3. Start target service
echo "Starting quartz-$TARGET_ENV..."
sudo systemctl start quartz-$TARGET_ENV

# 4. Health Check
echo "Waiting 3 seconds for service to boot..."
sleep 3

# -k ignores SSL cert validation, -f fails on HTTP errors (e.g. 404, 500)
if ! curl -k -f -s https://127.0.0.1:$TARGET_PORT/ > /dev/null; then
    echo "Health check failed! Aborting deployment."
    echo "Checking logs for $TARGET_ENV..."
    sudo journalctl -n 20 -u quartz-$TARGET_ENV.service
    sudo systemctl stop quartz-$TARGET_ENV
    exit 1
fi
echo "Health check passed!"

# 5. Switch Nginx
echo "Switching Nginx upstream to $TARGET_PORT..."
echo "upstream quartz_backend { server 127.0.0.1:$TARGET_PORT; }" | sudo tee /etc/nginx/conf.d/quartz-upstream.conf > /dev/null
sudo systemctl reload nginx

# 6. Shut down old service
echo "Shutting down old environment ($ACTIVE_ENV)..."
sudo systemctl stop quartz-$ACTIVE_ENV || true

echo "Zero-downtime deployment complete!"
