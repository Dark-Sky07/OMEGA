#!/bin/sh
# BuildKit supplies TARGETARCH as "arm" for both arm/v6 and arm/v7. The
# variant is therefore part of the selection; falling back to amd64 would
# publish an executable that cannot run on 386/ARM images.
TARGET_ARCH="${1:-}"
TARGET_VARIANT="${2:-}"
MTG_SUPPORTED="1"
case "${TARGET_ARCH}/${TARGET_VARIANT}" in
    386/*|i386/*)
        ARCH="32"
        XRAY_NAME="386"
        MTG_NAME="386"
        MTG_ARCH="386"
        ;;
    amd64/*|x86_64/*)
        ARCH="64"
        XRAY_NAME="amd64"
        MTG_NAME="amd64"
        MTG_ARCH="amd64"
        ;;
    arm64/*|aarch64/*|arm64/v8)
        ARCH="arm64-v8a"
        XRAY_NAME="arm64"
        MTG_NAME="arm64"
        MTG_ARCH="arm64"
        ;;
    arm/v6|armv6/*|armv6)
        ARCH="arm32-v6"
        XRAY_NAME="arm32"
        MTG_NAME="arm"
        MTG_ARCH="armv6"
        ;;
    arm/v5|armv5/*|armv5)
        ARCH="arm32-v5"
        XRAY_NAME="arm32"
        # mtg does not publish an ARMv5 artifact. Keep the image build
        # usable; MTProto is unavailable on this legacy target.
        MTG_SUPPORTED="0"
        ;;
    arm/v7|arm/*|arm|armv7/*|armv7|arm32/*|arm32)
        ARCH="arm32-v7a"
        XRAY_NAME="arm32"
        MTG_NAME="arm"
        MTG_ARCH="armv7"
        ;;
    *)
        echo "Unsupported Docker target architecture: ${TARGET_ARCH}/${TARGET_VARIANT}" >&2
        exit 1
        ;;
esac
MTG_VER="2.2.8"
mkdir -p build/bin
cd build/bin
XRAY_VERSION="$(../../scripts/resolve-xray-version.sh)" || {
    echo "Unable to resolve the latest Xray-core release; refusing to build a stale image." >&2
    exit 1
}
echo "Using latest Xray-core ${XRAY_VERSION}"
curl -sfLRO "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/Xray-linux-${ARCH}.zip"
unzip "Xray-linux-${ARCH}.zip"
rm -f "Xray-linux-${ARCH}.zip" geoip.dat geosite.dat
mv xray "xray-linux-${XRAY_NAME}"
if [ "${MTG_SUPPORTED}" = "1" ]; then
    curl -sfLRO "https://github.com/9seconds/mtg/releases/download/v${MTG_VER}/mtg-${MTG_VER}-linux-${MTG_ARCH}.tar.gz"
    tar -xzf "mtg-${MTG_VER}-linux-${MTG_ARCH}.tar.gz"
    mv "mtg-${MTG_VER}-linux-${MTG_ARCH}/mtg" "mtg-linux-${MTG_NAME}" 2>/dev/null || mv mtg "mtg-linux-${MTG_NAME}"
    rm -rf "mtg-${MTG_VER}-linux-${MTG_ARCH}" "mtg-${MTG_VER}-linux-${MTG_ARCH}.tar.gz"
    chmod +x "mtg-linux-${MTG_NAME}"
else
    echo "Skipping mtg: no ${TARGET_ARCH}/${TARGET_VARIANT} artifact is published"
fi
curl -sfLRO https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat
curl -sfLRO https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat
curl -sfLRo geoip_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat
curl -sfLRo geosite_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat
curl -sfLRo geoip_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat
curl -sfLRo geosite_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat
cd ../../
