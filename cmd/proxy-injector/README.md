# symphony-injector

A lightweight tool to automatically inject the Symphony proxy (similar to istio) and iptables init container into Kubernetes Deployment manifests.

## Prerequisite

```bash
pip install pyyaml
```

## Usage

### Options

#### Basic Options
- `-f, --file`: Input YAML file. Reads from stdin if not specified.
- `-o, --output`: Output YAML file. Writes to stdout if not specified.
- `-m, --mode`: Proxy mode: `symphony`, `h2`, or `tcp` (default: `symphony`).

#### TLS Options
- `--tls`: Enable TLS termination for the proxy.
- `--mtls`: Enable mutual TLS (mTLS) for the proxy.
- `--tls-cert-file`: Server cert file path in container (default: `/app/certs/server-cert.pem`).
- `--tls-key-file`: Server key file path in container (default: `/app/certs/server-key.pem`).
- `--tls-ca-file`: CA cert file path for verifying client certs in mTLS.
- `--tls-client-cert-file`: Client cert file path for authenticating to upstream in mTLS.
- `--tls-client-key-file`: Client key file path for authenticating to upstream in mTLS.
- `--tls-skip-verify`: Skip server certificate verification on outbound connections (insecure, for testing only).
- `--tls-secret-name`: Name of the Kubernetes secret containing TLS certificates (default: `kvstore-tls-certs`).

### Inject via file

```bash
# Using -o option
python symphony-injector.py -f input.yaml -o output.yaml

# Using stdout redirection
python symphony-injector.py -f input.yaml > output.yaml
```

### Inject via stdin

```bash
# Using -o option
cat input.yaml | python symphony-injector.py -o output.yaml

# Using stdout redirection
cat input.yaml | python symphony-injector.py > output.yaml
```

### Specify proxy mode

```bash
# Use h2 mode
python symphony-injector.py -f input.yaml -m h2 -o output.yaml

# Use tcp mode
python symphony-injector.py -f input.yaml -m tcp -o output.yaml
```

### Enable TLS for proxy

```bash
# Basic TLS termination (server-side only)
python symphony-injector.py -f input.yaml -m tcp --tls -o output.yaml

# Mutual TLS (mTLS) with default paths
python symphony-injector.py -f input.yaml -m tcp --mtls -o output.yaml

# mTLS with all certificate files
python symphony-injector.py -f input.yaml -m tcp --mtls \
  --tls-cert-file=/app/certs/server-cert.pem \
  --tls-key-file=/app/certs/server-key.pem \
  --tls-ca-file=/app/certs/ca-cert.pem \
  --tls-client-cert-file=/app/certs/client-cert.pem \
  --tls-client-key-file=/app/certs/client-key.pem \
  -o output.yaml

# mTLS with skip verify (for testing)
python symphony-injector.py -f input.yaml -m tcp --mtls \
  --tls-skip-verify \
  -o output.yaml

# Customize secret name
python symphony-injector.py -f input.yaml -m tcp --mtls \
  --tls-secret-name=my-tls-secret \
  -o output.yaml
```

## Opt-out

Add this label to your Deployment to skip injection:

```yaml
metadata:
  labels:
    symphony.appnet.io/inject: "false"
```
