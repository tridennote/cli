#!/usr/bin/env bash
set -euo pipefail

REPO="tridennote/cli"
BINARY="tridennote"

case "$(uname -s)" in
  Linux*) PLATFORM="linux" ;;
  Darwin*) PLATFORM="darwin" ;;
  *) echo "Unsupported OS: $(uname -s). On Windows, download the zip from https://github.com/${REPO}/releases" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) ARCH="amd64" ;;
  arm64 | aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

TAG="${TRIDENNOTE_VERSION:-}"
if [ -z "${TAG}" ]; then
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')"
fi
if [ -z "${TAG}" ]; then
  echo "Could not find the latest ${BINARY} release." >&2
  exit 1
fi

ARCHIVE="${BINARY}_${PLATFORM}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

echo "Installing ${BINARY} ${TAG} for ${PLATFORM}/${ARCH}…"
curl -fsSL "${BASE_URL}/${ARCHIVE}" -o "${TMP_DIR}/${ARCHIVE}"
curl -fsSL "${BASE_URL}/checksums.txt" -o "${TMP_DIR}/checksums.txt"

EXPECTED="$(grep " ${ARCHIVE}\$" "${TMP_DIR}/checksums.txt" | awk '{print $1}')"
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "${TMP_DIR}/${ARCHIVE}" | awk '{print $1}')"
else
  ACTUAL="$(shasum -a 256 "${TMP_DIR}/${ARCHIVE}" | awk '{print $1}')"
fi
if [ -z "${EXPECTED}" ] || [ "${EXPECTED}" != "${ACTUAL}" ]; then
  echo "Checksum mismatch for ${ARCHIVE}; aborting." >&2
  exit 1
fi

tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "${TMP_DIR}"

INSTALL_DIR="${TRIDENNOTE_INSTALL_DIR:-/usr/local/bin}"
if [ -z "${TRIDENNOTE_INSTALL_DIR:-}" ] && [ ! -w "${INSTALL_DIR}" ]; then
  INSTALL_DIR="${HOME}/.local/bin"
fi
mkdir -p "${INSTALL_DIR}"
mv "${TMP_DIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
chmod +x "${INSTALL_DIR}/${BINARY}"

echo "Installed $("${INSTALL_DIR}/${BINARY}" --version) to ${INSTALL_DIR}/${BINARY}"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) echo "Note: ${INSTALL_DIR} is not on your PATH. Add it to your ~/.zshrc or ~/.bashrc." ;;
esac
