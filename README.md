# Indexing Services

> Indexing for Storacha Network, Cached And Ready To Go

## Table of Contents

* [Overview](#overview)
* [Installation](#installation)
* [Deployment](#deployment)
* [Contribute](#contribute)
* [License](#license)

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

#### `make validate`

This will validate your terraform configuration -- good to run to check errors in any changes you make to terraform configs.

#### `make plan`

This will plan a deployment, but not execute it -- useful to see ahead what changes will happen when you run the next deployment.

#### `make apply`

The big kahuna! This will deploy all of your changes, including redeploying lambdas if any of code changes.

## Contribute

Early days PRs are welcome!

## License

This library is dual-licensed under Apache 2.0 and MIT terms.

Copyright 2024. Storacha Network Inc.
