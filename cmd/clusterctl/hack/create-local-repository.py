#!/usr/bin/env python3

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

# create-local-repository.py takes in input a list of provider and, for each of them, generates the components YAML from the
# local repositories (the GitHub repositories clone), and finally stores it in the clusterctl local override folder

# prerequisites:

# - the script should be executed from sigs.k8s.io/cluster-api/ by calling cmd/clusterctl/hack/create-local-repository.py
# - there should be a sigs.k8s.io/cluster-api/clusterctl-settings.json file with the list of provider for which
#   the local overrides should be generated and the list of provider repositories to be included (on top of cluster-api).
# {
#    "providers": [ "cluster-api", "bootstrap-kubeadm", "infrastructure-aws"],
#    "provider_repos": ["../cluster-api-provider-aws"]
# }
# - for each additional provider repository there should be a sigs.k8s.io/<provider_repo>/clusterctl-settings.json file e.g.
# {
#   "name": "infrastructure-aws",
#   "config": {
#     "componentsFile": "infrastructure-components.yaml",
#     "nextVersion": "v0.5.0",
# }

###################

from __future__ import unicode_literals

import sys
import errno
import json
import os
import subprocess
import urllib.request
from distutils.dir_util import copy_tree
from distutils.file_util import copy_file

settings = {}

providers = {
    'cluster-api': {
        'componentsFile': 'core-components.yaml',
        'nextVersion': 'v1.9.99',
        'type': 'CoreProvider',
    },
    'bootstrap-kubeadm': {
        'componentsFile': 'bootstrap-components.yaml',
        'nextVersion': 'v1.9.99',
        'type': 'BootstrapProvider',
        'configFolder': 'bootstrap/kubeadm/config/default',
    },
    'control-plane-kubeadm': {
        'componentsFile': 'control-plane-components.yaml',
        'nextVersion': 'v1.9.99',
        'type': 'ControlPlaneProvider',
        'configFolder': 'controlplane/kubeadm/config/default',
    },
    'infrastructure-docker': {
        'componentsFile': 'infrastructure-components-development.yaml',
        'nextVersion': 'v1.9.99',
        'type': 'InfrastructureProvider',
        'configFolder': 'test/infrastructure/docker/config/default',
    },
    'infrastructure-in-memory': {
          'componentsFile': 'infrastructure-components-in-memory-development.yaml',
          'nextVersion': 'v1.9.99',
          'type': 'InfrastructureProvider',
          'configFolder': 'test/infrastructure/inmemory/config/default',
      },
      'runtime-extension-test': {
        'componentsFile': 'runtime-extension-components-development.yaml',
        'nextVersion': 'v1.9.99',
        'type': 'RuntimeExtensionProvider',
        'configFolder': 'test/extension/config/default',
    },
}


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
    except Exception as e:
        raise Exception('failed to run {}: {}'.format(args, e))


def get_repository_folder():
    config_dir = os.getenv("XDG_CONFIG_HOME", "")
    if config_dir == "":
        home_dir = os.getenv("HOME", "")
        if home_dir == "":
            raise Exception('HOME variable is not set')
        config_dir = os.path.join(home_dir, ".config")
    return os.path.join(config_dir, 'cluster-api', 'dev-repository')


def write_local_repository(provider, version, components_file, components_yaml, metadata_file):
    try:
        repository_folder = get_repository_folder()
        provider_folder = os.path.join(repository_folder, provider, version)
        try:
            os.makedirs(provider_folder)
        except OSError as e:
            if e.errno != errno.EEXIST:
                raise
        components_path = os.path.join(provider_folder, components_file)
        f = open(components_path, 'wb')
        f.write(components_yaml)
        f.close()

        copy_file(metadata_file, provider_folder)

        if provider == "infrastructure-docker":
            copy_tree("test/infrastructure/docker/templates", provider_folder)

        if provider == "infrastructure-in-memory":
            copy_tree("test/infrastructure/inmemory/templates", provider_folder)

        return components_path
    except Exception as e:
        raise Exception('failed to write {} to {}: {}'.format(components_file, provider_folder, e))


