.PHONY: vendor

VERSION_COMPILE := -ldflags "-X bitbucket.org/lomoware/lomo-backend/common/release.Version=$$(cat LATEST_RELEASE)"

SHELL=/bin/bash # Use bash syntax
DEFAULT_LOMOD_BIN_DIR=/opt/lomorage/bin
DEFAULT_DEB_CONTROL_FILE?=DEBIAN/control
DEB_CONTROL_FILE?=${DEFAULT_DEB_CONTROL_FILE}
DIST=${PWD}/dist
PREFIX_BASE=lomorage/base
PREFIX_LOMO_VIPS=lomorage/vips
PREFIX_LOMOD=lomorage/lomod
SUFFIX_BUILD=build
SUFFIX_TEST=test
SUFFIX_META=meta
DOCKER_FILE=Dockerfile
MAKE_TEST_ARGS=test-all
SRC_DIR=/root/src/bitbucket.org/lomoware/lomo-backend
WINDOWS_MINGW_BIN?=/c/mingw64/bin
WINDOWS_PKGCONFIG_BIN?=/c/pkgconfig/bin
WINDOWS_DEPS_DIR?=${PWD}/windows-deps
WINDOWS_VIPS_DIR?=${WINDOWS_DEPS_DIR}/vips
WINDOWS_DIST=${PWD}/dist-windows
# Release archives go here, next to the Linux packages, not in the repo root.
RELEASES_WINDOWS=${PWD}/releases/windows
RELEASES_MACOS=${PWD}/releases/macos
MACOS_DIST=${PWD}/dist-macos
MACOS_DIST_AMD64=${PWD}/dist-macos-amd64
MACOS_ARCH:=$(shell uname -m | sed 's/x86_64/amd64/')
# Codesign identity for release-macos/release-macos-amd64 -- e.g.
# "Developer ID Application: WTAO LLC (3GRDMJ5JMM)" (must already be in the login keychain,
# paired with its private key; `security find-identity -v -p codesigning` lists valid ones).
# Left unset (no env var, no .macos-sign-identity file below), both targets fall back to ad-hoc
# signing (`codesign --sign -`) -- fine for local testing, but ad-hoc-signed binaries fail
# Apple notarization outright and still trip Gatekeeper's "unidentified developer" warning for
# anyone who downloads them via a browser. Easy to hit by accident: an ad-hoc build still
# *looks* successful (codesign --verify passes, the tarball builds fine) right up until
# `xcrun notarytool submit` rejects it, which is a slow, easy-to-miss way to notice a forgotten
# --sign-identity/MACOS_SIGN_IDENTITY.
#
# Defaults from .macos-sign-identity (gitignored -- a different build machine would have a
# different cert) if present, so this doesn't need to be typed/remembered per invocation on a
# machine that's already set up for it; `echo -n "Developer ID Application: ..." >
# .macos-sign-identity` once to populate it. An explicit `make ... MACOS_SIGN_IDENTITY=...` or
# exported env var still overrides the file.
MACOS_SIGN_IDENTITY?=$(shell cat .macos-sign-identity 2>/dev/null)


release-version:
	echo -n "$$(date +%Y-%m-%d.%H-%M-%S).0.$$(git rev-parse --short=7 HEAD)" >LATEST_RELEASE

vendor:
	go get github.com/LK4D4/vndr
	vndr

systemd:
	sudo mkdir -p /var/lib/lomo
	sudo chown pi:pi /var/lib/lomo
	sudo cp ./template/lomod.service /lib/systemd/system
	sudo systemctl enable lomod
	sqlite3 /var/lib/lomo/assets.db < ./migrations/sqls/0.sql

install:
	CGO_ENABLED=1 go build -mod=vendor -v -tags "sqlite_trace trace" -o ${DEFAULT_LOMOD_BIN_DIR}/lomod ./cmd/lomod
	CGO_ENABLED=1 go build -mod=vendor -v -tags "sqlite_trace trace" -o ${DEFAULT_LOMOD_BIN_DIR}/lomoc ./cmd/lomoc

check:
	golangci-lint run -v --max-same-issues 10 --build-tags "sqlite_trace trace" --skip-dirs common/cast

test-simple:
	go list ./... | grep -vE 'api|cmd|handler|vendor' | (CGO_ENABLED=1 xargs -I{} go test -mod=vendor -v -tags "sqlite_trace trace" "{}" -timeout 30m -check.vv )

test-api-install: DEFAULT_LOMOD_BIN_DIR=/opt/lomorage/apitest/bin
test-api-install: install

test-api: test-api-install
	cd api/test/lomod; CGO_ENABLED=1 go test -mod=vendor -v -tags "sqlite_trace trace" -timeout 30m -check.vv

