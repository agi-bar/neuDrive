#!/usr/bin/env bash
set -euo pipefail

BIN_NAMES=("neudrive" "neu")
PRIMARY_BIN="${BIN_NAMES[0]}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BIN_DIR=""
FORCE=0
NO_RC=0

usage() {
  cat <<'EOF'
Usage: tools/install-neudrive.sh [--bin-dir DIR] [--force] [--no-rc]

Builds the local neuDrive binaries, installs both `neudrive` and `neu` into a
writable bin directory, and adds that directory to your shell PATH if needed.

Options:
  --bin-dir DIR  Install into this directory instead of auto-selecting one
  --force        Overwrite an existing installed binary
  --no-rc        Do not modify shell rc files even if PATH needs updating
  -h, --help     Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bin-dir)
      [[ $# -ge 2 ]] || { echo "missing value for --bin-dir" >&2; exit 2; }
      BIN_DIR="$2"
      shift 2
      ;;
    --force)
      FORCE=1
      shift
      ;;
    --no-rc)
      NO_RC=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd go
need_cmd install

is_in_path_now() {
  local dir="$1"
  case ":${PATH:-}:" in
    *":${dir}:"*) return 0 ;;
    *) return 1 ;;
  esac
}

first_writable_path_dir() {
  local entry
  IFS=':' read -r -a path_entries <<< "${PATH:-}"
  for entry in "${path_entries[@]}"; do
    [[ -n "$entry" && "$entry" != "." ]] || continue
    if [[ -d "$entry" && -w "$entry" ]]; then
      printf '%s\n' "$entry"
      return 0
    fi
  done
  return 1
}

select_bin_dir() {
  if [[ -n "$BIN_DIR" ]]; then
    printf '%s\n' "$BIN_DIR"
    return 0
  fi
  if dir="$(first_writable_path_dir)"; then
    printf '%s\n' "$dir"
    return 0
  fi
  if [[ -n "${HOME:-}" ]]; then
    printf '%s\n' "${HOME}/.local/bin"
    return 0
  fi
  echo "could not determine an install directory" >&2
  exit 1
}

choose_rc_file() {
  local shell_name
  shell_name="$(basename "${SHELL:-}")"
  case "$shell_name" in
    zsh)
      printf '%s\n' "${HOME}/.zshrc"
      ;;
    bash)
      if [[ -f "${HOME}/.bashrc" ]]; then
        printf '%s\n' "${HOME}/.bashrc"
      else
        printf '%s\n' "${HOME}/.bash_profile"
      fi
      ;;
    fish)
      printf '%s\n' "${HOME}/.config/fish/config.fish"
      ;;
    *)
      printf '%s\n' ""
      ;;
  esac
}

ensure_path_in_shell_rc() {
  local dir="$1"
  local rc_file shell_name line
  rc_file="$(choose_rc_file)"
  shell_name="$(basename "${SHELL:-}")"
  [[ -n "$rc_file" ]] || return 1

  mkdir -p "$(dirname "$rc_file")"
  touch "$rc_file"

  case "$shell_name" in
    fish)
      line="fish_add_path -m ${dir}"
      ;;
    *)
      line="export PATH=\"${dir}:\$PATH\""
      ;;
  esac

  if grep -Fqs "$line" "$rc_file"; then
    return 0
  fi

  {
    printf '\n# Added by neuDrive installer\n'
    printf '%s\n' "$line"
  } >> "$rc_file"
}

INSTALL_DIR="$(select_bin_dir)"
INSTALL_DIR="${INSTALL_DIR/#\~/${HOME}}"
TARGET_PATHS=()
for bin_name in "${BIN_NAMES[@]}"; do
  target_path="${INSTALL_DIR}/${bin_name}"
  TARGET_PATHS+=("$target_path")
  if [[ -e "$target_path" && "$FORCE" -ne 1 ]]; then
    echo "${target_path} already exists; rerun with --force to overwrite" >&2
    exit 1
  fi
done

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

mkdir -p "$INSTALL_DIR"

echo "Building ${BIN_NAMES[*]} from ${REPO_ROOT} ..."
(
  cd "$REPO_ROOT"
  go build -o "${TMP_DIR}/neudrive" ./cmd/neudrive
  go build -o "${TMP_DIR}/neu" ./cmd/neu
)

for bin_name in "${BIN_NAMES[@]}"; do
  install -m 0755 "${TMP_DIR}/${bin_name}" "${INSTALL_DIR}/${bin_name}"
done

PATH_UPDATED=0
RC_FILE=""
if ! is_in_path_now "$INSTALL_DIR" && [[ "$NO_RC" -ne 1 ]]; then
  if ensure_path_in_shell_rc "$INSTALL_DIR"; then
    PATH_UPDATED=1
    RC_FILE="$(choose_rc_file)"
  fi
fi

echo
echo "Installed:"
for target_path in "${TARGET_PATHS[@]}"; do
  echo "  ${target_path}"
done

if is_in_path_now "$INSTALL_DIR"; then
  echo "PATH: ${INSTALL_DIR} is already on PATH"
  hash -r 2>/dev/null || true
  echo
  echo "You can use them now:"
  echo "  ${PRIMARY_BIN} status"
  echo "  neu status"
  exit 0
fi

if [[ "$PATH_UPDATED" -eq 1 ]]; then
  echo "PATH: added ${INSTALL_DIR} to ${RC_FILE}"
  echo
  echo "Open a new shell, or run:"
  case "$(basename "${SHELL:-}")" in
    fish)
      echo "  source ${RC_FILE}"
      ;;
    *)
      echo "  source ${RC_FILE}"
      ;;
  esac
  echo
  echo "Then use:"
  echo "  ${PRIMARY_BIN} status"
  echo "  neu status"
  exit 0
fi

echo "PATH: ${INSTALL_DIR} is not on PATH"
echo "Add it manually, or rerun with --bin-dir pointing to a writable PATH directory."
echo
echo "Then use:"
echo "  ${PRIMARY_BIN} status"
echo "  neu status"
