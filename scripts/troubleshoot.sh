#!/bin/bash
# Troubleshooting script for provider issues

echo "=== Asterisk Router Troubleshooting ==="
echo

# Check database connectivity
echo "1. Checking database connectivity..."
mysql -u root -ptemppass -e "SELECT 1" asterisk_router &>/dev/null
if [ $? -eq 0 ]; then
    echo "   ✓ Database connection OK"
else
    echo "   ✗ Database connection FAILED"
fi

# Check AGI server
echo "2. Checking AGI server..."
nc -zv localhost 8002 &>/dev/null
if [ $? -eq 0 ]; then
    echo "   ✓ AGI server listening on port 8002"
else
    echo "   ✗ AGI server not responding"
fi

# Check Asterisk
echo "3. Checking Asterisk..."
asterisk -rx "core show version" &>/dev/null
if [ $? -eq 0 ]; then
    echo "   ✓ Asterisk is running"
else
    echo "   ✗ Asterisk not running"
fi

# Show provider status
echo
echo "4. Provider Status:"
./router -cli provider list

# Show DID availability
echo
echo "5. DID Status:"
./router -cli did list | head -20

# Show recent calls
echo
echo "6. Recent Calls:"
./router -cli calls --limit 10

# Show load balancer status
echo
echo "7. Load Balancer Status:"
./router -cli loadbalancer

# Check logs
echo
echo "8. Recent Errors in Logs:"
grep -i error /var/log/asterisk/full | tail -10

echo
echo "=== Troubleshooting Complete ==="
