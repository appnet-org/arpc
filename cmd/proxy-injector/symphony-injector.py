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

def inject_proxy(pod_spec, mode, tls_config=None):
    """
    Injects Symphony proxy and iptables initContainer into the given pod spec.
    
    Args:
        pod_spec: The Kubernetes pod spec to inject into
        mode: The proxy mode (symphony, h2, tcp)
        tls_config: Dict containing TLS configuration options:
            - enabled: bool (TLS or mTLS enabled)
            - mtls: bool (mutual TLS mode)
            - cert_file: server cert path
            - key_file: server key path
            - ca_file: CA cert path
            - client_cert_file: client cert path
            - client_key_file: client key path
            - skip_verify: bool
            - secret_name: Kubernetes secret name
    """
    if "containers" not in pod_spec:
        return

    proxy_container = {
        "name": "symphony-proxy",
        "image": mode_to_image[mode]["proxy"],
        "command": ["/app/proxy"],
        "env": [
            {
                "name": "LOG_LEVEL",
                "value": "info"
            }
        ],
        "securityContext": {
            "runAsUser": 1337,
            "capabilities": {"add": ["NET_ADMIN", "NET_RAW"]}
        }
    }

    # Add TLS arguments if enabled
    if tls_config and tls_config.get("enabled"):
        args = []
        
        # Add mTLS or TLS flag
        if tls_config.get("mtls"):
            args.append("-mtls")
        else:
            args.append("-tls")
        
        # Add certificate files
        if tls_config.get("cert_file"):
            args.extend(["-tls-cert-file", tls_config["cert_file"]])
        if tls_config.get("key_file"):
            args.extend(["-tls-key-file", tls_config["key_file"]])
        if tls_config.get("ca_file"):
            args.extend(["-tls-ca-file", tls_config["ca_file"]])
        if tls_config.get("client_cert_file"):
            args.extend(["-tls-client-cert-file", tls_config["client_cert_file"]])
        if tls_config.get("client_key_file"):
            args.extend(["-tls-client-key-file", tls_config["client_key_file"]])
        if tls_config.get("skip_verify"):
            args.append("-tls-skip-verify")
        
        if args:
            proxy_container["args"] = args
        
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
                    "secretName": tls_config.get("secret_name", "kvstore-tls-certs")
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

def process_yaml(documents, mode, tls_config=None):
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
        inject_proxy(pod_spec, mode, tls_config)

    return documents

def main():
    parser = argparse.ArgumentParser(description="Auto-inject Symphony proxy and init container into K8s manifests.")
    parser.add_argument("-f", "--file", help="Input YAML file. Reads from stdin if not specified.")
    parser.add_argument("-m", "--mode", choices=["symphony", "h2", "tcp"], default="symphony", 
                       help="Proxy mode: symphony or h2 or tcp (default: symphony)")
    parser.add_argument("-o", "--output", help="Output YAML file. Writes to stdout if not specified.")
    
    # TLS configuration
    parser.add_argument("--tls", action="store_true", help="Enable TLS termination for the proxy")
    parser.add_argument("--mtls", action="store_true", help="Enable mutual TLS (mTLS) for the proxy")
    parser.add_argument("--tls-cert-file", default="/app/certs/server-cert.pem", 
                       help="Server cert file path in container (default: /app/certs/server-cert.pem)")
    parser.add_argument("--tls-key-file", default="/app/certs/server-key.pem", 
                       help="Server key file path in container (default: /app/certs/server-key.pem)")
    parser.add_argument("--tls-ca-file", default="/app/certs/ca-cert.pem", 
                       help="CA cert file path for verifying client certs in mTLS")
    parser.add_argument("--tls-client-cert-file", default="/app/certs/client-cert.pem", 
                       help="Client cert file path for authenticating to upstream in mTLS")
    parser.add_argument("--tls-client-key-file", default="/app/certs/client-key.pem", 
                       help="Client key file path for authenticating to upstream in mTLS")
    parser.add_argument("--tls-skip-verify", action="store_true", 
                       help="Skip server certificate verification on outbound connections (insecure, for testing only)")
    parser.add_argument("--tls-secret-name", default="kvstore-tls-certs", 
                       help="Name of the Kubernetes secret containing TLS certificates (default: kvstore-tls-certs)")
    
    args = parser.parse_args()

    with open(args.file) if args.file else sys.stdin as f:
        documents = list(yaml.safe_load_all(f))

    # Build TLS configuration
    tls_config = None
    if args.tls or args.mtls:
        tls_config = {
            "enabled": True,
            "mtls": args.mtls,
            "cert_file": args.tls_cert_file,
            "key_file": args.tls_key_file,
            "ca_file": args.tls_ca_file,
            "client_cert_file": args.tls_client_cert_file,
            "client_key_file": args.tls_client_key_file,
            "skip_verify": args.tls_skip_verify,
            "secret_name": args.tls_secret_name
        }

    modified_documents = process_yaml(documents, args.mode, tls_config)
    
    if args.output:
        with open(args.output, "w") as out:
            yaml.dump_all(modified_documents, out, sort_keys=False)
    else:
        yaml.dump_all(modified_documents, sys.stdout, sort_keys=False)

if __name__ == "__main__":
    main()
