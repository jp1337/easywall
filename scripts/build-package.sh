#!/bin/bash

set -e # exit when any command fails

SCRIPTSPATH=$(dirname "$(readlink -f "$0")")
HOMEPATH="$(dirname "$SCRIPTSPATH")"
BUILD_DIR="${HOMEPATH}/deb"
OUTPUT_DIR="${HOMEPATH}/artifacts"
VERSION=$(cat "${HOMEPATH}/.version")
PACKAGE="easywall"
ARCHITECTURE="amd64"
RELEASE="0"
PACKAGE_NAME="${PACKAGE}_${VERSION}-${RELEASE}_${ARCHITECTURE}.deb"

STEPS=7
STEP=1

# Step 1
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m remove build directory if present \\e[39m" && ((STEP++))
[ -d "${BUILD_DIR}" ] && rm -rfv "${BUILD_DIR}"

# Step 2
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m create build directory structure \\e[39m" && ((STEP++))
mkdir -pv "${BUILD_DIR}"
mkdir -pv "${BUILD_DIR}/opt/$PACKAGE"

# Step 3
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m copy all the files to the build directory \\e[39m" && ((STEP++))
cp -rav DEBIAN "${BUILD_DIR}"/
cp -rav easywall "${BUILD_DIR}"/opt/$PACKAGE/
mkdir -pv "${BUILD_DIR}/opt/$PACKAGE/config/"
cp -rav config/*.sample.ini "${BUILD_DIR}"/opt/$PACKAGE/config/
cp -rav docs "${BUILD_DIR}"/opt/$PACKAGE/
cp -rav scripts "${BUILD_DIR}"/opt/$PACKAGE/
cp -rav ssl "${BUILD_DIR}"/opt/$PACKAGE/
cp -rav .version "${BUILD_DIR}"/opt/$PACKAGE/
cp -rav requirements.txt "${BUILD_DIR}"/opt/$PACKAGE/
cp -rav setup* "${BUILD_DIR}"/opt/$PACKAGE/

# Step 4
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m clean up build folder before packaging \\e[39m" && ((STEP++))
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/__pycache__
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/__pycache__
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/static/css/*.min.css
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/static/css/font-awesome*
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/static/fonts/*.woff*
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/static/fonts/*.ttf*
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/static/fonts/*.eot*
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/static/fonts/*.svg*
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/static/fonts/*.otf*
rm -rv "${BUILD_DIR}"/opt/$PACKAGE/easywall/web/static/js/*.min.js

# Step 5
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m fix permissions in build directory \\e[39m" && ((STEP++))
chmod -v 755 "${BUILD_DIR}"/DEBIAN/postinst

# Step 6
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m build the debian project \\e[39m" && ((STEP++))
dpkg-deb --build "${BUILD_DIR}" "${PACKAGE_NAME}"
mkdir -pv "${OUTPUT_DIR}"
[ -f "${PACKAGE_NAME}" ] && mv "${PACKAGE_NAME}" "${OUTPUT_DIR}"

# Step 7
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m cleanup build directory \\e[39m" && ((STEP++))
rm -rfv "${BUILD_DIR}"
