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
- `-m, --mode`: Proxy mode: `symphony` or `h2` (default: `symphony`).

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
```

## Opt-out

Add this label to your Deployment to skip injection:

```yaml
metadata:
  labels:
    symphony.appnet.io/inject: "false"
```
