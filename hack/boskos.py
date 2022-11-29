#!/usr/bin/env python3

# Copyright 2021 The Kubernetes Authors.
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

import argparse
import json
import os

import requests
import time

BOSKOS_HOST = os.environ.get("BOSKOS_HOST", "boskos")
BOSKOS_RESOURCE_NAME = os.environ.get('BOSKOS_RESOURCE_NAME')


def checkout_account_request(resource_type, user, input_state):
    url = f'http://{BOSKOS_HOST}/acquire?type={resource_type}&state={input_state}&dest=busy&owner={user}'
    r = requests.post(url)
    status = r.status_code
    reason = r.reason
    result = ""

    if status == 200:
        content = r.content.decode()
        result = json.loads(content)

    return status, reason, result


def checkout_account(resource_type, user):
    status, reason, result = checkout_account_request(resource_type, user, "clean")
    # TODO(sbueringer): find out if we still need this
    # replicated the implementation of cluster-api-provider-gcp
    #  we're working around an issue with the data in boskos.
    #  We'll remove the code that tries both free and clean once all the data is good.
    #  Afterwards we should just check for free
    if status == 404:
        status, reason, result = checkout_account_request(resource_type, user, "free")

    if status != 200:
        raise Exception(f"Got invalid response {status}: {reason}")

    print(f"export BOSKOS_RESOURCE_NAME={result['name']}")
    print(f"export GCP_PROJECT={result['name']}")


def release_account(user):
    url = f'http://{BOSKOS_HOST}/release?name={BOSKOS_RESOURCE_NAME}&dest=dirty&owner={user}'

    r = requests.post(url)

    if r.status_code != 200:
        raise Exception(f"Got invalid response {r.status_code}: {r.reason}")


def send_heartbeat(user):
    url = f'http://{BOSKOS_HOST}/update?name={BOSKOS_RESOURCE_NAME}&state=busy&owner={user}'

    while True:
        print(f"POST-ing heartbeat for resource {BOSKOS_RESOURCE_NAME} to {BOSKOS_HOST}")
        r = requests.post(url)

        if r.status_code == 200:
            print(f"response status: {r.status_code}")
        else:
            print(f"Got invalid response {r.status_code}: {r.reason}")

        time.sleep(60)


def main():
    parser = argparse.ArgumentParser(description='Boskos GCP Account Management')

    parser.add_argument(
        '--get', dest='checkout_account', action="store_true",
        help='Checkout a Boskos GCP Account'
    )

    parser.add_argument(
        '--release', dest='release_account', action="store_true",
        help='Release a Boskos GCP Account'
    )

    parser.add_argument(
        '--heartbeat', dest='send_heartbeat', action="store_true",
        help='Send heartbeat for the checked out a Boskos GCP Account'
    )

    parser.add_argument(
        '--resource-type', dest="resource_type", type=str,
        default="gce-project",
        help="Type of Boskos resource to manage"
    )

    parser.add_argument(
        '--user', dest="user", type=str,
        default="cluster-api",
        help="username"
    )

    args = parser.parse_args()

    if args.checkout_account:
        checkout_account(args.resource_type, args.user)

    elif args.release_account:
        release_account(args.user)

    elif args.send_heartbeat:
        send_heartbeat(args.user)


if __name__ == "__main__":
    main()
