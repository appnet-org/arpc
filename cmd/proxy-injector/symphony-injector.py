import argparse
import sys
import yaml

# Label key to control injection
INJECTION_LABEL = "symphony.appnet.io/inject"

mode_to_image = {
    "symphony": {
        "proxy": "appnetorg/symphony-proxy:latest",
        "init-container": "appnetorg/symphony-proxy-init-container:latest"
    },
    "h2": {
        "proxy": "appnetorg/symphony-proxy-h2:latest",
        "init-container": "appnetorg/symphony-proxy-h2-init-container:latest"
    }
}

def should_inject(metadata):
    """
    Determines whether to inject based on the symphony.appnet.io/inject label.
    If label is explicitly set to "false", return False. Otherwise, inject.
    """
    labels = metadata.get("labels", {})
    return labels.get(INJECTION_LABEL, "true").lower() != "false"

def inject_proxy(pod_spec, mode):
    """
    Injects Symphony proxy and iptables initContainer into the given pod spec.
    """
    if "containers" not in pod_spec:
        return

    proxy_container = {
        "name": "symphony-proxy",
        "image": mode_to_image[mode]["proxy"],
        "command": ["/app/proxy"],
        "securityContext": {
            "runAsUser": 1337,
            "capabilities": {"add": ["NET_ADMIN", "NET_RAW"]}
        }
    }

    init_container = {
        "name": "set-iptables",
        "image": mode_to_image[mode]["init-container"],
        "command": ["/bin/sh", "-c", "bash /apply_symphony_iptables.sh"],
        "securityContext": {
            "runAsUser": 0,
            "capabilities": {"add": ["NET_ADMIN"]}
        }
    }

    pod_spec.setdefault("initContainers", []).append(init_container)
    pod_spec["containers"].append(proxy_container)

def process_yaml(documents, mode):
    """
    Processes all YAML documents, injecting into Deployments unless excluded by label.
    """
    for doc in documents:
        if doc.get("kind") != "Deployment":
            continue

        metadata = doc.get("metadata", {})
        if not should_inject(metadata):
            continue

        pod_spec = doc.get("spec", {}).get("template", {}).get("spec", {})
        inject_proxy(pod_spec, mode)

    return documents

def main():
    parser = argparse.ArgumentParser(description="Auto-inject Symphony proxy and init container into K8s manifests.")
    parser.add_argument("-f", "--file", help="Input YAML file. Reads from stdin if not specified.")
    parser.add_argument("--mode", choices=["symphony", "h2"], default="symphony", help="Proxy mode: symphony or h2 (default: symphony)")
    args = parser.parse_args()

    with open(args.file) if args.file else sys.stdin as f:
        documents = list(yaml.safe_load_all(f))

    modified_documents = process_yaml(documents, args.mode)
    yaml.dump_all(modified_documents, sys.stdout, sort_keys=False)

if __name__ == "__main__":
    main()
