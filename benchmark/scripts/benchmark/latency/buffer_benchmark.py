import subprocess
import os
import sys
import logging
import time
import json

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(message)s',
    datefmt='%H:%M:%S',
    handlers=[
        logging.StreamHandler(sys.stdout),
    ]
)
logger = logging.getLogger(__name__)

# Get the directory of this script
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
ARPC_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(SCRIPT_DIR))))

# Use relative paths from the script directory
wrk_path = os.path.join(ARPC_DIR, "benchmark/scripts/wrk/wrk")

if not os.path.exists(wrk_path):
    logger.error(f"Wrk not found at {wrk_path}")
    exit(1)

manifest_dict = {
    # "kv-store-grpc-no-proxy": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/latency/buffer-manifest/kvstore.yaml"),
    # "kv-store-grpc-envoy-buffering": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/latency/buffer-manifest/kvstore-envoy-h2-buffering.yaml"),
    # "kv-store-grpc-envoy-streaming": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/latency/buffer-manifest/kvstore-envoy-h2-streaming.yaml"),
    # "kv-store-symphony-no-proxy": os.path.join(ARPC_DIR, "benchmark/kv-store-symphony/manifest/kvstore.yaml"),
    # "kv-store-symphony-proxy-streaming": os.path.join(ARPC_DIR, "benchmark/kv-store-symphony/manifest/kvstore-proxy.yaml"),
    "kv-store-symphony-proxy-buffering": os.path.join(ARPC_DIR, "benchmark/kv-store-symphony/manifest/kvstore-proxy-buffering.yaml"),
}


# check if all manifests exist
for manifest_path in manifest_dict.values():
    if not os.path.exists(manifest_path):
        logger.error(f"Manifest {manifest_path} does not exist")
        exit(1)
    logger.info(f"Manifest {manifest_path} exists")

