SHELL=/bin/bash # Use bash syntax
DEFAULT_DEB_CONTROL_FILE?=DEBIAN/control
DEB_CONTROL_FILE?=${DEFAULT_DEB_CONTROL_FILE}
DIST=${PWD}/dist
LOMOD_PATH=./
LOMOD_PI=lomod_armhf
LOMOD_X86=lomod_amd64

clean:
	rm -rf ${DIST} *.deb

release-gen: clean
	for dir in bin etc var; do mkdir -p ${DIST}/opt/lomorage/$$dir; done
	cp ${LOMOD} ${DIST}/opt/lomorage/bin/lomod
	cp -r rpms-build/release/* ${DIST}/
	cp -r rpms-build/${DEB_CONTROL_FILE} ${DIST}/DEBIAN/control
	if [[ $$(uname -m) == "x86"* ]]; then \
	  sed -i "s/version-replace-me/$$(${LOMOD_PATH}${LOMOD_X86} -v)/g" ${DIST}/DEBIAN/control; \
	else \
	  sed -i "s/version-replace-me/$$(${LOMOD_PATH}${LOMOD_PI} -v)/g" ${DIST}/DEBIAN/control; \
	fi
	dpkg-deb --build dist

release-pi: DEB_CONTROL_FILE=control_pi
release-pi: LOMOD=${LOMOD_PI}
release-pi: clean release-gen
	mv dist.deb lomo-backend_armhf.deb

release-x86: DEB_CONTROL_FILE=control_x86
release-x86: LOMOD=${LOMOD_X86}
release-x86: clean release-gen
	mv dist.deb lomo-backend_amd64.deb
