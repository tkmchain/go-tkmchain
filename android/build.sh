#!/usr/bin/env bash
set -euo pipefail

# Builds the gtkm Android app (node installed as a native executable) into
# app/build/outputs/apk/debug/app-debug.apk, ready to sideload.
#
# Requirements:
#   - Rust 1.89 with the aarch64-linux-android target
#   - This repo's Go toolchain with CGO (CMake for RandomX is also needed)
#   - Android SDK: cmdline-tools, platforms;android-34, build-tools;34.0.0
#   - Android NDK r25+ (aarch64 clang toolchain)
#   - JDK 17+ and Gradle >= 8.7 (AGP 8.5.2)
#
# Env (overridable):
#   ANDROID_HOME       SDK root                (default /opt/android-sdk)
#   ANDROID_NDK_HOME   NDK root                (default /opt/android-ndk/android-ndk-r27c)
#   GRADLE_BIN         gradle binary           (default /opt/gradle/gradle-8.9/bin/gradle)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ANDROID_HOME="${ANDROID_HOME:-/opt/android-sdk}"
ANDROID_NDK_HOME="${ANDROID_NDK_HOME:-/opt/android-ndk/android-ndk-r27c}"
GRADLE_BIN="${GRADLE_BIN:-/opt/gradle/gradle-8.9/bin/gradle}"

RX_SRC="$ROOT/build/_workspace/randomx"
RX_SRC_DIR="$RX_SRC/src"
RX_BUILD="$ROOT/build/_workspace/randomx/build-android-arm64"
NDK_API=24
NDK_BIN="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin"
APP="$SCRIPT_DIR/app/src/main/assets"

echo "==> Android SDK:  $ANDROID_HOME"
echo "==> Android NDK:  $ANDROID_NDK_HOME"
echo "==> Gradle:       $(which "$GRADLE_BIN" || echo "$GRADLE_BIN")"

if [ ! -d "$RX_SRC_DIR" ]; then
    echo "RandomX source not found at $RX_SRC_DIR; run: make randomx-host"
    exit 1
fi
if [ ! -x "$GRADLE_BIN" ]; then
    echo "Gradle not found at $GRADLE_BIN; set GRADLE_BIN"
    exit 1
fi

mkdir -p "$RX_BUILD" "$APP"

echo "==> Building RandomX for android-arm64"
cmake -S "$RX_SRC" -B "$RX_BUILD" \
    -DCMAKE_TOOLCHAIN_FILE="$ANDROID_NDK_HOME/build/cmake/android.toolchain.cmake" \
    -DANDROID_ABI=arm64-v8a \
    -DANDROID_PLATFORM="android-$NDK_API" \
    -DCMAKE_SYSTEM_PROCESSOR=aarch64 \
    -DARCH_ID=aarch64 -DARM_ID=aarch64 -DARCH=default \
    -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF >/dev/null
cmake --build "$RX_BUILD" --target randomx --parallel "$(nproc)" >/dev/null

echo "==> Building gtkm node for android-arm64"
export GOOS=android GOARCH=arm64 CGO_ENABLED=1
export CC="$NDK_BIN/aarch64-linux-android${NDK_API}-clang"
export CXX="$NDK_BIN/aarch64-linux-android${NDK_API}-clang++"
export CGO_CFLAGS="-I$RX_SRC_DIR"
export CGO_LDFLAGS="-L$RX_BUILD -lrandomx -static-libstdc++ -lm -ldl -llog"
SHIELD3_RUST_TARGET=aarch64-linux-android "$ROOT/scripts/shield3-build.sh"
(cd "$ROOT" && go build -tags "randomx,shield3,urfave_cli_no_docs" -o "$APP/gtkm" ./cmd/gtkm)
chmod 0700 "$APP/gtkm"
file --brief "$APP/gtkm"

echo "==> Building shielded-payout-prover for android-arm64"
unset CGO_CFLAGS
export CGO_LDFLAGS="-lm -ldl -llog"
(cd "$ROOT" && go build -tags "shield3,urfave_cli_no_docs" -o "$APP/shielded-payout-prover" ./cmd/shielded-payout-prover)
chmod 0700 "$APP/shielded-payout-prover"
file --brief "$APP/shielded-payout-prover"

echo "==> Copying libc++ runtime"
cp "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/sysroot/usr/lib/aarch64-linux-android/libc++_shared.so" "$APP/libc++_shared.so"

BUILD_VARIANT="${TKM_ANDROID_BUILD_VARIANT:-debug}"
case "$BUILD_VARIANT" in
  debug) GRADLE_TASK=assembleDebug ;;
  release) GRADLE_TASK=assembleRelease ;;
  *) echo "TKM_ANDROID_BUILD_VARIANT must be debug or release" >&2; exit 1 ;;
esac
echo "==> Building $BUILD_VARIANT APK"
printf 'sdk.dir=%s\n' "$ANDROID_HOME" > "$SCRIPT_DIR/local.properties"
"$GRADLE_BIN" --no-daemon -p "$SCRIPT_DIR" "$GRADLE_TASK"

APK="$SCRIPT_DIR/app/build/outputs/apk/$BUILD_VARIANT/app-$BUILD_VARIANT.apk"
echo
echo "APK: $APK"
echo "Install: adb install -r $APK   (or copy the file to your phone and open it)"
