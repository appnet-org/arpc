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
    },
    "tcp": {
        "proxy": "appnetorg/symphony-proxy-tcp:latest",
        "init-container": "appnetorg/symphony-proxy-tcp-init-container:latest"
    }
}

def should_inject(metadata):
    """
    Determines whether to inject based on the symphony.appnet.io/inject label.
    If label is explicitly set to "false", return False. Otherwise, inject.
    """
    labels = metadata.get("labels", {})
    return labels.get(INJECTION_LABEL, "true").lower() != "false"

def inject_proxy(pod_spec, mode, tls_enabled=False, tls_cert_path="/app/certs/server-cert.pem", 
                 tls_key_path="/app/certs/server-key.pem", tls_secret_name="kvstore-tls-certs"):
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

    # Add TLS arguments if enabled
    if tls_enabled:
        proxy_container["args"] = [
            "-mtls",
            f"-tls-cert-file={tls_cert_path}",
            f"-tls-key-file={tls_key_path}"
        ]
        
        # Add volume mounts for TLS certificates
        proxy_container["volumeMounts"] = [
            {
                "name": "tls-certs",
                "mountPath": "/app/certs",
                "readOnly": True
            }
        ]
        
        # Add volumes to pod spec if not already present
        if "volumes" not in pod_spec:
            pod_spec["volumes"] = []
        
        # Check if tls-certs volume already exists
        tls_volume_exists = any(v.get("name") == "tls-certs" for v in pod_spec["volumes"])
        if not tls_volume_exists:
            pod_spec["volumes"].append({
                "name": "tls-certs",
                "secret": {
                    "secretName": tls_secret_name
                }
            })

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

def process_yaml(documents, mode, tls_enabled=False, tls_cert_path=None, 
                 tls_key_path=None, tls_secret_name=None):
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
        inject_proxy(pod_spec, mode, tls_enabled, tls_cert_path, tls_key_path, tls_secret_name)

    return documents

def main():
    parser = argparse.ArgumentParser(description="Auto-inject Symphony proxy and init container into K8s manifests.")
    parser.add_argument("-f", "--file", help="Input YAML file. Reads from stdin if not specified.")
    parser.add_argument("-m", "--mode", choices=["symphony", "h2", "tcp"], default="symphony", help="Proxy mode: symphony or h2 or tcp (default: symphony)")
    parser.add_argument("-o", "--output", help="Output YAML file. Writes to stdout if not specified.")
    parser.add_argument("--tls", action="store_true", help="Enable mTLS for the proxy")
    parser.add_argument("--tls-cert-path", default="/app/certs/server-cert.pem", help="Path to TLS certificate file in container (default: /app/certs/server-cert.pem)")
    parser.add_argument("--tls-key-path", default="/app/certs/server-key.pem", help="Path to TLS key file in container (default: /app/certs/server-key.pem)")
    parser.add_argument("--tls-secret-name", default="kvstore-tls-certs", help="Name of the Kubernetes secret containing TLS certificates (default: kvstore-tls-certs)")
    args = parser.parse_args()

    with open(args.file) if args.file else sys.stdin as f:
        documents = list(yaml.safe_load_all(f))

    modified_documents = process_yaml(documents, args.mode, args.tls, 
                                     args.tls_cert_path, args.tls_key_path, 
                                     args.tls_secret_name)
    if args.output:
        with open(args.output, "w") as out:
            yaml.dump_all(modified_documents, out, sort_keys=False)
    else:
        yaml.dump_all(modified_documents, sys.stdout, sort_keys=False)

if __name__ == "__main__":
    main()
