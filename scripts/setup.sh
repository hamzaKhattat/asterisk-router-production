#!/bin/bash

echo "=== Asterisk Router Setup ==="
echo

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if binary exists
if [ ! -f "bin/router" ]; then
    echo -e "${RED}Error: Router binary not found. Run 'make build' first${NC}"
    exit 1
fi

# Initialize database
echo "Initializing database..."
./bin/router -init-db
if [ $? -ne 0 ]; then
    echo -e "${RED}Failed to initialize database${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Database initialized${NC}"
echo

# Example providers
echo "Adding example providers..."

# Inbound provider
./bin/router provider add s1 \
  --type inbound \
  --host 192.168.1.10 \
  --codecs "ulaw,alaw,g729" \
  --max-channels 100

# Intermediate providers
./bin/router provider add s3-1 \
  --type intermediate \
  --host 10.0.0.20 \
  --username user1 \
  --password pass1 \
  --codecs "ulaw,alaw" \
  --max-channels 50 \
  --weight 2

./bin/router provider add s3-2 \
  --type intermediate \
  --host 10.0.0.21 \
  --codecs "ulaw,alaw" \
  --max-channels 50 \
  --weight 1

# Final providers
./bin/router provider add s4-1 \
  --type final \
  --host 172.16.0.30 \
  --codecs "ulaw,alaw,g729" \
  --max-channels 200 \
  --priority 10

./bin/router provider add s4-2 \
  --type final \
  --host 172.16.0.31 \
  --codecs "ulaw,alaw,g729" \
  --max-channels 200 \
  --priority 5

echo -e "${GREEN}✓ Providers added${NC}"
echo

# Add DIDs
echo "Adding example DIDs..."

./bin/router did add 18001234567 18001234568 18001234569 --provider s3-1
./bin/router did add 18009876543 18009876544 --provider s3-2

echo -e "${GREEN}✓ DIDs added${NC}"
echo

# Create routes
echo "Creating routes..."

./bin/router route add main-route s1 s3-1 s4-1 --mode round_robin --priority 10
./bin/router route add backup-route s1 s3-2 s4-2 --mode failover --priority 5

echo -e "${GREEN}✓ Routes created${NC}"
echo

# Show summary
echo "=== Setup Complete ==="
echo
./bin/router provider list
echo
./bin/router route list
echo
./bin/router stats

echo
echo "To start the AGI server, run:"
echo "  ./bin/router -agi -verbose"
