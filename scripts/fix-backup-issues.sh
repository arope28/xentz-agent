#!/bin/bash
# fix-backup-issues.sh

echo "=== Step 1: Check Restic ==="
if ! command -v restic &> /dev/null; then
    echo "❌ Restic not installed"
    echo "Installing restic..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        brew install restic
    else
        echo "Please install restic manually: https://restic.net"
        exit 1
    fi
else
    echo "✅ Restic installed: $(restic version | head -1)"
fi

echo ""
echo "=== Step 2: Check Mock Server ==="
if curl -s http://local.xentz.test:8080/health > /dev/null; then
    echo "✅ Mock server is running"
else
    echo "❌ Mock server is not running"
    echo "Start it with: cd mock-server && docker-compose up -d"
    exit 1
fi

echo ""
echo "=== Step 3: Check Config ==="
CONFIG_FILE="$HOME/.xentz-agent/config.json"
if [ ! -f "$CONFIG_FILE" ]; then
    echo "❌ Config file not found: $CONFIG_FILE"
    echo "Run: xentz-agent install --token test-token-123 --server http://local.xentz.test:8080 --include /tmp/test"
    exit 1
fi

REPO=$(cat "$E" | grep -o '"repository": "[^"]*' | cut -d'"' -f4)
PASSWORD_FILE=$(cat "$CONFIG_FILE" | grep -o '"password_file": "[^"]*' | cut -d'"' -f4)

echo "Repository: $REPO"
echo "Password file: $PASSWORD_FILE"

echo ""
echo "=== Step 4: Check Password File ==="
if [ -f "$PASSWORD_FILE" ]; then
    echo "✅ Password file exists"
else
    echo "❌ Password file not found: $PASSWORD_FILE"
    echo "Creating with default password..."
    mkdir -p "$(dirname "$PASSWORD_FILE")"
    echo "test-restic-password-12345" > "$PASSWORD_FILE"
    chmod 600 "$PASSWORD_FILE"
fi

echo ""
echo "=== Step 5: Test Repository Connection ==="
if echo "$REPO" | grep -q "s3:"; then
    echo "S3 backend detected"
    export AWS_ACCESS_KEY_ID=minioadmin
    export AWS_SECRET_ACCESS_KEY=minioadmin123
    export AWS_DEFAULT_REGION=us-east-1
    
    # Extract bucket name from URL
    BUCKET=$(echo "$REPO" | sed 's|s3:http://[^/]*/||' | cut -d'/' -f1)
    echo "Bucket name: $BUCKET"
    
    # Check if MinIO is accessible
    if curl -s httpocal.xentz.test:9000/minio/health/live > /dev/null; then
        echo "✅ MinIO is running"
    else
        echo "❌ MinIO is not running"
        echo "Start it with: cd mock-server && docker-compose up -d minio"
        exit 1
    fi
    
    # Try to initialize repository
    echo "Attempting to initialize repository..."
    restic -r "$REPO" init 2>&1
else
    echo "REST backend detected"
    # Test REST server connection
    REST_URL=$(echo "$REPO" | sed 's|rest:||' | sed 's|/[^/]*$||')
    if curl -s "$REST_URL" > /dev/null; then
        echo "✅ REST server is accessible"
    else
        echo "❌ REST server is not accessible: $REST_URL"
        exit 1
    fi
fi

echo ""
echo "=== Step 6: Try Manual Backup Test ==="
echo "Testing with a small backup..."
mkdir -p /tmp/test-backup
echo "test file" > /tmp/test-backup/test.txt

export RESTIC_REPOSITORY="$REPO"
export RESTIC_PASSWORD_FILE="$PASSWORD_FILE"
if echo "$REPO" | grep -q "s3:"; then
    export AWS_ACCESS_KEY_ID=minioadmin
    export AWS_SEESS_KEY=minioadmin123
    export AWS_DEFAULT_REGION=us-east-1
fi

restic backup /tmp/test-backup 2>&1 | tail -5
