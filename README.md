# Asterisk Router Production System

A comprehensive call routing system for Asterisk with dynamic DID management, multiple provider support, and advanced load balancing.

## Features

- **Dynamic Call Routing**: Routes calls through multiple providers with ANI/DNIS transformation
- **DID Management**: Automatic DID assignment and release
- **Load Balancing**: Multiple algorithms (round-robin, weighted, priority, failover)
- **Provider Management**: Support for IP and credential-based authentication
- **Real-time Configuration**: All configuration through ARA (Asterisk Realtime Architecture)
- **Extensive Logging**: Detailed logs for troubleshooting provider configurations
- **Call Verification**: Ensures calls are routed correctly and not intercepted

## Architecture

```
S1 (Inbound) -> S2 (Router) -> S3 (Intermediate) -> S2 (Router) -> S4 (Final)
```

## Installation

1. **Prerequisites**:
   - Go 1.19+
   - MySQL/MariaDB
   - Asterisk 16+ with ARA support

2. **Build**:
   ```bash
   make build
   ```

3. **Install**:
   ```bash
   make install
   ```

4. **Initialize Database**:
   ```bash
   router -init-db
   ```

## Configuration

Edit `configs/router.yaml`:

```yaml
database:
  host: localhost
  port: 3306
  user: root
  password: temppass
  name: asterisk_router

agi:
  port: 8002
```

## Usage

### Running the AGI Server

```bash
# Normal mode
router -agi

# Verbose mode (recommended for debugging)
router -agi -verbose
```

### CLI Commands

#### Provider Management

```bash
# Add a provider
router -cli provider add \
  --name "provider-name" \
  --type "inbound|intermediate|final" \
  --host "192.168.1.10" \
  --port 5060 \
  --username "user" \
  --password "pass" \
  --codecs "ulaw,alaw" \
  --max-channels 100 \
  --priority 10 \
  --weight 1

# List providers
router -cli provider list

# Delete a provider
router -cli provider delete provider-name
```

#### DID Management

```bash
# Add DIDs
router -cli did add \
  --provider "provider-name" \
  --number "18001234567" \
  --country "US" \
  --city "New York"

# List DIDs
router -cli did list
router -cli did list --provider "provider-name"
router -cli did list --in-use
router -cli did list --available

# Delete a DID
router -cli did delete 18001234567
```

#### Route Management

```bash
# Add a route
router -cli route add \
  --name "route-name" \
  --inbound "inbound-provider" \
  --intermediate "intermediate-provider" \
  --final "final-provider" \
  --mode "round_robin|weighted|priority|failover" \
  --priority 10

# List routes
router -cli route list

# Delete a route
router -cli route delete route-name
```

#### Monitoring

```bash
# Show statistics
router -cli stats

# Show load balancer status
router -cli loadbalancer

# Show recent calls
router -cli calls --limit 20
router -cli calls --status ACTIVE
```

## Load Balancing Modes

- **round_robin**: Distributes calls equally among providers
- **weighted**: Distributes based on provider weight values
- **priority**: Always uses highest priority provider first
- **failover**: Uses backup providers only when primary fails

## Troubleshooting

1. **Enable verbose logging**:
   ```bash
   router -agi -verbose
   ```

2. **Run troubleshooting script**:
   ```bash
   ./scripts/troubleshoot.sh
   ```

3. **Check logs**:
   - Router logs: `/var/log/asterisk-router.log`
   - Asterisk logs: `/var/log/asterisk/full`

4. **Common issues**:
   - **No DIDs available**: Add more DIDs or check if existing DIDs are stuck
   - **Provider authentication**: Check IP addresses and credentials
   - **Load balancing**: Verify provider health and channel limits

## Provider Authentication Types

- **IP-based**: No username/password required, authentication by source IP
- **Credentials**: Username and password required
- **Both**: Supports both methods (automatically detected)

## Example Setup

```bash
# 1. Add providers
./scripts/setup_providers.sh

# 2. Start AGI server
router -agi -verbose

# 3. Monitor calls
router -cli stats
```

## Development

```bash
# Run tests
make test

# Build for development
make build

# Clean build files
make clean
```

## License

MIT License
