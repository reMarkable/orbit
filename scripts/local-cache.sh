#!/bin/bash
set -euo pipefail

HOSTNAME=${1:-}

if [ -z "$HOSTNAME" ]; then
  echo "Usage: $0 <hostname>"
  exit 1
fi

if ! grep -q "$HOSTNAME" /etc/hosts; then
  echo "127.0.0.1 $HOSTNAME" | sudo tee -a /etc/hosts
  echo "Added $HOSTNAME to /etc/hosts"
fi

if [ ! -f "deploy/$HOSTNAME.pem" ] || [ ! -f "deploy/$HOSTNAME.key" ]; then
  echo "Generating SSL/TLS certificate for $HOSTNAME"
  mkcert --cert-file "deploy/$HOSTNAME.pem" --key-file "deploy/$HOSTNAME.key" "$HOSTNAME"
  mkcert -install
fi

if [ ! -f "orbit" ]; then
  echo "Building orbit server"
    go build -o orbit cmd/server/main.go
fi

if ! getcap orbit | grep -q 'cap_net_bind_service=ep'; then
  echo "Setting capability to allow binding to port 443"
  sudo setcap 'cap_net_bind_service=+ep' orbit
fi

mkdir -p tmp
export CACHE_ENABLED=true
export CACHE_PATH=./tmp
export CACHE_EXPIRATION=24h

export PORT=443
export TLS_ENABLED=true
export TLS_KEY_FILE=deploy/$HOSTNAME.key
export TLS_CERT_FILE=deploy/$HOSTNAME.pem
./orbit
