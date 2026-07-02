#!/bin/sh
set -eu

API_BASE_URL="${KTUI_API_BASE_URL:-https://gitea.bytevibe.dev/api/v1/repos/gary/ktui}"
INSTALL_DIR="${KTUI_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${KTUI_VERSION:-latest}"
BINARY_NAME="ktui"

fail() {
	printf 'ktui install: %s\n' "$*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

http_get() {
	if [ -n "${KTUI_UPDATE_TOKEN:-}" ]; then
		curl -fsSL -H "Authorization: token $KTUI_UPDATE_TOKEN" "$1"
	elif [ -n "${GITEA_TOKEN:-}" ]; then
		curl -fsSL -H "Authorization: token $GITEA_TOKEN" "$1"
	else
		curl -fsSL "$1"
	fi
}

detect_os() {
	case "$(uname -s)" in
		Linux) printf linux ;;
		Darwin) printf darwin ;;
		*) fail "unsupported OS: $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
		x86_64 | amd64) printf amd64 ;;
		arm64 | aarch64) printf arm64 ;;
		*) fail "unsupported architecture: $(uname -m)" ;;
	esac
}

json_value() {
	if command -v jq >/dev/null 2>&1; then
		printf '%s' "$1" | jq -r --arg key "$2" '.[$key] // empty' | head -n 1
	else
		printf '%s' "$1" | sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" | head -n 1
	fi
}

release_asset_name() {
	suffix="_${os}_${arch}.tar.gz"
	if command -v jq >/dev/null 2>&1; then
		printf '%s' "$release_json" |
			jq -r --arg suffix "$suffix" '
				.assets[]? |
				select((.name // "") | startswith("ktui_") and endswith($suffix)) |
				.name
			' |
			head -n 1
	else
		printf '%s' "$release_json" |
			tr '{' '\n' |
			sed -n "s/.*\"name\"[[:space:]]*:[[:space:]]*\"\(ktui_[^\"]*_${os}_${arch}\.tar\.gz\)\".*/\1/p" |
			head -n 1
	fi
}

asset_url() {
	name="$1"
	if command -v jq >/dev/null 2>&1; then
		printf '%s' "$release_json" |
			jq -r --arg name "$name" '
				.assets[]? |
				select(.name == $name) |
				.browser_download_url // empty
			' |
			head -n 1
	else
		printf '%s' "$release_json" |
			tr '{' '\n' |
			grep "\"name\"[[:space:]]*:[[:space:]]*\"$name\"" |
			sed -n 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
			head -n 1
	fi
}

checksum_for() {
	name="$1"
	awk -v name="$name" '
		{
			file = $NF
			sub(/^\*/, "", file)
			n = split(file, parts, "/")
			if (parts[n] == name) {
				print tolower($1)
				exit
			}
		}
	' "$2"
}

verify_sha256() {
	file="$1"
	expected="$2"
	if command -v sha256sum >/dev/null 2>&1; then
		got="$(sha256sum "$file" | awk '{print tolower($1)}')"
	elif command -v shasum >/dev/null 2>&1; then
		got="$(shasum -a 256 "$file" | awk '{print tolower($1)}')"
	else
		fail "missing sha256sum or shasum"
	fi
	[ "$got" = "$expected" ] || fail "checksum mismatch for $(basename "$file")"
}

need curl
need tar
need awk
if ! command -v jq >/dev/null 2>&1; then
	need sed
	need grep
fi

os="$(detect_os)"
arch="$(detect_arch)"
endpoint="$API_BASE_URL/releases/latest"
if [ "$VERSION" != "latest" ]; then
	endpoint="$API_BASE_URL/releases/tags/$VERSION"
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/ktui-install.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

release_json="$(http_get "$endpoint")"
tag="$(json_value "$release_json" tag_name)"
[ -n "$tag" ] || fail "release response does not include tag_name"

asset_name="$(release_asset_name)"
[ -n "$asset_name" ] || fail "release $tag does not contain an asset for $os/$arch"

archive_url="$(asset_url "$asset_name")"
checksums_url="$(asset_url checksums.txt)"
[ -n "$archive_url" ] || fail "release asset $asset_name has no download URL"
[ -n "$checksums_url" ] || fail "release does not contain checksums.txt"

archive_path="$tmp_dir/$asset_name"
checksums_path="$tmp_dir/checksums.txt"

printf 'ktui install: downloading %s\n' "$asset_name"
http_get "$archive_url" >"$archive_path"
http_get "$checksums_url" >"$checksums_path"

expected="$(checksum_for "$asset_name" "$checksums_path")"
[ -n "$expected" ] || fail "checksums.txt does not contain $asset_name"
verify_sha256 "$archive_path" "$expected"

tar -xzf "$archive_path" -C "$tmp_dir"
[ -f "$tmp_dir/$BINARY_NAME" ] || fail "archive does not contain $BINARY_NAME"

mkdir -p "$INSTALL_DIR"
install_path="$INSTALL_DIR/$BINARY_NAME"
if command -v install >/dev/null 2>&1; then
	install -m 0755 "$tmp_dir/$BINARY_NAME" "$install_path"
else
	cp "$tmp_dir/$BINARY_NAME" "$install_path"
	chmod 0755 "$install_path"
fi

printf 'ktui install: installed %s\n' "$install_path"
case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*) printf 'ktui install: add %s to PATH if ktui is not found\n' "$INSTALL_DIR" ;;
esac
"$install_path" version