test-handler:
	cd handler; CGO_ENABLED=1 CGO_CFLAGS_ALLOW=-Xpreprocessor go test -mod=vendor -v -tags "sqlite_trace trace" -timeout 60m -check.vv

test-all: test-simple test-api-install test-handler

clean:
	rm -rf ${DIST} *.deb

build: build-lomod build-lomocloud build-lomoframed build-lomoc

build-rice:
	cd ./handler && rice embed-go

build-lomocloud:
	GOBIN=${PWD}/cmd/lomocloud go install $(VERSION_COMPILE) -mod=vendor -tags "sqlite_trace trace" ./cmd/lomocloud

build-lomod: release-version build-rice
	GOBIN=${PWD}/cmd/lomod CGO_ENABLED=1 CGO_CFLAGS_ALLOW="-Xpreprocessor" go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomod

build-lomod-mac: release-version build-rice
	CGO_CFLAGS=-mmacosx-version-min=10.11 CGO_LDFLAGS=-mmacosx-version-min=10.11 GOBIN="${PWD}/cmd/lomod" CGO_ENABLED=1 CGO_CFLAGS_ALLOW="-Xpreprocessor" go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomod

# Cross-builds lomod for amd64 from an Apple Silicon host. CGO still needs real x86_64
# .dylibs to link against -- Go's compiler can target amd64 directly (no emulation needed for
# codegen: `clang -arch x86_64` is natively supported by Xcode on Apple Silicon), but vips
# itself can't be conjured for the other architecture, so this expects a second, parallel
# x86_64 Homebrew installed under /usr/local via Rosetta (`arch -x86_64 brew install vips
# ffmpeg exiftool pkg-config`) -- Homebrew bottles only relocate cleanly to /opt/homebrew or
# /usr/local, so that's the practical prefix rather than an arbitrary one. Just pointing
# PKG_CONFIG at the x86_64 pkg-config binary is enough (and deliberately does NOT also set
# PKG_CONFIG_LIBDIR/PKG_CONFIG_PATH): that binary's own compiled-in default search path is
# already both correctly scoped to /usr/local (no risk of picking up the arm64 vips.pc under
# /opt/homebrew) and includes Homebrew's version-pinned libffi.pc stub for the current macOS
# SDK (glib needs it transitively; it isn't a real installable formula, just a stub pointing
# at Apple's system libffi) -- overriding the search path ourselves, as an earlier version of
# this target did via PKG_CONFIG_LIBDIR, silently drops that stub and breaks the build with
# "Package libffi was not found in the pkg-config search path".
# Uses `go build -o`, not `go install`: `go install` refuses to honor GOBIN when GOARCH
# differs from the host's, which it does here (host is arm64).
build-lomod-mac-amd64: release-version build-rice
	PKG_CONFIG="/usr/local/bin/pkg-config" \
	CC="clang -arch x86_64" CGO_CFLAGS=-mmacosx-version-min=10.13 CGO_LDFLAGS=-mmacosx-version-min=10.13 \
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CGO_CFLAGS_ALLOW="-Xpreprocessor" \
	go build $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" -o "${PWD}/cmd/lomod/lomod-amd64" ./cmd/lomod

build-lomoupg-mac-amd64: release-version
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 \
	go build $(VERSION_COMPILE) -mod=vendor -v -o "${PWD}/cmd/lomoupg/lomoupg-amd64" ./cmd/lomoupg

fetch-vips-windows:
	PATH="${WINDOWS_MINGW_BIN}:$$PATH" powershell -NoProfile -ExecutionPolicy Bypass -File scripts/windows/fetch-vips.ps1 -VipsDir "${WINDOWS_VIPS_DIR}"

# build-lomod-windows expects a mingw64 gcc toolchain on WINDOWS_MINGW_BIN and a pkg-config.exe
# on WINDOWS_PKGCONFIG_BIN (e.g. pkg-config-lite: https://sourceforge.net/projects/pkgconfiglite/);
# fetch-vips-windows stages the vips-dev tree (pkgconfig + headers + dlls) under WINDOWS_VIPS_DIR first.
# Must run on a real Windows host: CGO cross-compilation against libvips from Linux/Mac is not supported.
build-lomod-windows: release-version build-rice fetch-vips-windows
	PATH="${WINDOWS_MINGW_BIN}:${WINDOWS_PKGCONFIG_BIN}:$$PATH" PKG_CONFIG_PATH="${WINDOWS_VIPS_DIR}/lib/pkgconfig" \
	GOOS=windows GOARCH=amd64 GOBIN="${PWD}/cmd/lomod" CGO_ENABLED=1 CGO_CFLAGS_ALLOW="-Xpreprocessor" \
	go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomod

