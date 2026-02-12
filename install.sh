#!/usr/bin/env bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
REPO="seunggabi/claude-dashboard"
DEFAULT_BINARY_NAME="claude-dashboard"
INSTALL_DIR="$HOME/.local/bin"

# Helper function to run commands and exit on failure
ensure() {
    if ! "$@"; then
        echo -e "${RED}Error: Command failed: $*${NC}" >&2
        exit 1
    fi
}

# Print colored messages
print_info() {
    echo -e "${GREEN}$1${NC}"
}

print_warn() {
    echo -e "${YELLOW}$1${NC}"
}

print_error() {
    echo -e "${RED}$1${NC}"
}

# Setup shell and PATH
setup_shell_and_path() {
    print_info "Setting up shell configuration..."

    # Create .local/bin if it doesn't exist
    mkdir -p "$INSTALL_DIR"

    # Detect shell
    local shell_config=""
    if [ -n "$ZSH_VERSION" ]; then
        shell_config="$HOME/.zshrc"
    elif [ -n "$BASH_VERSION" ]; then
        if [ -f "$HOME/.bashrc" ]; then
            shell_config="$HOME/.bashrc"
        elif [ -f "$HOME/.bash_profile" ]; then
            shell_config="$HOME/.bash_profile"
        fi
    fi

    # Add to PATH if not already there
    if [ -n "$shell_config" ]; then
        if ! grep -q "$INSTALL_DIR" "$shell_config" 2>/dev/null; then
            echo "" >> "$shell_config"
            echo "# Added by claude-dashboard installer" >> "$shell_config"
            echo "export PATH=\"\$HOME/.local/bin:\$PATH\"" >> "$shell_config"
            print_info "Added $INSTALL_DIR to PATH in $shell_config"
        fi
    fi

    # Add to current session PATH
    export PATH="$INSTALL_DIR:$PATH"
}

# Detect platform and architecture
detect_platform_and_arch() {
    # Detect OS
    case "$(uname -s)" in
        Linux*)
            PLATFORM="linux"
            ;;
        Darwin*)
            PLATFORM="darwin"
            ;;
        *)
            print_error "Unsupported operating system: $(uname -s)"
            exit 1
            ;;
    esac

    # Detect architecture
    local machine_arch="$(uname -m)"
    case "$machine_arch" in
        x86_64|amd64)
            ARCHITECTURE="amd64"
            ;;
        arm64|aarch64)
            ARCHITECTURE="arm64"
            ;;
        *)
            print_error "Unsupported architecture: $machine_arch"
            exit 1
            ;;
    esac

    # Handle Rosetta on macOS
    if [ "$PLATFORM" = "darwin" ] && [ "$ARCHITECTURE" = "arm64" ]; then
        if [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = "1" ]; then
            print_warn "Running under Rosetta, using amd64 binary"
            ARCHITECTURE="amd64"
        fi
    fi

    print_info "Detected platform: $PLATFORM $ARCHITECTURE"
}

