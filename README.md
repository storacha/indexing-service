# Indexing Services

> Indexing for Storacha Network, Cached And Ready To Go

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Deployment](#deployment)
- [Contribute](#contribute)
- [License](#license)

## Overview

This is a cache and query node for finding content on Storacha quickly.

## Installation

Download the [indexing service binary from the latest release](https://github.com/storacha/indexing-service/releases/latest) based on your system architecture, or download and install the [indexing-service](https://github.com/storacha/indexing-service) package using the Go package manager:

```bash
$ go install github.com/storacha/indexing-service/cmd@latest

...
```

## Deployment

Deployment of this service to AWS is managed by terraform which you can invoke with `make`.

First, install OpenTofu e.g.

```sh
brew install opentofu
```

or for Linux distributions that support Snap:

```sh
snap install --classic opentofu
```

for other Operating Systems see: https://opentofu.org/docs/intro/install

### AWS settings

The terraform configuration will fetch AWS settings (such as credentials and the region to deploy resources to) from your local AWS configuration. Although an installation of the AWS CLI is not strictly required, it can be a convenient way to manage these settings.

OpenTofu will go to the same places as the AWS CLI to find settings, which means it will read environment variables such as `AWS_REGION` and `AWS_PROFILE` and the `~/.aws/config` and `~/.aws/credentials` files.

Make sure you are using the correct AWS profile and region before invoking `make` targets.

### `.env`

You need to first generate a .env with relevant vars. Copy `.env.local` to `.env` and then set the following environment variables:

#### `TF_WORKSPACE`

Best to set this to your name. "prod" and "staging" are reserved for shared deployments.

#### `TF_VAR_private_key`

This is a multibase encoded ed25519 private key used to sign receipts and for the indexer's peer ID. For development, you can generate one by running `make ucankey`.

#### `TF_VAR_did`

This is the DID for this deployment (did:web:... for example). e.g.

```sh
TF_VAR_did='did:web:yourname.indexer.storacha.network'
```

#### `TF_VAR_public_url`

This is the public URL of the peer for this deployment. e.g.

```sh
TF_VAR_public_url='https://yourname.indexer.storacha.network'
```

### Deployment commands

Note that these commands will call needed prerequisites -- `make apply` will essentially do all of these start to finish.

#### `make lambdas`

This will simply compile the lambdas locally and put then in the `build` directory.

#### `make init`

You should only need to run this once -- initializes your terraform deployment and workspace. Make sure you've set `TF_WORKSPACE` first!

If the `make init` fails you will need to execute `tofu init` directly from the `deploy/app` folder to install the required dependencies, and it will update the `.terraform.lock.hcl` file if needed.

#### `make validate`

This will validate your terraform configuration -- good to run to check errors in any changes you make to terraform configs.

#### `make plan`

This will plan a deployment, but not execute it -- useful to see ahead what changes will happen when you run the next deployment.

#### `make apply`

The big kahuna! This will deploy all of your changes, including redeploying lambdas if any of code changes.

## Query

#### `./indexer query <CID>`
Attempts to find the given CID in the Indexer node. The result is a Location Claim that needs to be used to fetch the actual content associated with that CID. In case you want to query a specific node, you can use the following command:

```sh
./indexer query -u https://<NODE_NAME>.indexer.storacha.network <CID>
```

If you don't specify a node it will query the Storacha Production node at https://indexer.storacha.network

## Releasing a new version

Every time changes are merged to `main` the staging environment is automatically updated. Therefore, staging always runs the latest version of the code. The production environment, however, is only updated when a new version is released.

The release process is automated in the repo's GitHub Actions workflows. Releasing a new version is as easy as updating the version in [version.json](version.json).

When a branch with changes to [version.json](version.json) is merged to `main`, the release workflow will automatically tag the commit with the version, build and upload binaries, and create a new release.

The release workflow not only publishes new releases. On successful runs, it will also trigger an additional deployment workflow that will deploy the new version to production by applying the Terraform configuration.

## Contribute

Early days PRs are welcome!

## License

This library is dual-licensed under Apache 2.0 and MIT terms.

Copyright 2024. Storacha Network Inc.
