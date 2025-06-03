#!/bin/bash
# Example script to set up providers

# Add inbound provider (S1)
./router -cli provider add \
  --name "provider-s1" \
  --type "inbound" \
  --host "192.168.1.10" \
  --port 5060 \
  --codecs "ulaw,alaw,g729" \
  --max-channels 100 \
  --priority 10 \
  --weight 1

# Add intermediate providers (S3)
./router -cli provider add \
  --name "provider-s3-1" \
  --type "intermediate" \
  --host "10.0.0.20" \
  --port 5060 \
  --username "user1" \
  --password "pass1" \
  --codecs "ulaw,alaw" \
  --max-channels 50 \
  --priority 10 \
  --weight 2

./router -cli provider add \
  --name "provider-s3-2" \
  --type "intermediate" \
  --host "10.0.0.21" \
  --port 5060 \
  --username "user2" \
  --password "pass2" \
  --codecs "ulaw,alaw" \
  --max-channels 50 \
  --priority 10 \
  --weight 1

# Add final providers (S4)
./router -cli provider add \
  --name "provider-s4-1" \
  --type "final" \
  --host "172.16.0.30" \
  --port 5060 \
  --codecs "ulaw,alaw,g729" \
  --max-channels 200 \
  --priority 10 \
  --weight 3

./router -cli provider add \
  --name "provider-s4-2" \
  --type "final" \
  --host "172.16.0.31" \
  --port 5060 \
  --codecs "ulaw,alaw,g729" \
  --max-channels 200 \
  --priority 5 \
  --weight 1

# Add DIDs for intermediate providers
./router -cli did add \
  --provider "provider-s3-1" \
  --number "18001234567" \
  --country "US" \
  --city "New York"

./router -cli did add \
  --provider "provider-s3-1" \
  --number "18001234568" \
  --country "US" \
  --city "New York"

./router -cli did add \
  --provider "provider-s3-2" \
  --number "18009876543" \
  --country "US" \
  --city "Los Angeles"

# Create routes with different load balancing modes
./router -cli route add \
  --name "route-main" \
  --inbound "provider-s1" \
  --intermediate "provider-s3-1" \
  --final "provider-s4-1" \
  --mode "round_robin" \
  --priority 10

./router -cli route add \
  --name "route-backup" \
  --inbound "provider-s1" \
  --intermediate "provider-s3-2" \
  --final "provider-s4-2" \
  --mode "failover" \
  --priority 5

echo "Providers, DIDs, and routes configured successfully!"
