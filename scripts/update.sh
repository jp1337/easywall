#!/bin/bash

CONFIGFOLDER="config"
RULESFOLDER="rules"
SSLFOLDER="ssl"
BACKUPFOLDER="backup"
WEB_CONFIG_FILE="web.ini"
CORE_CONFIG_FILE="easywall.ini"

SCRIPTNAME=$(basename "$0")
SCRIPTSPATH=$(dirname "$(readlink -f "$0")")
OLDPATH="$(dirname "${SCRIPTSPATH}")"
NEWPATH="$(dirname "${OLDPATH}")/$(basename "${OLDPATH}")-new"
BACKUPPATH="${OLDPATH}-backup"

GIT_VERSION=$(curl --silent "https://api.github.com/repos/jp1337/easywall/releases/latest" | grep '"tag_name":' | sed -E 's/.*"v([0-9\.]+)".*/\1/')
GIT_URL="https://github.com/jp1337/easywall/archive/v${GIT_VERSION}.zip"
GIT_FILE="easywall-${GIT_VERSION}.zip"
ZIP_FOLDER="easywall-${GIT_VERSION}"

STEPS=10
STEP=1

if [ "$EUID" -ne 0 ]; then
    read -r -d '' NOROOT <<EOF
To update easywall, you need administration rights.
You can use the following commands:

# sudo -H bash ${SCRIPTSPATH}/${SCRIPTNAME}
or
# su root -c "${SCRIPTSPATH}/${SCRIPTNAME}"
EOF
    echo "$NOROOT"
    exit 1
fi

# Step 1
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Check whether the requirements for the update are met \\e[39m" && ((STEP++))
apt -qqq update
apt -y install wget unzip
rm -rf "${NEWPATH}"

# Step 2
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Download the new program version into a new folder \\e[39m" && ((STEP++))
mkdir -p "${NEWPATH}"
wget -q --timeout=10 --tries=5 --retry-connrefused --show-progress -P "${NEWPATH}" "${GIT_URL}"

# Step 3
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Unpack the new program version \\e[39m" && ((STEP++))
unzip "${NEWPATH}/${GIT_FILE}" -d "${NEWPATH}"

# Step 4
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Move the unzipped files into the correct directory \\e[39m" && ((STEP++))
shopt -s dotglob nullglob
mv -v "${NEWPATH}"/"${ZIP_FOLDER}"/* "${NEWPATH}"

# Step 5
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Clean up the new installation directory \\e[39m" && ((STEP++))
rm -rv "${NEWPATH:?}/${ZIP_FOLDER}"
rm -v "${NEWPATH:?}/${GIT_FILE}"

# Step 6
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Copy the configuration from the old installation to the new installation \\e[39m" && ((STEP++))
cp -av "${OLDPATH}"/${CONFIGFOLDER}/${WEB_CONFIG_FILE} "${NEWPATH}/${CONFIGFOLDER}/"
cp -av "${OLDPATH}"/${CONFIGFOLDER}/${CORE_CONFIG_FILE} "${NEWPATH}/${CONFIGFOLDER}/"
mkdir -p "${NEWPATH}/${BACKUPFOLDER}/"
cp -av "${OLDPATH}"/${BACKUPFOLDER}/* "${NEWPATH}/${BACKUPFOLDER}/"
mkdir -p "${NEWPATH}/${RULESFOLDER}/"
cp -avr "${OLDPATH}"/${RULESFOLDER}/* "${NEWPATH}/${RULESFOLDER}/"
mkdir -p "${NEWPATH}/${SSLFOLDER}/"
cp -av "${OLDPATH}"/${SSLFOLDER}/* "${NEWPATH}/${SSLFOLDER}/"

# Step 7
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m the services are stopped \\e[39m" && ((STEP++))
systemctl --no-pager stop easywall
systemctl --no-pager stop easywall-web

# Step 8
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m move previous installation into a backup directory \\e[39m" && ((STEP++))
cp -arv "${OLDPATH}" "${BACKUPPATH}"
rm -rv "${OLDPATH}"

# Step 9
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m move new installation to the productive directory \\e[39m" && ((STEP++))
mv -v "${NEWPATH}" "${OLDPATH}"

# Step 10
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Execute the installation scripts of the new installation \\e[39m" && ((STEP++))
/bin/bash "${OLDPATH}"/scripts/install-core.sh
/bin/bash "${OLDPATH}"/scripts/install-web.sh
