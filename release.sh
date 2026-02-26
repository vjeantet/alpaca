#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
GITHUB_REPO="vjeantet/alpaca"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOMEBREW_TAP_DIR="${HOMEBREW_TAP_DIR:-$(cd "${SCRIPT_DIR}/../homebrew-tap" 2>/dev/null && pwd || echo "")}"
BUILD_DIR=""

# ---------------------------------------------------------------------------
# Utilities
# ---------------------------------------------------------------------------
info()    { printf '\033[1;34m[info]\033[0m %s\n' "$*"; }
success() { printf '\033[1;32m[ok]\033[0m   %s\n' "$*"; }
error()   { printf '\033[1;31m[err]\033[0m  %s\n' "$*" >&2; }

cleanup() {
    if [[ -n "${BUILD_DIR}" && -d "${BUILD_DIR}" ]]; then
        rm -rf "${BUILD_DIR}"
    fi
}
trap cleanup EXIT

confirm() {
    local msg="$1"
    printf '\033[1;33m[?]\033[0m %s [y/N] ' "$msg"
    read -r answer
    case "$answer" in
        [yY]|[yY][eE][sS]) return 0 ;;
        *) error "Aborted."; exit 1 ;;
    esac
}

# ---------------------------------------------------------------------------
# Preflight checks
# ---------------------------------------------------------------------------
preflight_checks() {
    info "Running preflight checks..."

    # Must be on master
    local branch
    branch="$(git -C "${SCRIPT_DIR}" rev-parse --abbrev-ref HEAD)"
    if [[ "${branch}" != "master" ]]; then
        error "Must be on master branch (currently on '${branch}')."
        exit 1
    fi
    success "On master branch."

    # Working tree must be clean
    if ! git -C "${SCRIPT_DIR}" diff --quiet || ! git -C "${SCRIPT_DIR}" diff --cached --quiet; then
        error "Working tree is not clean. Commit or stash your changes first."
        exit 1
    fi
    if [[ -n "$(git -C "${SCRIPT_DIR}" ls-files --others --exclude-standard)" ]]; then
        error "There are untracked files. Clean up or add them to .gitignore."
        exit 1
    fi
    success "Working tree is clean."

    # Tests
    info "Running tests..."
    CGO_ENABLED=1 go test ./...
    success "Tests passed."

    # Lint (warning only)
    if command -v golangci-lint &>/dev/null; then
        info "Running linter..."
        if golangci-lint run; then
            success "Linter passed."
        else
            error "Linter found issues. Fix them before releasing."
            exit 1
        fi
    else
        info "golangci-lint not found, skipping lint (warning)."
    fi

    # gh CLI
    if ! command -v gh &>/dev/null; then
        error "gh CLI is not installed. Install it: https://cli.github.com/"
        exit 1
    fi
    if ! gh auth status &>/dev/null; then
        error "gh CLI is not authenticated. Run: gh auth login"
        exit 1
    fi
    success "gh CLI is authenticated."

    # Homebrew tap directory
    if [[ -z "${HOMEBREW_TAP_DIR}" || ! -d "${HOMEBREW_TAP_DIR}" ]]; then
        error "Homebrew tap directory not found at '${HOMEBREW_TAP_DIR}'."
        error "Set HOMEBREW_TAP_DIR or clone homebrew-tap next to this repo."
        exit 1
    fi
    if ! git -C "${HOMEBREW_TAP_DIR}" diff --quiet || ! git -C "${HOMEBREW_TAP_DIR}" diff --cached --quiet; then
        error "Homebrew tap working tree is not clean (${HOMEBREW_TAP_DIR})."
        exit 1
    fi
    success "Homebrew tap directory found: ${HOMEBREW_TAP_DIR}"

    success "All preflight checks passed."
}

# ---------------------------------------------------------------------------
# Version calculation
# ---------------------------------------------------------------------------
calculate_version() {
    local year
    year="$(date +%y)"

    local max_n=0
    local tag
    while IFS= read -r tag; do
        [[ -z "$tag" ]] && continue
        local n="${tag#v${year}.}"
        if [[ "$n" =~ ^[0-9]+$ ]] && (( n > max_n )); then
            max_n=$n
        fi
    done < <(git -C "${SCRIPT_DIR}" tag -l "v${year}.*")

    VERSION="v${year}.$(( max_n + 1 ))"
    info "Next version: ${VERSION}"
}

# ---------------------------------------------------------------------------
# Build binaries
# ---------------------------------------------------------------------------
build_binaries() {
    BUILD_DIR="$(mktemp -d)"
    info "Building binaries in ${BUILD_DIR}..."

    local ldflags="-X 'main.BuildVersion=${VERSION}'"

    info "Building darwin/arm64..."
    CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
        go build -v -ldflags "${ldflags}" \
        -o "${BUILD_DIR}/alpaca_${VERSION}_darwin-arm64" .
    success "Built alpaca_${VERSION}_darwin-arm64"

    info "Building darwin/amd64..."
    CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
        go build -v -ldflags "${ldflags}" \
        -o "${BUILD_DIR}/alpaca_${VERSION}_darwin-amd64" .
    success "Built alpaca_${VERSION}_darwin-amd64"
}