# lomoupg has no CGO dependency, so it builds the same way on any host OS.
build-lomoupg-windows: release-version
	GOOS=windows GOARCH=amd64 GOBIN="${PWD}/cmd/lomoupg" CGO_ENABLED=0 \
	go install $(VERSION_COMPILE) -mod=vendor -v ./cmd/lomoupg

# lomoc needs the same mingw64+vips toolchain as lomod (cmd/lomoc/migrate.go imports govips
# unconditionally, even though the scan/import commands this is mainly bundled for don't touch
# vips themselves) -- reuses build-lomod-windows's fetch-vips-windows output rather than staging
# its own copy.
build-lomoc-windows: release-version fetch-vips-windows
	PATH="${WINDOWS_MINGW_BIN}:${WINDOWS_PKGCONFIG_BIN}:$$PATH" PKG_CONFIG_PATH="${WINDOWS_VIPS_DIR}/lib/pkgconfig" \
	GOOS=windows GOARCH=amd64 GOBIN="${PWD}/cmd/lomoc" CGO_ENABLED=1 CGO_CFLAGS_ALLOW="-Xpreprocessor" \
	go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomoc

build-lomoc: release-version
	GOBIN=${PWD}/cmd/lomoc CGO_ENABLED=1 go install $(VERSION_COMPILE) -mod=vendor -tags "sqlite_trace trace" ./cmd/lomoc

build-lomoc-mac: release-version
	CGO_CFLAGS=-mmacosx-version-min=10.11 CGO_LDFLAGS=-mmacosx-version-min=10.11 GOBIN=${PWD}/cmd/lomoc CGO_ENABLED=1 go install $(VERSION_COMPILE) -mod=vendor -tags "sqlite_trace trace" ./cmd/lomoc

# lomoupg has no CGO dependency, so it builds the same way on any host OS (this still just
# builds natively on whatever host runs `make release-macos` -- unlike Windows, there is no
# cross-compilation path in this Makefile for macOS CGO builds, so that host must be a real Mac).
build-lomoupg-mac: release-version
	GOBIN=${PWD}/cmd/lomoupg CGO_ENABLED=0 \
	go install $(VERSION_COMPILE) -mod=vendor -v ./cmd/lomoupg

build-lomoframed: release-version
	GOBIN=${PWD}/cmd/lomoframed CGO_ENABLED=1 go install $(VERSION_COMPILE) -mod=vendor -tags "sqlite_trace trace" ./cmd/lomoframed