def create_local_repositories():
    providerList = settings.get('providers', [])
    assert providerList is not None, 'invalid configuration: please define the list of providers to override'
    assert len(providerList)>0, 'invalid configuration: please define at least one provider to override'

    if len(sys.argv) == 1:
        execCmd(['make', 'kustomize'])

    for provider in providerList:
        p = providers.get(provider)
        assert p is not None, 'invalid configuration: please specify the configuration for the {} provider'.format(
            provider)

        repo = p.get('repo', '.')
        config_folder = p.get('configFolder', 'config/default')
        metadata_file = repo + '/metadata.yaml'

        next_version = p.get('nextVersion')
        assert next_version is not None, 'invalid configuration for provider {}: please provide nextVersion value'.format(
            provider)

        name, type = splitNameAndType(provider)
        assert name is not None, 'invalid configuration for provider {}: please use a valid provider label'.format(
            provider)

        components_file = p.get('componentsFile')
        assert components_file is not None, 'invalid configuration for provider {}: please provide componentsFile value'.format(
            provider)

        if len(sys.argv) > 1:
            url = "{}/{}".format(sys.argv[1], components_file)
            components_yaml = urllib.request.urlopen(url).read()
        else:
            components_yaml = execCmd(['./hack/tools/bin/kustomize', 'build', os.path.join(repo, config_folder)])

        components_path = write_local_repository(provider, next_version, components_file, components_yaml,
                                                     metadata_file)

        yield name, type, next_version, components_path


def injectLatest(path):
    head, tail = os.path.split(path)
    return '{}/latest/{}'.format(head, tail)


def create_dev_config(repos):
    yaml = "providers:\n"
    for name, type, next_version, components_path in repos:
        yaml += "- name: \"{}\"\n".format(name)
        yaml += "  type: \"{}\"\n".format(type)
        yaml += "  url: \"{}\"\n".format(components_path)
    yaml += "overridesFolder: \"{}/overrides\"\n".format(get_repository_folder())

    try:
        repository_folder = get_repository_folder()
        config_path = os.path.join(repository_folder, "config.yaml")
        f = open(config_path, 'w')
        f.write(yaml)
        f.close()
        return components_path
    except Exception as e:
        raise Exception('failed to write {}: {}'.format(config_path, e))


def splitNameAndType(provider):
    if provider == 'cluster-api':
        return 'cluster-api', 'CoreProvider'
    if provider.startswith('bootstrap-'):
        return provider[len('bootstrap-'):], 'BootstrapProvider'
    if provider.startswith('control-plane-'):
        return provider[len('control-plane-'):], 'ControlPlaneProvider'
    if provider.startswith('infrastructure-'):
        return provider[len('infrastructure-'):], 'InfrastructureProvider'
    if provider.startswith('ipam-'):
        return provider[len('ipam-'):], 'IPAMProvider'
    if provider.startswith('runtime-extension-'):
        return provider[len('runtime-extension-'):], 'RuntimeExtensionProvider'
    if provider.startswith('addon-'):
        return provider[len('addon-'):], 'AddonProvider'
    return None, None


def CoreProviderFlag():
    return '--core'


def BootstrapProviderFlag():
    return '--bootstrap'


def ControlPlaneProviderFlag():
    return '--control-plane'


def InfrastructureProviderFlag():
    return '--infrastructure'


def IPAMProviderFlag():
    return '--ipam'


def RuntimeExtensionProviderFlag():
    return '--runtime-extension'


def AddonProviderFlag():
    return '--addon'


def type_to_flag(type):
    switcher = {
        'CoreProvider': CoreProviderFlag,
        'BootstrapProvider': BootstrapProviderFlag,
        'ControlPlaneProvider': ControlPlaneProviderFlag,
        'InfrastructureProvider': InfrastructureProviderFlag,
        'IPAMProvider': IPAMProviderFlag,
        'RuntimeExtensionProvider': RuntimeExtensionProviderFlag,
        'AddonProvider': AddonProviderFlag
    }
    func = switcher.get(type, lambda: 'Invalid type')
    return func()


def print_instructions(repos):
    providerList = settings.get('providers', [])
    print('clusterctl local overrides generated from local repositories for the {} providers.'.format(
        ', '.join(providerList)))
    print('in order to use them, please run:')
    print
    cmd = "clusterctl init \\\n"
    for name, type, next_version, components_path in repos:
        cmd += "   {} {}:{} \\\n".format(type_to_flag(type), name, next_version)
    config_dir = os.getenv("XDG_CONFIG_HOME", "")
    if config_dir != "":
        cmd += "   --config $XDG_CONFIG_HOME/cluster-api/dev-repository/config.yaml"
    else:
        cmd += "   --config $HOME/.config/cluster-api/dev-repository/config.yaml"
    print(cmd)
    print
    if 'infrastructure-docker' in providerList:
        print('please check the documentation for additional steps required for using the docker provider')
        print
    if 'infrastructure-in-memory' in providerList:
        print ('please check the documentation for additional steps required for using the in-memory provider')
        print


load_settings()

load_providers()

repos = list(create_local_repositories())

create_dev_config(repos)

print_instructions(repos)