# ---------------------------------------------------------------------------
# Tag and release
# ---------------------------------------------------------------------------
tag_and_release() {
    confirm "Create tag ${VERSION} and push to origin?"

    info "Creating tag ${VERSION}..."
    git -C "${SCRIPT_DIR}" tag -a "${VERSION}" -m "Release ${VERSION}"
    success "Tag ${VERSION} created."

    info "Pushing tag ${VERSION} to origin..."
    if ! git -C "${SCRIPT_DIR}" push origin "${VERSION}"; then
        error "Failed to push tag. You can delete the local tag with:"
        error "  git tag -d ${VERSION}"
        exit 1
    fi
    success "Tag ${VERSION} pushed."

    info "Creating GitHub release..."
    if ! gh release create "${VERSION}" \
        "${BUILD_DIR}/alpaca_${VERSION}_darwin-arm64" \
        "${BUILD_DIR}/alpaca_${VERSION}_darwin-amd64" \
        --repo "${GITHUB_REPO}" \
        --title "Release ${VERSION}" \
        --generate-notes; then
        error "Failed to create GitHub release. The tag was pushed."
        error "To clean up, run:"
        error "  git push origin :refs/tags/${VERSION}"
        error "  git tag -d ${VERSION}"
        exit 1
    fi
    success "GitHub release ${VERSION} created."
}

# ---------------------------------------------------------------------------
# Update Homebrew formula
# ---------------------------------------------------------------------------
update_homebrew_formula() {
    info "Updating Homebrew formula..."

    local version_no_v="${VERSION#v}"
    local sha_arm64 sha_amd64
    sha_arm64="$(shasum -a 256 "${BUILD_DIR}/alpaca_${VERSION}_darwin-arm64" | awk '{print $1}')"
    sha_amd64="$(shasum -a 256 "${BUILD_DIR}/alpaca_${VERSION}_darwin-amd64" | awk '{print $1}')"

    info "SHA256 arm64: ${sha_arm64}"
    info "SHA256 amd64: ${sha_amd64}"

    mkdir -p "${HOMEBREW_TAP_DIR}/Formula"

    cat > "${HOMEBREW_TAP_DIR}/Formula/alpaca-proxy.rb" <<FORMULA
class AlpacaProxy < Formula
  desc "Local HTTP proxy with PAC, NTLM, Basic and Kerberos authentication"
  homepage "https://github.com/vjeantet/alpaca"
  version "${version_no_v}"
  license "Apache-2.0"
  depends_on :macos

  on_arm do
    url "https://github.com/vjeantet/alpaca/releases/download/${VERSION}/alpaca_${VERSION}_darwin-arm64"
    sha256 "${sha_arm64}"
  end

  on_intel do
    url "https://github.com/vjeantet/alpaca/releases/download/${VERSION}/alpaca_${VERSION}_darwin-amd64"
    sha256 "${sha_amd64}"
  end

  def install
    binary_name = stable.url.split("/").last
    bin.install binary_name => "alpaca"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/alpaca -version")
  end
end
FORMULA
    success "Formula written to ${HOMEBREW_TAP_DIR}/Formula/alpaca-proxy.rb"

    # Update README.md in the tap if alpaca-proxy is not mentioned
    local tap_readme="${HOMEBREW_TAP_DIR}/README.md"
    if [[ -f "${tap_readme}" ]]; then
        if ! grep -q "alpaca-proxy" "${tap_readme}"; then
            cat >> "${tap_readme}" <<README

## alpaca-proxy

Local HTTP proxy with PAC, NTLM, Basic and Kerberos authentication.

\`\`\`sh
brew install vjeantet/tap/alpaca-proxy
\`\`\`
README
            info "Added alpaca-proxy section to tap README.md"
        fi
    fi

    confirm "Commit and push Homebrew formula update?"

    git -C "${HOMEBREW_TAP_DIR}" add -A
    git -C "${HOMEBREW_TAP_DIR}" commit -m "Update alpaca-proxy to ${version_no_v}"
    git -C "${HOMEBREW_TAP_DIR}" push
    success "Homebrew formula pushed."
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    info "Alpaca release script"
    echo ""

    preflight_checks
    echo ""

    calculate_version
    echo ""

    build_binaries
    echo ""

    tag_and_release
    echo ""

    update_homebrew_formula
    echo ""

    success "Release ${VERSION} complete!"
    echo ""
    info "GitHub: https://github.com/${GITHUB_REPO}/releases/tag/${VERSION}"
    info "Install: brew install vjeantet/tap/alpaca-proxy"
}

main "$@"
