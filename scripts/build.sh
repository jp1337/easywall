#!/bin/bash

BUILD_DIR="deb"
OUTPUT_DIR="artifacts"
PACKAGE="easywall"
VERSION="0.2.0"

# remove build directory if present
[ -d $BUILD_DIR ] && rm -rf $BUILD_DIR

# create build directory structure
mkdir -p $BUILD_DIR
mkdir -p $BUILD_DIR/opt/$PACKAGE
mkdir -p $OUTPUT_DIR

# copy all the files to the build directory
cp -ra DEBIAN $BUILD_DIR/
cp -ra easywall $BUILD_DIR/opt/$PACKAGE/
cp -ra config $BUILD_DIR/opt/$PACKAGE/
cp -ra docs $BUILD_DIR/opt/$PACKAGE/
cp -ra scripts $BUILD_DIR/opt/$PACKAGE/
cp -ra ssl $BUILD_DIR/opt/$PACKAGE/
cp -ra .version $BUILD_DIR/opt/$PACKAGE/
cp -ra requirements.txt $BUILD_DIR/opt/$PACKAGE/
cp -ra setup* $BUILD_DIR/opt/$PACKAGE/

# build the debian project
PACKAGE_NAME="${PACKAGE}_${VERSION}.deb"
dpkg-deb --build $BUILD_DIR $PACKAGE_NAME
mv $PACKAGE_NAME $OUTPUT_DIR

# cleanup
rm -rf $BUILD_DIR
