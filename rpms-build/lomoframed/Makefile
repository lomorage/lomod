SHELL=/bin/bash # Use bash syntax
DEFAULT_DEB_CONTROL_FILE?=DEBIAN/control
DEB_CONTROL_FILE?=${DEFAULT_DEB_CONTROL_FILE}
DIST=${PWD}/dist
LOMOFRAMED_PATH=./
LOMOFRAMED_PI=lomoframed_armhf
LOMOFRAMED_PI_ARM64=lomoframed_arm64
LOMOFRAMED_X86=lomoframed_amd64

clean:
	rm -rf ${DIST} *.deb

release-gen:
	for dir in bin etc var; do mkdir -p ${DIST}/opt/lomorage/$$dir; done
	cp ${LOMOFRAMED} ${DIST}/opt/lomorage/bin/lomoframed
	cp -r release/* ${DIST}/
	cp -r ${DEB_CONTROL_FILE} ${DIST}/DEBIAN/control
	if [[ $$(uname -m) == "x86"* ]]; then \
	  sed -i "s/version-replace-me/$$(${LOMOFRAMED_PATH}${LOMOFRAMED_X86} -v)/g" ${DIST}/DEBIAN/control; \
	elif [[ $$(uname -m) == "aarch64"* ]]; then \
	  sed -i "s/version-replace-me/$$(${LOMOFRAMED_PATH}${LOMOFRAMED_PI_ARM64} -v)/g" ${DIST}/DEBIAN/control; \
	else \
	  sed -i "s/version-replace-me/$$(${LOMOFRAMED_PATH}${LOMOFRAMED_PI} -v)/g" ${DIST}/DEBIAN/control; \
	fi
	dpkg-deb --build dist

release-pi: DEB_CONTROL_FILE=control_pi
release-pi: LOMOFRAMED=${LOMOFRAMED_PI}
release-pi: release-gen
	mv dist.deb lomo-framed_armhf.deb

release-pi-arm64: DEB_CONTROL_FILE=control_pi_arm64
release-pi-arm64: LOMOFRAMED=${LOMOFRAMED_PI_ARM64}
release-pi-arm64: release-gen
	mv dist.deb lomo-framed_arm64.deb

release-x86: DEB_CONTROL_FILE=control_x86
release-x86: LOMOFRAMED=${LOMOFRAMED_X86}
release-x86: release-gen
	mv dist.deb lomo-framed_amd64.deb
