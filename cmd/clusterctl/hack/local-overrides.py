#!/usr/bin/env python

# Copyright 2020 The Kubernetes Authors.
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

###################

# local-overrides.py takes in input a list of provider and, for each of them, generates the components YAML from the
# local repositories (the GitHub repositories clone), and finally stores it in the clusterctl local override folder

# prerequisites:

# - the script should be executed from sigs.k8s.io/cluster-api/ by calling cmd/clusterctl/hack/local-overrides.py
# - there should be a sigs.k8s.io/cluster-api/clusterctl-settings.json file with the list of provider for which
#   the local overrides should be generated and the list of provider repositories to be included (on top of cluster-api).
# {
#    "providers": [ "cluster-api", "kubeadm-bootstrap", "aws"],
#    "provider_repos": ["../cluster-api-provider-aws"]
# }
# - for each additional provider repository there should be a sigs.k8s.io/<provider_repo>/clusterctl-settings.json file e.g.
# {
#   "name": "aws",
#   "config": {
#     "componentsFile": "infrastructure-components.yaml",
#     "nextVersion": "v0.5.0",
#     "type": "InfrastructureProvider"
# }

###################

from __future__ import unicode_literals

import json
import subprocess
import os
import errno
import sys

settings = {}

providers = {
      'cluster-api': {
              'componentsFile': 'core-components.yaml',
              'nextVersion': 'v0.3.0',
              'type': 'CoreProvider',
      },
      'kubeadm-bootstrap': {
            'componentsFile': 'bootstrap-components.yaml',
            'nextVersion': 'v0.3.0',
            'type': 'BootstrapProvider',
            'configFolder': 'bootstrap/kubeadm/config/default',
      },
      'docker': {
          'componentsFile': 'infrastructure-components.yaml',
          'nextVersion': 'v0.3.0',
          'type': 'InfrastructureProvider',
          'configFolder': 'test/infrastructure/docker/config/default',
      },
}

validTypes = ['CoreProvider','BootstrapProvider','InfrastructureProvider']

def load_settings():
    global settings
    try:
        settings = json.load(open('clusterctl-settings.json'))
    except  Exception as e:
        raise Exception('failed to load clusterctl-settings.json: {}'.format(e))

def load_providers():
    provider_repos = settings.get('provider_repos', [])
    for repo in provider_repos:
        file = repo + '/clusterctl-settings.json'
        try:
            provider_details = json.load(open(file))
            provider_name = provider_details['name']
            provider_config = provider_details['config']
            provider_config['repo'] = repo
            providers[provider_name] = provider_config
        except  Exception as e:
            raise Exception('failed to load clusterctl-settings.json from repo {}: {}'.format(repo, e))

def execCmd(args):
    try:
        out = subprocess.Popen(args,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT)

        stdout, stderr = out.communicate()
        if stderr is not None:
            raise Exception('stderr contains: \n{}'.format(stderr))

        return stdout
    except  Exception as e:
        raise Exception('failed to run {}: {}'.format(args, e))

def get_home():
    return os.path.expanduser('~')

def write_local_override(provider, version, components_file, components_yaml):
    try:
        home = get_home()
        overrides_folder = os.path.join(home, '.cluster-api', 'overrides')
        provider_overrides_folder = os.path.join(overrides_folder, provider, version)
        try:
            os.makedirs(provider_overrides_folder)
        except OSError as e:
            if e.errno != errno.EEXIST:
                raise
        f = open(os.path.join(provider_overrides_folder, components_file), 'wb')
        f.write(components_yaml)
        f.close()
    except Exception as e:
        raise Exception('failed to write {} to {}: {}'.format(components_file, provider_overrides_folder, e))

def create_local_overrides():
    providerList = settings.get('providers', [])
    assert providerList is not None, 'invalid configuration: please define the list of providers to override'
    assert len(providerList)>0, 'invalid configuration: please define at least one provider to override'

    for provider in providerList:
        p = providers.get(provider)
        assert p is not None, 'invalid configuration: please specify the configuration for the {} provider'.format(provider)

        repo = p.get('repo', '.')
        config_folder = p.get('configFolder', 'config/default')

        next_version = p.get('nextVersion')
        assert next_version is not None, 'invalid configuration for provider {}: please provide nextVersion value'.format(provider)

        type = p.get('type')
        assert type is not None, 'invalid configuration for provider {}: please provide type value'.format(provider)
        assert type in validTypes, 'invalid configuration for provider {}: please use one of {}'.format(provider, ', '.join(validTypes))

        components_file = p.get('componentsFile')
        assert components_file is not None, 'invalid configuration for provider {}: please provide componentsFile value'.format(provider)

        components_yaml = execCmd(['kustomize', 'build', os.path.join(repo, config_folder)])
        write_local_override(provider, next_version, components_file, components_yaml)

        yield provider, type, next_version


def CoreProviderFlag():
    return '--core'

def BootstrapProviderFlag():
    return '--bootstrap'

def InfrastructureProviderFlag():
    return '--infrastructure'

def type_to_flag(type):
    switcher = {
        'CoreProvider': CoreProviderFlag,
        'BootstrapProvider': BootstrapProviderFlag,
        'InfrastructureProvider': InfrastructureProviderFlag
    }
    func = switcher.get(type, lambda: 'Invalid type')
    return func()

def print_instructions(overrides):
    providerList = settings.get('providers', [])
    print ('clusterctl local overrides generated from local repositories for the {} providers.'.format(', '.join(providerList)))
    print ('in order to use them, please run:')
    print
    cmd = 'clusterctl init'
    for provider, type, next_version in overrides:
        cmd += ' {} {}:{}'.format(type_to_flag(type), provider, next_version)
    print (cmd)
    print

load_settings()

load_providers()

overrides = create_local_overrides()

print_instructions(overrides)
