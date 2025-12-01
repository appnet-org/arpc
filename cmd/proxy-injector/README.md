# symphony-injector

A lightweight tool to automatically inject the Symphony proxy (similar to istio) and iptables init container into Kubernetes Deployment manifests.

## Prerequisite

```bash
pip install pyyaml
```

## Usage

### Options

- `-f, --file`: Input YAML file. Reads from stdin if not specified.
- `-o, --output`: Output YAML file. Writes to stdout if not specified.
- `-m, --mode`: Proxy mode: `symphony`, `h2`, or `tcp` (default: `symphony`).
- `--tls`: Enable mTLS for the proxy.
- `--tls-cert-path`: Path to TLS certificate file in container (default: `/app/certs/server-cert.pem`).
- `--tls-key-path`: Path to TLS key file in container (default: `/app/certs/server-key.pem`).
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
# Inject proxy with mTLS enabled
python symphony-injector.py -f input.yaml -m tcp --tls -o output.yaml

# Customize TLS certificate paths and secret name
python symphony-injector.py -f input.yaml -m tcp --tls \
  --tls-cert-path=/custom/path/cert.pem \
  --tls-key-path=/custom/path/key.pem \
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
