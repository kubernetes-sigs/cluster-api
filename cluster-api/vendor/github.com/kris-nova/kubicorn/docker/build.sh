#!/bin/bash
VERBOSE=FALSE
VERBOSE_DOCKER_RUN=""
VERBOSE_DOCKER_BUILD="-q"
REMOVE_IMAGE=FALSE
SHOW_HELP=FALSE
MAKE_COMMAND="make"
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

which docker >/dev/null 2>&1 || { echo >&2 "Docker is required but it's not installed. Aborting."; exit 1; }

while test $# -gt 0; do
    case "$1" in
        -h|--help)
            SHOW_HELP=TRUE
            shift
            ;;
        -v|--verbose)
            VERBOSE=TRUE
            VERBOSE_DOCKER_RUN="-e 'VERBOSE=1'"
            VERBOSE_DOCKER_BUILD=""
            shift
            ;;
        -i|--remove-image)
            REMOVE_IMAGE=TRUE
            shift
            ;;
        --make*)
            MAKE_COMMAND="make "`echo $1 | sed -e 's/^[^=]*=//g'`
            shift
            ;;
    esac
done

if ${SHOW_HELP} ; then
    echo ""
    echo "Usage:  ./build.sh"
    echo ""
    echo "A script to build Kubicorn with Docker."
    echo ""
    echo "Flags:"
    echo "    -v, --verbose        Writes output to terminal"
    echo "    -i, --remove-image   Removes the docker image before building"
    echo "    -h, --help           Outputs all flags"
    echo "    --make=COMMAND       Specify the make command to be performd, e.g. '--make=lint'"
    echo ""
    exit 0;
fi

#if ${REMOVE_IMAGE} ; then
#    echo removing old container if exists
#    docker image rm gobuilder-kubicorn
#fi

if ${VERBOSE} ; then
    echo Building container
fi
#docker build -t gobuilder-kubicorn "${DIR}" ${VERBOSE_DOCKER_BUILD}

if ${VERBOSE} ; then
    echo Running make script
fi
docker run --rm -v "/${DIR}/.."://go/src/github.com/kris-nova/kubicorn -v "/${GOPATH}/bin"://go/bin -w //go/src/github.com/kris-nova/kubicorn golang:1.8 ${MAKE_COMMAND} ${VERBOSE_DOCKER_RUN}

read -p "Done. Press enter to continue"