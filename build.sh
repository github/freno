#!/bin/bash

# Simple packaging of freno
#
# Requires fpm: https://github.com/jordansissel/fpm
#

# This script is based off https://github.com/github/orchestrator/blob/master/build.sh
# hence licensed under the Apache 2.0 license.

# Copyright 2017 Shlomi Noach, GitHub Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

mydir=$(dirname $0)
GIT_COMMIT=$(git rev-parse HEAD)
RELEASE_VERSION=
TOPDIR=/tmp/freno-release
export RELEASE_VERSION TOPDIR

usage() {
  echo
  echo "Usage: $0 [-t target ] [-a arch ] [ -p prefix ] [-h] [-d] [-r]"
  echo "Options:"
  echo "-h Show this screen"
  echo "-t (linux|darwin) Target OS Default:(linux)"
  echo "-a (amd64|386) Arch Default:(amd64)"
  echo "-d debug output"
  echo "-b build only, do not generate packages"
  echo "-p build prefix Default:(/usr/bin)"
  echo "-r build with race detector"
  echo "-v release version (optional; default: content of RELEASE_VERSION file)"
  echo "-s release subversion (optional; default: empty)"
  echo
}

function precheck() {
  local target="$1"
  local build_only="$2"
  local ok=0 # return err. so shell exit code

  if [[ "$target" == "linux" ]]; then
    if [[ $build_only -eq 0 ]] && [[ ! -x "$( which fpm )" ]]; then
      echo "Please install fpm and ensure it is in PATH (typically: 'gem install fpm')"
      ok=1
    fi

    if [[ $build_only -eq 0 ]] && [[ ! -x "$( which rpmbuild )" ]]; then
      echo "rpmbuild not in PATH, rpm will not be built (OS/X: 'brew install rpm')"
    fi
  fi

  if [[ -z "$GOPATH" ]]; then
    echo "GOPATH not set"
    ok=1
  fi

  if [[ ! -x "$( which go )" ]]; then
    echo "go binary not found in PATH"
    ok=1
  fi

  if ! go version | egrep -q 'go(1\.1[234])' ; then
    echo "go version must be 1.12 or above"
    ok=1
  fi

  return $ok
}

function setuptree() {
  local b prefix
  prefix="$1"

  mkdir -p $TOPDIR
  rm -rf ${TOPDIR:?}/*
  b=$( mktemp -d $TOPDIR/frenoXXXXXX ) || return 1
  mkdir -p $b/freno
  mkdir -p $b/freno${prefix}
  mkdir -p $b/freno/etc/init.d
  mkdir -p $b/freno/var/lib
  echo $b
}

function oinstall() {
  local builddir prefix
  builddir="$1"
  prefix="$2"

  cd  $mydir
  gofmt -s -w pkg/
  cp ./resources/etc/init.d/freno $builddir/freno/etc/init.d/freno
  chmod +x $builddir/freno/etc/init.d/freno
  cp ./resources/freno.conf.skeleton.json $builddir/freno/etc/freno.conf.json
}

function package() {
  local target builddir prefix packages
  target="$1"
  builddir="$2"
  prefix="$3"

  cd $TOPDIR

  echo "Release version is ${RELEASE_VERSION} ( ${GIT_COMMIT} )"

  case $target in
    'linux')
      echo "Creating Linux Tar package"
      tar -C $builddir/freno -czf $TOPDIR/freno-"${RELEASE_VERSION}"-$target-$arch.tar.gz ./

      echo "Creating Distro full packages"
      fpm -v "${RELEASE_VERSION}" --epoch 1 -f -s dir -n freno -m shlomi-noach --description "Cooperative, highly available throttler service" --url "https://github.com/github/freno" --vendor "GitHub" --license "Apache 2.0" -C $builddir/freno --prefix=/ -t rpm .
      fpm -v "${RELEASE_VERSION}" --epoch 1 -f -s dir -n freno -m shlomi-noach --description "Cooperative, highly available throttler service" --url "https://github.com/github/freno" --vendor "GitHub" --license "Apache 2.0" -C $builddir/freno --prefix=/ -t deb --deb-no-default-config-files .
      ;;
    'darwin')
      echo "Creating Darwin full Package"
      tar -C $builddir/freno -czf $TOPDIR/freno-"${RELEASE_VERSION}"-$target-$arch.tar.gz ./
      ;;
  esac

  echo "---"
  echo "Done. Find releases in $TOPDIR"
}

function build() {
  local target arch builddir gobuild prefix
  os="$1"
  arch="$2"
  builddir="$3"
  prefix="$4"
  ldflags="-X main.AppVersion=${RELEASE_VERSION} -X main.GitCommit=${GIT_COMMIT}"
  echo "Building via $(go version)"
  gobuild="go build -i ${opt_race} -ldflags \"$ldflags\" -o $builddir/freno${prefix}/freno cmd/freno/main.go"

  case $os in
    'linux')
      echo "GOOS=$os GOARCH=$arch $gobuild" | bash
    ;;
    'darwin')
      echo "GOOS=darwin GOARCH=amd64 $gobuild" | bash
    ;;
  esac
  [[ $(find $builddir/freno${prefix}/ -type f -name freno) ]] &&  echo "freno binary created" || (echo "Failed to generate freno binary" ; exit 1)
}

function main() {
  local target="$1"
  local arch="$2"
  local prefix="$3"
  local build_only=$4
  local builddir

  if [ -z "${RELEASE_VERSION}" ] ; then
    RELEASE_VERSION=$(git describe --abbrev=0 --tags | tr -d 'v')
  fi

  precheck "$target" "$build_only"
  builddir=$( setuptree "$prefix" )
  oinstall "$builddir" "$prefix"
  build "$target" "$arch" "$builddir" "$prefix"
  [[ $? == 0 ]] || return 1
  if [[ $build_only -eq 0 ]]; then
    package "$target" "$builddir" "$prefix"
  fi
}

build_only=0
opt_race=
while getopts a:t:p:s:v:dbhr flag; do
  case $flag in
  a)
    arch="${OPTARG}"
    ;;
  t)
    target="${OPTARG}"
    ;;
  h)
    usage
    exit 0
    ;;
  d)
    debug=1
    ;;
  b)
    echo "Build only; no packaging"
    build_only=1
    ;;
  p)
    prefix="${OPTARG}"
    ;;
  r)
    opt_race="-race"
    ;;
  v)
    RELEASE_VERSION="${OPTARG}"
    ;;
  s)
    ;;
  ?)
    usage
    exit 2
    ;;
  esac
done

shift $(( OPTIND - 1 ));

if [ -z "$target" ]; then
	uname=$(uname)
	case $uname in
	Linux)	target=linux;;
	Darwin)	target=darwin;;
	*)	echo "Unexpected OS from uname: $uname. Exiting"
		exit 1
	esac
fi
arch=${arch:-"amd64"} # default for arch is amd64 but should take from environment
prefix=${prefix:-"/usr/bin"}

[[ $debug -eq 1 ]] && set -x
main "$target" "$arch" "$prefix" "$build_only"

echo "freno build done; exit status is $?"