# Get latest version from GitHub
get_latest_version() {
    print_info "Fetching latest version from GitHub..."

    local api_url="https://api.github.com/repos/$REPO/releases/latest"
    local version_json

    if command -v curl >/dev/null 2>&1; then
        version_json=$(curl -s "$api_url")
    elif command -v wget >/dev/null 2>&1; then
        version_json=$(wget -qO- "$api_url")
    else
        print_error "Neither curl nor wget found. Please install one of them."
        exit 1
    fi

    # Extract version tag (remove 'v' prefix)
    VERSION=$(echo "$version_json" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/')

    if [ -z "$VERSION" ]; then
        print_error "Failed to fetch latest version"
        exit 1
    fi

    print_info "Latest version: $VERSION"
}

# Download release
download_release() {
    local download_url="$1"
    local output_file="$2"

    print_info "Downloading from $download_url..."

    if command -v curl >/dev/null 2>&1; then
        ensure curl -fsSL -o "$output_file" "$download_url"
    elif command -v wget >/dev/null 2>&1; then
        ensure wget -q -O "$output_file" "$download_url"
    else
        print_error "Neither curl nor wget found. Please install one of them."
        exit 1
    fi
}

# Extract and install
extract_and_install() {
    local archive_file="$1"
    local binary_name="$2"
    local temp_dir="$3"

    print_info "Extracting archive..."
    ensure tar -xzf "$archive_file" -C "$temp_dir"

    # Find the binary (should be named claude-dashboard)
    local binary_path="$temp_dir/claude-dashboard"

    if [ ! -f "$binary_path" ]; then
        print_error "Binary not found in archive"
        exit 1
    fi

    # Make executable
    ensure chmod +x "$binary_path"

    # Move to install directory
    local install_path="$INSTALL_DIR/$binary_name"
    print_info "Installing to $install_path..."
    ensure mv "$binary_path" "$install_path"

    # Install helper scripts if they exist in the archive
    if [ -d "$temp_dir/scripts" ]; then
        print_info "Installing helper scripts..."
        ensure cp "$temp_dir/scripts/tmux-mouse-toggle.sh" "$INSTALL_DIR/claude-dashboard-mouse-toggle"
        ensure cp "$temp_dir/scripts/tmux-status-bar.sh" "$INSTALL_DIR/claude-dashboard-status-bar"
        ensure chmod +x "$INSTALL_DIR/claude-dashboard-mouse-toggle"
        ensure chmod +x "$INSTALL_DIR/claude-dashboard-status-bar"
    fi

    print_info "Installation complete!"
}

# Check if command exists
check_command_exists() {
    local cmd="$1"
    if command -v "$cmd" >/dev/null 2>&1; then
        local current_version
        current_version=$("$cmd" --version 2>/dev/null | head -n1 || echo "unknown")
        print_warn "$cmd is already installed: $current_version"
        print_info "This will upgrade/reinstall it."
        return 0
    fi
    return 1
}

# Check and install dependencies
check_and_install_dependencies() {
    print_info "Checking dependencies..."

    # Check for tmux
    if ! command -v tmux >/dev/null 2>&1; then
        print_warn "tmux is not installed"

        if [ "$PLATFORM" = "darwin" ]; then
            if command -v brew >/dev/null 2>&1; then
                print_info "Installing tmux via Homebrew..."
                ensure brew install tmux
            else
                print_error "Homebrew not found. Please install tmux manually:"
                print_error "  brew install tmux"
                exit 1
            fi
        elif [ "$PLATFORM" = "linux" ]; then
            if command -v apt-get >/dev/null 2>&1; then
                print_info "Installing tmux via apt-get..."
                ensure sudo apt-get update
                ensure sudo apt-get install -y tmux
            elif command -v yum >/dev/null 2>&1; then
                print_info "Installing tmux via yum..."
                ensure sudo yum install -y tmux
            elif command -v pacman >/dev/null 2>&1; then
                print_info "Installing tmux via pacman..."
                ensure sudo pacman -S --noconfirm tmux
            else
                print_error "Package manager not found. Please install tmux manually."
                exit 1
            fi
        fi
    else
        print_info "tmux is already installed"
    fi
}

# Main installation flow
main() {
    local binary_name="$DEFAULT_BINARY_NAME"

    # Parse arguments
    while [ $# -gt 0 ]; do
        case "$1" in
            --name)
                binary_name="$2"
                shift 2
                ;;
            *)
                print_error "Unknown option: $1"
                echo "Usage: $0 [--name BINARY_NAME]"
                exit 1
                ;;
        esac
    done

    print_info "Installing claude-dashboard as '$binary_name'..."

    # Check if already installed
    check_command_exists "$binary_name" || true

    # Detect platform and architecture
    detect_platform_and_arch

    # Check and install dependencies
    check_and_install_dependencies

    # Setup shell and PATH
    setup_shell_and_path

    # Get version (from env or fetch latest)
    if [ -z "$VERSION" ]; then
        get_latest_version
    else
        print_info "Using version from environment: $VERSION"
    fi

    # Build download URL
    local archive_name="claude-dashboard_${VERSION}_${PLATFORM}_${ARCHITECTURE}.tar.gz"
    local download_url="https://github.com/$REPO/releases/download/v${VERSION}/${archive_name}"

    # Create temporary directory
    local temp_dir
    temp_dir=$(mktemp -d)
    trap 'rm -rf "$temp_dir"' EXIT

    local archive_file="$temp_dir/$archive_name"

    # Download release
    download_release "$download_url" "$archive_file"

    # Extract and install
    extract_and_install "$archive_file" "$binary_name" "$temp_dir"

    # Success message
    echo ""
    print_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    print_info "  claude-dashboard v${VERSION} installed successfully!"
    print_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    print_info "Command: $binary_name"
    print_info "Location: $INSTALL_DIR/$binary_name"
    echo ""
    print_info "To start using it, either:"
    print_info "  1. Restart your terminal, or"
    print_info "  2. Run: export PATH=\"$INSTALL_DIR:\$PATH\""
    echo ""
    print_info "Then run: $binary_name"
    echo ""
}

main "$@"
