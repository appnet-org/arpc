# AppNet RPC (aRPC)

[![Go Report Card](https://goreportcard.com/badge/github.com/appnet-org/arpc)](https://goreportcard.com/report/github.com/appnet-org/arpc)
[![Apache 2.0 License][license-badge]][license-link]

[license-badge]: https://img.shields.io/github/license/appnet-org/arpc
[license-link]: https://github.com/appnet-org/arpc/blob/main/LICENSE

**AppNet RPC (arpc)** is a minimal, fast, and pluggable Remote Procedure Call framework built on top of **UDP**, with support for customizable serialization formats.

## Prerequisites

- Go 1.20 or later
    - For installation instructions, see Go’s [Getting Started](https://go.dev/doc/install) guide.

## Quick Start 

See [examples/README.md](examples/README.md)

> **Note:** If you're running `aRPC` on Kubernetes and want to connect using a DNS name (e.g., `server.default.svc.cluster.local`), you must:
>
> 1. Define your service as a **headless service** by setting:
>    ```yaml
>    spec:
>      clusterIP: None
>    ```
> 2. Explicitly specify the **UDP protocol** for your service port:
>    ```yaml
>    ports:
>      - port: 9000
>        targetPort: 9000
>        protocol: UDP
>    ```
> 3. Use the **fully qualified domain name (FQDN)** when specifying the server address, such as `server.default.svc.cluster.local:9000`.
>
> Without these settings, Kubernetes will assign a default TCP-based ClusterIP, which does **not work properly for aRPC(UDP) communication**.



## Learn more

- [Low-level technical docs](docs/)
- [Performance Benchmark](benchmark/)


## Contact

If you have any questions or comments, please get in touch with Xiangfeng Zhu (xfzhu@cs.washington.edu).

