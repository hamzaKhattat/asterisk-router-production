#!/bin/bash
# Examples of different load balancing configurations

echo "=== Load Balancing Configuration Examples ==="

# Example 1: Round Robin - Equal distribution
echo "1. Round Robin Configuration:"
./router -cli route add \
  --name "route-round-robin" \
  --inbound "provider-inbound-1" \
  --intermediate "provider-intermediate-group" \
  --final "provider-final-group" \
  --mode "round_robin" \
  --priority 10

# Example 2: Weighted - Distribution based on capacity
echo "2. Weighted Configuration:"
./router -cli route add \
  --name "route-weighted" \
  --inbound "provider-inbound-1" \
  --intermediate "provider-intermediate-group" \
  --final "provider-final-group" \
  --mode "weighted" \
  --priority 10

# Example 3: Priority - Use highest priority first
echo "3. Priority Configuration:"
./router -cli route add \
  --name "route-priority" \
  --inbound "provider-inbound-1" \
  --intermediate "provider-intermediate-group" \
  --final "provider-final-group" \
  --mode "priority" \
  --priority 10

# Example 4: Failover - Use backup only on failure
echo "4. Failover Configuration:"
./router -cli route add \
  --name "route-failover" \
  --inbound "provider-inbound-1" \
  --intermediate "provider-intermediate-group" \
  --final "provider-final-group" \
  --mode "failover" \
  --priority 10

echo "
Load Balancing Modes Explained:
- round_robin: Calls distributed equally among all providers
- weighted: Calls distributed based on provider weight values
- priority: Always use highest priority provider first
- failover: Use backup providers only when primary fails
"
