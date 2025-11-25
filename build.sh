#!/bin/bash

# ═══════════════════════════════════════════════════════════════
# NOFX AI Trading System - Native Build Script
# Usage: ./build.sh [options]
# Output: nofx (native binary with embedded frontend)
# ═══════════════════════════════════════════════════════════════

set -e

# ------------------------------------------------------------------------
# Color Definitions
# ------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ------------------------------------------------------------------------
# Utility Functions: Colored Output
# ------------------------------------------------------------------------
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# ------------------------------------------------------------------------
# Variables
# ------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_BINARY="nofx"
SKIP_FRONTEND=false
SKIP_TEST=false

# ------------------------------------------------------------------------
# Parse Arguments
# ------------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-frontend)
            SKIP_FRONTEND=true
            shift
            ;;
        --skip-test)
            SKIP_TEST=true
            shift
            ;;
        --output|-o)
            OUTPUT_BINARY="$2"
            shift 2
            ;;
        --help|-h)
            echo "NOFX Native Build Script"
            echo ""
            echo "Usage: ./build.sh [options]"
            echo ""
            echo "Options:"
            echo "  --skip-frontend    Skip frontend build (use existing web/dist)"
            echo "  --skip-test        Skip running tests before build"
            echo "  --output, -o       Output binary name (default: nofx)"
            echo "  --help, -h         Show this help message"
            echo ""
            echo "Prerequisites:"
            echo "  - Go 1.21+"
            echo "  - Node.js 18+ (for frontend build)"
            echo ""
            echo "Output:"
            echo "  - nofx binary with embedded frontend"
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ------------------------------------------------------------------------
# Check Prerequisites
# ------------------------------------------------------------------------
check_prerequisites() {
    print_info "Checking prerequisites..."

    # Check Go
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed! Please install Go 1.21+"
        print_info "Download: https://go.dev/dl/"
        exit 1
    fi

    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    print_success "Go version: $GO_VERSION"

    # Check Node.js (only if building frontend)
    if [ "$SKIP_FRONTEND" = false ]; then
        if ! command -v node &> /dev/null; then
            print_error "Node.js is not installed! Please install Node.js 18+"
            print_info "Download: https://nodejs.org/"
            exit 1
        fi

        NODE_VERSION=$(node --version)
        print_success "Node.js version: $NODE_VERSION"

        if ! command -v npm &> /dev/null; then
            print_error "npm is not installed!"
            exit 1
        fi

        NPM_VERSION=$(npm --version)
        print_success "npm version: $NPM_VERSION"
    fi
}

# ------------------------------------------------------------------------
# Build Frontend
# ------------------------------------------------------------------------
build_frontend() {
    if [ "$SKIP_FRONTEND" = true ]; then
        if [ ! -d "$SCRIPT_DIR/web/dist" ]; then
            print_error "web/dist not found! Cannot skip frontend build."
            exit 1
        fi
        print_warning "Skipping frontend build (using existing web/dist)"
        return
    fi

    print_info "Building frontend..."
    cd "$SCRIPT_DIR/web"

    # Install dependencies
    print_info "Installing npm dependencies..."
    npm install

    # Build frontend
    print_info "Running npm build..."
    npm run build

    cd "$SCRIPT_DIR"

    if [ ! -d "$SCRIPT_DIR/web/dist" ]; then
        print_error "Frontend build failed! web/dist not created."
        exit 1
    fi

    print_success "Frontend build completed"
}

# ------------------------------------------------------------------------
# Run Tests
# ------------------------------------------------------------------------
run_tests() {
    if [ "$SKIP_TEST" = true ]; then
        print_warning "Skipping tests"
        return
    fi

    print_info "Running Go tests..."
    cd "$SCRIPT_DIR"

    if ! go test ./...; then
        print_error "Tests failed! Fix tests before building."
        exit 1
    fi

    print_success "All tests passed"
}

# ------------------------------------------------------------------------
# Build Backend
# ------------------------------------------------------------------------
build_backend() {
    print_info "Building backend..."
    cd "$SCRIPT_DIR"

    # Build with embedded frontend
    print_info "Compiling Go binary (with embedded frontend)..."
    go build -o "$OUTPUT_BINARY" .

    if [ ! -f "$SCRIPT_DIR/$OUTPUT_BINARY" ]; then
        print_error "Backend build failed!"
        exit 1
    fi

    # Show binary info
    BINARY_SIZE=$(du -h "$OUTPUT_BINARY" | cut -f1)
    print_success "Backend build completed: $OUTPUT_BINARY ($BINARY_SIZE)"
}

# ------------------------------------------------------------------------
# Main
# ------------------------------------------------------------------------
main() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "  NOFX AI Trading System - Native Build"
    echo "═══════════════════════════════════════════════════════════════"
    echo ""

    cd "$SCRIPT_DIR"

    # Step 1: Check prerequisites
    check_prerequisites

    echo ""

    # Step 2: Build frontend
    build_frontend

    echo ""

    # Step 3: Run tests
    run_tests

    echo ""

    # Step 4: Build backend
    build_backend

    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    print_success "Build completed successfully!"
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
    print_info "Output: $SCRIPT_DIR/$OUTPUT_BINARY"
    echo ""
    print_info "To run:"
    echo "  1. cp config.json.example config.json"
    echo "  2. Edit config.json (set jwt_secret)"
    echo "  3. ./$OUTPUT_BINARY"
    echo "  4. Open http://localhost:3000"
    echo ""
}

main
