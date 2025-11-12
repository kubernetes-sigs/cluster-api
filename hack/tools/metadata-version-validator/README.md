# Metadata Version Validator

## Purpose

A small utility that given a version number it will check that a release series is defined in the passed in metadata.yaml file.

This is primarily for use in release pipelines.

## Usage

First create the binary by:

```bash
cd hack/tools
go build -tags=tools -o bin/metadata-version-validator sigs.k8s.io/cluster-api/hack/tools/metadata-version-validator
```

Then run the binary to check a version:

```bash
bin/metadata-version-validator --file metadata.yaml --version v2.8.0
```

If there is no release series defined there will be an error message and also a non-zero exit code.