docker-release: clean build-rice
	echo -n "$$(date +%Y-%m-%d.%H-%M-%S).0.$$(git rev-parse --short=7 HEAD)" >LATEST_RELEASE
	mkdir -p ${DIST}
	cp -r rpms-build/lomod-docker/* ${DIST}/
	sed -i "s/version-replace-me/$$(cat LATEST_RELEASE)/g" ${DIST}/DEBIAN/control
	sed -i "s/lomo-backend/lomo-backend-docker/g" ${DIST}/DEBIAN/control
	ls migrations/sqls/lomod | grep -cE '^[0-9]+\.go$$' > ${DIST}/DEBIAN/schema-version
	mkdir -p ${DIST}/opt/lomorage/var
	mkdir -p ${DIST}/opt/lomorage/bin
	GOBIN=${DIST}/opt/lomorage/bin/ go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomod
	GOBIN=${DIST}/opt/lomorage/bin/ go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomoc
	cp LATEST_RELEASE ${DIST}/opt/lomorage/LATEST_RELEASE
	if [[ $$(uname -m) == "x86"* ]]; then \
	  sed -i "s/arch-replace-me/amd64/g" ${DIST}/DEBIAN/control; \
	  dpkg-deb --build dist; \
	  mv dist.deb lomo-backend-docker_amd64.deb; \
	elif [[ $$(uname -m) == "aarch64"* ]]; then \
	  sed -i "s/arch-replace-me/arm64/g" ${DIST}/DEBIAN/control; \
	  dpkg-deb --build dist; \
	  mv dist.deb lomo-backend-docker_arm64.deb; \
	else \
	  sed -i "s/arch-replace-me/armhf/g" ${DIST}/DEBIAN/control; \
	  dpkg-deb --build dist; \
	  mv dist.deb lomo-backend-docker_armhf.deb; \
	fi

release-gen: clean
	echo -n "$$(date +%Y-%m-%d.%H-%M-%S).0.$$(git rev-parse --short=7 HEAD)" >LATEST_RELEASE
	mkdir -p ${DIST}
	cp -r rpms-build/${TARGET_DIR}/* ${DIST}/
	if [ ${DEB_CONTROL_FILE} != ${DEFAULT_DEB_CONTROL_FILE} ]; then \
	  mv ${DIST}/${DEB_CONTROL_FILE} ${DIST}/${DEFAULT_DEB_CONTROL_FILE}; \
	fi
	sed -i "s/version-replace-me/$$(cat LATEST_RELEASE)/g" ${DIST}/DEBIAN/control
	mkdir -p ${DIST}/opt/lomorage/var
	mkdir -p ${DIST}/opt/lomorage/bin
	for dir in ${BIN_DIR}; \
	do \
	  GOBIN=${DIST}/opt/lomorage/bin/ go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/$$dir; \
	done
	cp LATEST_RELEASE ${DIST}/opt/lomorage/LATEST_RELEASE.${TARGET_DIR}
	dpkg-deb --build dist

release-lomod: clean build-rice
	echo -n "$$(date +%Y-%m-%d.%H-%M-%S).0.$$(git rev-parse --short=7 HEAD)" >LATEST_RELEASE
	cd ${PWD}/rpms-build/lomod; make clean
	mkdir -p ${PWD}/rpms-build/lomod/dist/opt/lomorage/bin
	GOBIN=${PWD}/rpms-build/lomod/dist/opt/lomorage/bin go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomoc
	GOBIN=${PWD}/rpms-build/lomod/ go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomod
	if [[ $$(uname -m) == "x86"* ]]; then \
	  cd rpms-build/lomod; mv lomod lomod_amd64; make release-x86; mv lomo-backend_amd64.deb ../../; \
	elif [[ $$(uname -m) == "aarch64"* ]]; then \
	  cd rpms-build/lomod; mv lomod lomod_arm64; make release-pi-arm64; mv lomo-backend_arm64.deb ../../; \
	else \
	  cd rpms-build/lomod; mv lomod lomod_armhf; make release-pi; mv lomo-backend_armhf.deb ../../; \
	fi

release-windows: clean build-lomod-windows build-lomoupg-windows build-lomoc-windows
	rm -rf ${WINDOWS_DIST}
	mkdir -p ${WINDOWS_DIST}
	cp cmd/lomod/lomod.exe ${WINDOWS_DIST}/
	cp cmd/lomoupg/lomoupg.exe ${WINDOWS_DIST}/
	cp cmd/lomoc/lomoc.exe ${WINDOWS_DIST}/
	cp installers/windows/lomorage-start.bat installers/windows/lomorage-start-hidden.vbs installers/windows/lomorage-stop.bat installers/windows/lomorage-tray.ps1 installers/windows/lomorage-post-update.bat installers/windows/lomorage-update.ps1 ${WINDOWS_DIST}/
	cp installers/windows/lomorage.ico ${WINDOWS_DIST}/lomorage.ico
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/windows/collect-deps.ps1 -DestDir "${WINDOWS_DIST}" -VipsDir "${WINDOWS_VIPS_DIR}"
	mkdir -p ${RELEASES_WINDOWS}
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/windows/make-zip.ps1 -SourceDir "${WINDOWS_DIST}" -ZipPath "${RELEASES_WINDOWS}/lomorage-windows-amd64-$$(cat LATEST_RELEASE).zip"
	cd ${RELEASES_WINDOWS} && sha256sum lomorage-windows-amd64-$$(cat ${PWD}/LATEST_RELEASE).zip > lomorage-windows-amd64-$$(cat ${PWD}/LATEST_RELEASE).zip.sha256
	@echo "Built ${RELEASES_WINDOWS}/lomorage-windows-amd64-$$(cat LATEST_RELEASE).zip"

# Builds lomod+lomoupg for macOS and bundles them with a self-contained copy of their
# Homebrew-installed vips/ffmpeg/exiftool dependencies (see scripts/macos/collect-deps.sh --
# requires `brew install vips ffmpeg exiftool dylibbundler` on the build host first). Must run
# on a real Mac of the target architecture: CGO cross-compilation against libvips from another
# host is not supported, and this doesn't produce a universal (arm64+amd64) binary.
release-macos: clean build-lomod-mac build-lomoupg-mac
	rm -rf ${MACOS_DIST}
	mkdir -p ${MACOS_DIST}
	cp cmd/lomod/lomod ${MACOS_DIST}/
	cp cmd/lomoupg/lomoupg ${MACOS_DIST}/
	cp installers/macos/lomorage-start.sh installers/macos/lomorage-stop.sh installers/macos/lomorage-update.sh ${MACOS_DIST}/
	sips -c 824 824 installers/macos/AppIcon-1024.png --out ${MACOS_DIST}/lomorage-tray.png >/dev/null && sips -z 36 36 ${MACOS_DIST}/lomorage-tray.png >/dev/null
	chmod +x ${MACOS_DIST}/lomod ${MACOS_DIST}/lomoupg ${MACOS_DIST}/lomorage-start.sh ${MACOS_DIST}/lomorage-stop.sh ${MACOS_DIST}/lomorage-update.sh
	bash scripts/macos/build-app-bundle.sh --dest-dir "${MACOS_DIST}" --sign-identity "${MACOS_SIGN_IDENTITY}"
	bash scripts/macos/collect-deps.sh --dest-dir "${MACOS_DIST}" --binary "${MACOS_DIST}/lomod" --sign-identity "${MACOS_SIGN_IDENTITY}"
	if [ -n "${MACOS_SIGN_IDENTITY}" ]; then \
	  codesign --force --options runtime --timestamp --sign "${MACOS_SIGN_IDENTITY}" ${MACOS_DIST}/lomoupg; \
	else \
	  codesign --force --sign - ${MACOS_DIST}/lomoupg; \
	fi
	mkdir -p ${RELEASES_MACOS}
	tar -C ${MACOS_DIST} -czf ${RELEASES_MACOS}/lomorage-macos-${MACOS_ARCH}-$$(cat LATEST_RELEASE).tar.gz .
	cd ${RELEASES_MACOS} && shasum -a 256 lomorage-macos-${MACOS_ARCH}-$$(cat ${PWD}/LATEST_RELEASE).tar.gz > lomorage-macos-${MACOS_ARCH}-$$(cat ${PWD}/LATEST_RELEASE).tar.gz.sha256
	@echo "Built ${RELEASES_MACOS}/lomorage-macos-${MACOS_ARCH}-$$(cat LATEST_RELEASE).tar.gz"

# Same as release-macos, but cross-built for amd64 (see build-lomod-mac-amd64) -- meant to be
# run on the same Apple Silicon host as release-macos, not on a real Intel Mac. Prepends
# /usr/local/bin to PATH so collect-deps.sh's `brew --prefix ffmpeg` / `brew --cellar exiftool`
# calls resolve to the x86_64 Homebrew rather than the arm64 one at /opt/homebrew; dylibbundler
# itself is left to resolve from /opt/homebrew since it only edits Mach-O load commands via
# otool/install_name_tool (architecture-agnostic host tools) and never executes the target file.
release-macos-amd64: clean build-lomod-mac-amd64 build-lomoupg-mac-amd64
	rm -rf ${MACOS_DIST_AMD64}
	mkdir -p ${MACOS_DIST_AMD64}
	cp cmd/lomod/lomod-amd64 ${MACOS_DIST_AMD64}/lomod
	cp cmd/lomoupg/lomoupg-amd64 ${MACOS_DIST_AMD64}/lomoupg
	cp installers/macos/lomorage-start.sh installers/macos/lomorage-stop.sh installers/macos/lomorage-update.sh ${MACOS_DIST_AMD64}/
	sips -c 824 824 installers/macos/AppIcon-1024.png --out ${MACOS_DIST_AMD64}/lomorage-tray.png >/dev/null && sips -z 36 36 ${MACOS_DIST_AMD64}/lomorage-tray.png >/dev/null
	chmod +x ${MACOS_DIST_AMD64}/lomod ${MACOS_DIST_AMD64}/lomoupg ${MACOS_DIST_AMD64}/lomorage-start.sh ${MACOS_DIST_AMD64}/lomorage-stop.sh ${MACOS_DIST_AMD64}/lomorage-update.sh
	bash scripts/macos/build-app-bundle.sh --dest-dir "${MACOS_DIST_AMD64}" --sign-identity "${MACOS_SIGN_IDENTITY}"
	PATH="/usr/local/bin:$$PATH" bash scripts/macos/collect-deps.sh --dest-dir "${MACOS_DIST_AMD64}" --binary "${MACOS_DIST_AMD64}/lomod" --sign-identity "${MACOS_SIGN_IDENTITY}"
	if [ -n "${MACOS_SIGN_IDENTITY}" ]; then \
	  codesign --force --options runtime --timestamp --sign "${MACOS_SIGN_IDENTITY}" ${MACOS_DIST_AMD64}/lomoupg; \
	else \
	  codesign --force --sign - ${MACOS_DIST_AMD64}/lomoupg; \
	fi
	mkdir -p ${RELEASES_MACOS}
	tar -C ${MACOS_DIST_AMD64} -czf ${RELEASES_MACOS}/lomorage-macos-amd64-$$(cat LATEST_RELEASE).tar.gz .
	cd ${RELEASES_MACOS} && shasum -a 256 lomorage-macos-amd64-$$(cat ${PWD}/LATEST_RELEASE).tar.gz > lomorage-macos-amd64-$$(cat ${PWD}/LATEST_RELEASE).tar.gz.sha256
	@echo "Built ${RELEASES_MACOS}/lomorage-macos-amd64-$$(cat LATEST_RELEASE).tar.gz"

release-lomoframed: clean
	cd ${PWD}/rpms-build/lomoframed; make clean
	mkdir -p ${PWD}/rpms-build/lomoframed/dist/opt/lomorage/bin
	GOBIN=${PWD}/rpms-build/lomoframed/ go install $(VERSION_COMPILE) -mod=vendor -v -tags "sqlite_trace trace" ./cmd/lomoframed
	if [[ $$(uname -m) == "x86"* ]]; then \
	  cd rpms-build/lomoframed; mv lomoframed lomoframed_amd64; make release-x86; mv lomo-framed_amd64.deb ../../; \
	elif [[ $$(uname -m) == "aarch64"* ]]; then \
	  cd rpms-build/lomoframed; mv lomoframed lomoframed_arm64; make release-pi-arm64; mv lomo-framed_arm64.deb ../../; \
	else \
	  cd rpms-build/lomoframed; mv lomoframed lomoframed_armhf; make release-pi; mv lomo-framed_armhf.deb ../../; \
	fi

release-lomocloud-amd64: TARGET_DIR=lomocloud
release-lomocloud-amd64: BIN_DIR=lomocloud
release-lomocloud-amd64: clean release-gen
	mv dist.deb lomo-cloud.deb

release-lomocloud-arm64: TARGET_DIR=lomocloud
release-lomocloud-arm64: BIN_DIR=lomocloud
release-lomocloud-arm64: DEB_CONTROL_FILE=DEBIAN/control.arm64
release-lomocloud-arm64: clean release-gen
	mv dist.deb lomo-cloud.deb

release-lomocloud-armhf: TARGET_DIR=lomocloud
release-lomocloud-armhf: BIN_DIR=lomocloud
release-lomocloud-armhf: DEB_CONTROL_FILE=DEBIAN/control.armhf
release-lomocloud-armhf: clean release-gen
	mv dist.deb lomo-cloud.deb

dev-container:
	docker build --tag "lomorage/dev-image:1.3" -f dockerfiles/dev-image .

dev:
	docker build --tag "lomorage/dev-run" -f dockerfiles/dev-run .
	docker rm -f lomod-dev || :
	docker run \
		-u pi \
		--name lomod-dev --hostname lomod-dev \
		--privileged --cap-add=ALL -v /dev:/dev -v /lib/modules:/lib/modules \
		-v "${PWD}:/go/src/bitbucket.org/lomoware/lomo-backend" \
		--net host --add-host lomod-dev:127.0.0.1 --dns-search local \
		-it "lomorage/dev-run" bash

build-lomod-arm: release-version
	DOCKER_BUILD_KIT=1 DOCKER_CLI_EXPERIMENTAL=enabled docker buildx build \
		--build-arg BUILD_VERSION='$$(cat LATEST_RELEASE)' \
		--build-arg SRC_DIR=${SRC_DIR} \
		--build-arg ARCH=${ARCH} \
		--build-arg OS_DISTRO=${OS_DISTRO} \
		--build-arg OS_RELEASE=${OS_RELEASE} \
		--platform ${PLATFORM} --push --tag ${IMG_BUILD} -f dockerfiles/lomod/build/Dockerfile .

build-lomod-deb:
	docker build \
		--build-arg BASE_DOCKER=${PREFIX_LOMO_VIPS}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_BUILD} \
		--build-arg SRC_DIR=${SRC_DIR} \
		--build-arg BUILD_VERSION=$$(cat LATEST_RELEASE) \
		--tag ${PREFIX_LOMOD}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_BUILD} -f dockerfiles/lomod/build/${DOCKER_FILE} .

release-lomod-deb: release-version
	sudo rm -rf artifacts
	sudo mkdir -p artifacts/opt/lomorage/bin
	docker run -v "${PWD}/artifacts/:/artifacts" \
		-it ${PREFIX_LOMOD}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_BUILD} \
		sh -c "cd /artifacts/opt/lomorage/bin/; cp /opt/lomorage/apitest/bin/* .; strip -s ./lomod ./lomoc"
		#sh -c "/sbin/ldconfig; /usr/local/bin/staticx /root/bin/lomod /artifacts/opt/lomorage/bin/lomod; /usr/local/bin/staticx /root/bin/lomoc /artifacts/opt/lomorage/bin/lomoc"
	sudo cp -r dockerfiles/lomod/DEBIAN artifacts
	sudo cp -r dockerfiles/lomod/etc artifacts
	sudo cp -r dockerfiles/lomod/lib artifacts
	sudo cp -r dockerfiles/lomod/opt artifacts
	sudo chmod -R +r artifacts
	sudo sed -i "s/version-replace-me/$$(cat LATEST_RELEASE)/g" artifacts/DEBIAN/control
	sudo sed -i "s/arch-replace-me/${DEB_ARCH}/g" artifacts/DEBIAN/control
	sudo dpkg-deb --build artifacts
	mkdir -p releases
	mv -f artifacts.deb releases/lomod_${ARCH}_${OS_DISTRO}_${OS_RELEASE}.deb

test-lomod-deb:
	docker build \
		--build-arg BASE_DOCKER=${PREFIX_LOMO_VIPS}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_BUILD} \
		--build-arg SRC_DIR=${SRC_DIR} \
		--tag ${PREFIX_LOMOD}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_TEST} -f dockerfiles/lomod/test/${DOCKER_FILE} .
	docker run --rm --name test-lomod-release \
		-e LOMOD_ENV_BASE_DIR=/opt/lomorage \
		-e LOMOD_ENV_OS_DISTRO=${OS_DISTRO} \
		-e LOMOD_ENV_OS_RELEASE=${OS_RELEASE} \
		-it ${PREFIX_LOMOD}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_TEST} make ${MAKE_TEST_ARGS} | tee ./test-lomod-${ARCH}-${OS_DISTRO}-${OS_RELEASE}.log

test-lomod-release:
	docker run --name lomod-test-${OS_DISTRO}-${OS_RELEASE} -v "${PWD}/test-results:/artifacts" -it ${IMG_TEST} cp -r ${SRC_DIR}/api/test/lomod/lomod_api_test.log /artifacts/

gen-lomod-test-meta:
	docker build \
		--build-arg BASE_DOCKER=${PREFIX_LOMO_VIPS}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_BUILD} \
		--build-arg SRC_DIR=${SRC_DIR} \
		--tag ${PREFIX_LOMOD}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_META} -f dockerfiles/lomod/meta/${DOCKER_FILE} .
	docker run --rm --name get-lomod-test-meta -v "${PWD}/artifacts/:/artifacts" \
		-it ${PREFIX_LOMOD}-${ARCH}-${OS_DISTRO}-${OS_RELEASE}-${SUFFIX_META} \
		sh -c "./gen-info ${OS_DISTRO} ${OS_RELEASE}; cp -r ../../lomod/testdata /artifacts/"

build-lomod-armv7hf-debian-buster: ARCH=armv7hf
build-lomod-armv7hf-debian-buster: DEB_ARCH=armhf
build-lomod-armv7hf-debian-buster: OS_DISTRO=debian
build-lomod-armv7hf-debian-buster: OS_RELEASE=buster
build-lomod-armv7hf-debian-buster: build-lomod-deb release-lomod-deb

test-lomod-armv7hf-debian-buster: ARCH=armv7hf
test-lomod-armv7hf-debian-buster: OS_DISTRO=debian
test-lomod-armv7hf-debian-buster: OS_RELEASE=buster
test-lomod-armv7hf-debian-buster: MAKE_TEST_ARGS=test-simple
test-lomod-armv7hf-debian-buster: test-lomod-deb

build-lomod-aarch64-debian-buster: ARCH=aarch64
build-lomod-aarch64-debian-buster: DEB_ARCH=arm64
build-lomod-aarch64-debian-buster: OS_DISTRO=debian
build-lomod-aarch64-debian-buster: OS_RELEASE=buster
build-lomod-aarch64-debian-buster: build-lomod-deb release-lomod-deb

test-lomod-aarch64-debian-buster: ARCH=aarch64
test-lomod-aarch64-debian-buster: OS_DISTRO=debian
test-lomod-aarch64-debian-buster: OS_RELEASE=buster
test-lomod-aarch64-debian-buster: MAKE_TEST_ARGS=test-simple
test-lomod-aarch64-debian-buster: test-lomod-deb

build-lomod-amd64-ubuntu-focal: ARCH=amd64
build-lomod-amd64-ubuntu-focal: DEB_ARCH=amd64
build-lomod-amd64-ubuntu-focal: OS_DISTRO=ubuntu
build-lomod-amd64-ubuntu-focal: OS_RELEASE=focal
build-lomod-amd64-ubuntu-focal: build-lomod-deb release-lomod-deb

test-lomod-amd64-ubuntu-focal: ARCH=amd64
test-lomod-amd64-ubuntu-focal: OS_DISTRO=ubuntu
test-lomod-amd64-ubuntu-focal: OS_RELEASE=focal
test-lomod-amd64-ubuntu-focal: test-lomod-deb 

build-lomod-amd64-ubuntu-groovy: ARCH=amd64
build-lomod-amd64-ubuntu-groovy: DEB_ARCH=amd64
build-lomod-amd64-ubuntu-groovy: OS_DISTRO=ubuntu
build-lomod-amd64-ubuntu-groovy: OS_RELEASE=groovy
build-lomod-amd64-ubuntu-groovy: build-lomod-deb release-lomod-deb

test-lomod-amd64-ubuntu-groovy: ARCH=amd64
test-lomod-amd64-ubuntu-groovy: OS_DISTRO=ubuntu
test-lomod-amd64-ubuntu-groovy: OS_RELEASE=groovy
test-lomod-amd64-ubuntu-groovy: test-lomod-deb

gen-lomod-test-meta-armv7hf-debian-buster: ARCH=armv7hf
gen-lomod-test-meta-armv7hf-debian-buster: OS_DISTRO=debian
gen-lomod-test-meta-armv7hf-debian-buster: OS_RELEASE=buster
gen-lomod-test-meta-armv7hf-debian-buster: gen-lomod-test-meta

gen-lomod-test-meta-aarch64-debian-buster: ARCH=aarch64
gen-lomod-test-meta-aarch64-debian-buster: OS_DISTRO=debian
gen-lomod-test-meta-aarch64-debian-buster: OS_RELEASE=buster
gen-lomod-test-meta-aarch64-debian-buster: gen-lomod-test-meta

gen-lomod-test-meta-amd64-ubuntu-focal: ARCH=amd64
gen-lomod-test-meta-amd64-ubuntu-focal: OS_DISTRO=ubuntu
gen-lomod-test-meta-amd64-ubuntu-focal: OS_RELEASE=focal
gen-lomod-test-meta-amd64-ubuntu-focal: gen-lomod-test-meta

gen-lomod-test-meta-amd64-ubuntu-groovy: ARCH=amd64
gen-lomod-test-meta-amd64-ubuntu-groovy: OS_DISTRO=ubuntu
gen-lomod-test-meta-amd64-ubuntu-groovy: OS_RELEASE=groovy
gen-lomod-test-meta-amd64-ubuntu-groovy: gen-lomod-test-meta

# Cross-builds lomod+lomoc for Raspberry Pi (armhf/arm64) against Debian
# buster via native cross-gcc, no QEMU/Docker in the hot path. Meant to be
# run from WSL2 on Windows; see scripts/cross-build-rpi/README.md for the
# one-time setup (docker+buildx for sysroot extraction, cross-gcc install).
rpi-build-arm64:
	bash scripts/cross-build-rpi/build.sh arm64

rpi-build-armhf:
	bash scripts/cross-build-rpi/build.sh armhf

# amd64 builds natively inside a self-contained buster+vips Docker image
# (no lomorage base image exists for amd64/buster); build the image once with
#   docker build -t lomod-amd64-buster-build -f scripts/cross-build-rpi/amd64/Dockerfile.buster-build scripts/cross-build-rpi/amd64
rpi-build-amd64:
	bash scripts/cross-build-rpi/amd64/build.sh

rpi-package:
	bash scripts/cross-build-rpi/package.sh

rpi-release: rpi-build-arm64 rpi-build-armhf rpi-build-amd64 rpi-package

# Builds everything: arm64/armhf/amd64 lomod/lomoc + .deb packages, plus
# lomo-vips for Ubuntu jammy/noble, plus RPMs for Fedora/Rocky9/openSUSE.
# Doesn't sign or publish -- see rpm-publish-fedora and the publish-*.sh
# scripts for that. Run from WSL2; see scripts/cross-build-rpi/README.md.
build-all:
	bash scripts/cross-build-rpi/build-all.sh

# Fedora ships vips directly via dnf, so this reuses the amd64 binaries
# above unmodified -- no from-source vips build needed (unlike Rocky/
# openSUSE, not yet supported; see scripts/cross-build-rpi/rpm/README.md).
rpm-package-fedora:
	bash scripts/cross-build-rpi/rpm/package-fedora.sh

rpm-publish-fedora:
	bash scripts/cross-build-rpi/rpm/publish-fedora.sh

# Builds AND publishes release artifacts: Linux and Windows from native Windows
# git-bash, macOS from a Mac. RELEASE_ARGS="--only windows" releases just some
# platforms -- see scripts/release-all.sh's header for prerequisites and what
# "publish" touches.
release-all:
	bash scripts/release-all.sh $(RELEASE_ARGS)
