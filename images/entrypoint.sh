#!/bin/sh
# Copyright 2025 NVIDIA CORPORATION & AFFILIATES
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
#
# SPDX-License-Identifier: Apache-2.0


# Always exit on errors.
set -e

# Set known directories.
CNI_BIN_DIR="/host/opt/cni/bin"
RDMA_CNI_BIN_FILE="/usr/bin/rdma"
NO_SLEEP=0

# Give help text for parameters.
usage()
{
    printf "This is an entrypoint script for RDMA CNI to overlay its\n"
    printf "binary into location in a filesystem. The binary file will\n"
    printf "be copied to the corresponding directory.\n"
    printf "\n"
    printf "./entrypoint.sh\n"
    printf "\t-h --help\n"
    printf "\t--cni-bin-dir=%s\n" "$CNI_BIN_DIR"
    printf "\t--rdma-cni-bin-file=%s\n" "$RDMA_CNI_BIN_FILE"
    printf "\t--no-sleep\n"
}

# Parse parameters given as arguments to this script.
while [ "$1" != "" ]; do
    PARAM=$(echo "$1" | awk -F= '{print $1}')
    VALUE=$(echo "$1" | awk -F= '{print $2}')
    case $PARAM in
        -h | --help)
            usage
            exit
            ;;
        --cni-bin-dir)
            CNI_BIN_DIR=$VALUE
            ;;
        --rdma-cni-bin-file)
            RDMA_CNI_BIN_FILE=$VALUE
            ;;
        --no-sleep)
            NO_SLEEP=1
            ;;
        *)
            /bin/echo "ERROR: unknown parameter \"$PARAM\""
            usage
            exit 1
            ;;
    esac
    shift
done


# Loop through and verify each location each.
for i in $CNI_BIN_DIR $RDMA_CNI_BIN_FILE
do
  if [ ! -e "$i" ]; then
    /bin/echo "Location $i does not exist"
    exit 1;
  fi
done

# Copy file into proper place.
cp -f "$RDMA_CNI_BIN_FILE" "$CNI_BIN_DIR"

if [ $NO_SLEEP -eq 1 ]; then
  exit 0
fi

echo "Entering sleep... (success)"
trap : TERM INT

# Sleep forever.
# sleep infinity is not available in alpine; instead lets go sleep for ~68 years. Hopefully that's enough sleep
sleep 2147483647 & wait