def deploy_manifest(manifest_path):
    """Deploy the Kubernetes manifest file."""
    logger.info(f"Deploying manifest: {manifest_path}")
    result = subprocess.run(
        ["kubectl", "apply", "-f", manifest_path],
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        logger.error(f"Error deploying manifest: {result.stderr}")
        return False
    logger.info(f"Successfully deployed: {manifest_path}")
    return True

def wait_for_deployment_ready(timeout=120):
    """Wait for all deployments to be ready."""
    logger.info("Waiting for deployments to be ready...")
    start_time = time.time()
    while time.time() - start_time < timeout:
        # Check if all deployments are ready
        result = subprocess.run(
            ["kubectl", "get", "deployments", "-o", "json"],
            capture_output=True,
            text=True
        )
        if result.returncode != 0:
            logger.error(f"Error checking deployments: {result.stderr}")
            return False
        
        try:
            deployments = json.loads(result.stdout)
            all_ready = True
            for item in deployments.get("items", []):
                status = item.get("status", {})
                ready_replicas = status.get("readyReplicas", 0)
                replicas = status.get("replicas", 0)
                if ready_replicas < replicas or replicas == 0:
                    all_ready = False
                    break
            
            if all_ready and len(deployments.get("items", [])) > 0:
                logger.info("All deployments are ready!")
                return True
        except json.JSONDecodeError:
            pass
        
        time.sleep(2)
    
    logger.warning("Timeout waiting for deployments to be ready")
    return False

def test_application(num_requests=10, timeout_duration=1):
    """Test if the application is healthy by making curl requests."""
    url = "http://10.96.88.88:80/?op=SET&key=82131353f9ddc8c6&key_size=48&value_size=87"
    successful_requests = 0

    for _ in range(num_requests):
        try:
            curl_command = ["curl", "-s", url]
            result = subprocess.run(
                curl_command, capture_output=True, text=True, timeout=timeout_duration
            )
            
            if result.returncode == 0:
                successful_requests += 1
            else:
                logger.warning(f"Curl request failed with return code {result.returncode}")
        except subprocess.TimeoutExpired:
            logger.warning("Curl request timed out!")
        except Exception as e:
            logger.warning(f"Curl request error: {e}")
 
        time.sleep(0.2)

    # Consider healthy if at least 80% of requests succeed
    is_healthy = successful_requests >= (num_requests * 0.8)
    if is_healthy:
        logger.info(f"Application is healthy ({successful_requests}/{num_requests} requests succeeded)")
    else:
        logger.warning(f"Application may be unhealthy ({successful_requests}/{num_requests} requests succeeded)")
    return is_healthy

def cleanup_all_resources():
    """Delete all Kubernetes resources in the current namespace."""
    logger.info("Cleaning up all resources using 'kubectl delete all,envoyfilters --all'...")
    result = subprocess.run(
        ["kubectl", "delete", "all,envoyfilters", "--all"],
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        # It's okay if there are no resources to delete
        if "No resources found" not in result.stderr:
            logger.warning(f"Error cleaning up resources: {result.stderr}")
    else:
        logger.info("Successfully cleaned up all resources")
    # Wait a bit for resources to be deleted
    time.sleep(15)

def cleanup_manifest(manifest_path):
    """Delete the Kubernetes manifest."""
    logger.info(f"Cleaning up manifest: {manifest_path}")
    result = subprocess.run(
        ["kubectl", "delete", "-f", manifest_path],
        capture_output=True,
        text=True
    )
    if result.returncode != 0:
        logger.warning(f"Error cleaning up manifest: {result.stderr}")
    else:
        logger.info(f"Successfully cleaned up: {manifest_path}")
    # Wait a bit for resources to be deleted
    time.sleep(5)


def run_buffer_benchmark(manifest_path):
    # Step 0: Clean up all existing resources
    cleanup_all_resources()
    
    # Step 1: Deploy the manifest
    if not deploy_manifest(manifest_path):
        logger.error(f"Failed to deploy {manifest_path}, skipping...")
        return
    
    # Step 2: Wait for deployment to be ready
    if not wait_for_deployment_ready():
        logger.warning(f"Deployment for {manifest_path} did not become ready, skipping...")
        cleanup_manifest(manifest_path)
        return
    
    time.sleep(15)
    
    # Step 3: Test application health
    if not test_application():
        logger.warning(f"Application {manifest_path} is not healthy, skipping benchmark...")
        cleanup_manifest(manifest_path)
        return
    
    # Step 4: Run wrk and collect latency
    for i in range(0, 51, 5): 
        # First generates the kv.lua file
        if i == 0:
            value_size = 1
        else: 
            value_size = 1400 * (i - 1)
        
        with open("kv.lua", "w") as f:
            f.write(f'wrk.method = "GET"\n')
            f.write(f'wrk.path = "/?op=SET&key=82131353f9ddc8c6&key_size=48&value_size={value_size}"\n')
        
        num_of_packet = value_size // 1400 + 1
        print(f"Running wrk for (num_of_packet={num_of_packet})")
        # Run wrk for latency test
        cmd =[wrk_path, "-d", "30s", "-t", "1", "-c", "1", "http://10.96.88.88:80", "-s", "kv.lua", "-L"]
        result = subprocess.run(" ".join(cmd), shell=True, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout.decode("utf-8").split('\n')
        
        # Extract 50% latency and convert to milliseconds
        for line in result:
            if "50%" in line:
                # Parse the line like "     50%  330.00us"
                parts = line.strip().split()
                if len(parts) >= 2:
                    latency_str = parts[1]
                    # Extract number and unit
                    latency_value = float(latency_str.rstrip('us').rstrip('s').rstrip('ms'))
                    unit = latency_str[-2:] if len(latency_str) >= 2 else 'ms'
                    
                    # Convert to milliseconds
                    if unit == 'us':
                        latency_ms = latency_value / 1000
                    elif unit == 's':
                        latency_ms = latency_value * 1000
                    else:  # already ms
                        latency_ms = latency_value
                    
                    
                    print(f"50% Latency: {latency_ms:.2f}ms (num_of_packet={num_of_packet})")
                    break

if __name__ == "__main__":
    for manifest_name, manifest_path in manifest_dict.items():
        logger.info(f"Running buffer benchmark for {manifest_name}")
        run_buffer_benchmark(manifest_path)
        logger.info(f"Buffer benchmark for {manifest_name} completed")
