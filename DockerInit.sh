#!/bin/sh
case $1 in
    amd64)
        ARCH="64"
        FNAME="amd64"
        MTG_ARCH="amd64"
        ;;
    i386)
        ARCH="32"
        FNAME="i386"
        MTG_ARCH="386"
        ;;
    armv8 | arm64 | aarch64)
        ARCH="arm64-v8a"
        FNAME="arm64"
        MTG_ARCH="arm64"
        ;;
    armv7 | arm | arm32)
        ARCH="arm32-v7a"
        FNAME="arm32"
        MTG_ARCH="armv7"
        ;;
    armv6)
        ARCH="arm32-v6"
        FNAME="armv6"
        MTG_ARCH="armv6"
        ;;
    *)
        ARCH="64"
        FNAME="amd64"
        MTG_ARCH="amd64"
        ;;
esac
MTG_VER="2.2.8"
mkdir -p build/bin
cd build/bin
XRAY_VERSION="$(../../scripts/resolve-xray-version.sh)" || {
    echo "Unable to resolve a stable Xray-core release; refusing to build a stale image." >&2
    exit 1
}
echo "Using stable Xray-core ${XRAY_VERSION}"
curl -sfLRO "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/Xray-linux-${ARCH}.zip"
unzip "Xray-linux-${ARCH}.zip"
rm -f "Xray-linux-${ARCH}.zip" geoip.dat geosite.dat
mv xray "xray-linux-${FNAME}"
curl -sfLRO "https://github.com/9seconds/mtg/releases/download/v${MTG_VER}/mtg-${MTG_VER}-linux-${MTG_ARCH}.tar.gz"
tar -xzf "mtg-${MTG_VER}-linux-${MTG_ARCH}.tar.gz"
mv "mtg-${MTG_VER}-linux-${MTG_ARCH}/mtg" "mtg-linux-${FNAME}" 2>/dev/null || mv mtg "mtg-linux-${FNAME}"
rm -rf "mtg-${MTG_VER}-linux-${MTG_ARCH}" "mtg-${MTG_VER}-linux-${MTG_ARCH}.tar.gz"
chmod +x "mtg-linux-${FNAME}"
curl -sfLRO https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat
curl -sfLRO https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat
curl -sfLRo geoip_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat
curl -sfLRo geosite_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat
curl -sfLRo geoip_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat
curl -sfLRo geosite_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat
cd ../../
