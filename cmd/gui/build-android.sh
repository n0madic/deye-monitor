#!/usr/bin/env bash
#
# Build a sideload-ready Android APK for the Deye Monitor GUI.
#
# Why this script exists (and a plain `fyne package -os android` is not enough):
#
#   * Fyne's bundled mobile builder hard-codes targetSdkVersion = 29 for debug
#     packages (only its `release`/.aab path uses 35). Modern Android (11+, and
#     enforced harder on 15/16) refuses to install APKs whose target SDK is too
#     old, so we must build with a current target (35).
#   * Fyne's own APK writer signs with APK Signature Scheme v1 (JAR) only. Android
#     requires v2+ for any app targeting API 30+. So after building we re-align and
#     re-sign the APK with apksigner (v1+v2+v3).
#
# The script builds a one-line-patched copy of the `fyne` CLI (target 35) from the
# module cache, packages arm64, then aligns and re-signs with a local keystore.
#
# Requirements: Go, Android SDK (platform 34/35, build-tools, NDK), a JDK (keytool).
# Set ANDROID_HOME / ANDROID_NDK_HOME, or rely on the defaults below.

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$here"

# --- Configuration -----------------------------------------------------------
APP_ID="com.deye.monitor"
ICON="Icon.png"
TARGET_OS="android/arm64"          # arm64 covers every modern phone; smaller APK
KEYSTORE="$here/deye.keystore"
KEY_ALIAS="deye"
STORE_PASS="${DEYE_STORE_PASS:-deye-monitor}"
KEY_PASS="${DEYE_KEY_PASS:-deye-monitor}"
OUT_APK="$here/Deye_Monitor.apk"

ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
BUILD_TOOLS_VER="${BUILD_TOOLS_VER:-34.0.0}"
NDK_VER="${NDK_VER:-26.3.11579264}"
export ANDROID_HOME
export ANDROID_NDK_HOME="${ANDROID_NDK_HOME:-$ANDROID_HOME/ndk/$NDK_VER}"
BT="$ANDROID_HOME/build-tools/$BUILD_TOOLS_VER"

# --- 1. Build a target-35 patched copy of the fyne CLI ------------------------
PATCHED_FYNE="${PATCHED_FYNE:-/tmp/fyne-patched}"
if [[ ! -x "$PATCHED_FYNE" ]]; then
  echo ">> Building target-35 patched fyne CLI..."
  tools_src="$(ls -d "$(go env GOMODCACHE)"/fyne.io/tools@* 2>/dev/null | sort -V | tail -1 || true)"
  if [[ -z "$tools_src" ]]; then
    echo "fyne tools module not found in the module cache." >&2
    echo "Run: go install fyne.io/tools/cmd/fyne@latest" >&2
    exit 1
  fi
  work="$(mktemp -d)"
  cp -r "$tools_src"/. "$work"/
  chmod -R u+w "$work"
  # Debug packages default to target 29; bump to 35 (we re-sign v2/v3 below).
  sed -i '' 's|target = 29 // TODO|target = 35 // patched for sideload|' \
    "$work/cmd/fyne/internal/mobile/build.go"
  ( cd "$work" && go build -o "$PATCHED_FYNE" ./cmd/fyne )
  rm -rf "$work"
fi

# --- 2. Generate the signing keystore on first run ---------------------------
if [[ ! -f "$KEYSTORE" ]]; then
  echo ">> Creating keystore $KEYSTORE (keep it — updates must reuse the same key)"
  keytool -genkeypair -v \
    -keystore "$KEYSTORE" -alias "$KEY_ALIAS" \
    -keyalg RSA -keysize 2048 -validity 10000 \
    -storepass "$STORE_PASS" -keypass "$KEY_PASS" \
    -dname "CN=Deye Monitor, OU=Personal, O=Deye, C=UA"
fi

# --- 3. Package the APK ------------------------------------------------------
echo ">> Packaging $TARGET_OS APK..."
rm -f "$OUT_APK"
"$PATCHED_FYNE" package -os "$TARGET_OS" -app-id "$APP_ID" -icon "$ICON"

# --- 4. Align + re-sign with v1+v2+v3 ----------------------------------------
echo ">> Aligning and re-signing (v1+v2+v3)..."
aligned="$(mktemp -u).apk"
"$BT/zipalign" -p -f 4 "$OUT_APK" "$aligned"
"$BT/apksigner" sign \
  --ks "$KEYSTORE" --ks-pass "pass:$STORE_PASS" \
  --ks-key-alias "$KEY_ALIAS" --key-pass "pass:$KEY_PASS" \
  --out "$OUT_APK" "$aligned"
rm -f "$aligned" "$OUT_APK.idsig"

# --- 5. Report ---------------------------------------------------------------
echo ">> Done. APK metadata:"
"$BT/aapt" dump badging "$OUT_APK" 2>/dev/null | grep -E "package:|targetSdkVersion|native-code"
"$BT/apksigner" verify --print-certs "$OUT_APK" >/dev/null 2>&1 && echo "signature: v1+v2+v3 OK"
echo ">> Install with: adb install -r \"$OUT_APK\""
