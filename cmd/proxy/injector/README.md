# symphony-injector

A lightweight tool to automatically inject the Symphony proxy (similar to istio) and iptables init container into Kubernetes Deployment manifests.

## Prerequisite

```bash
pip install pyyaml
```

## Usage

### Inject via file

```bash
python symphony_injector.py -f input.yaml > output.yaml
```

### Inject via stdin

```bash
cat input.yaml | python symphony_injector.py > output.yaml
```

## Opt-out

Add this label to your Deployment to skip injection:

```yaml
metadata:
  labels:
    symphony.io/inject: "false"
```
