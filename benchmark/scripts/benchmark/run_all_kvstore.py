import subprocess
import time
import json
import logging
import sys
from datetime import datetime

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(message)s',
    datefmt='%H:%M:%S',
    handlers=[
        logging.StreamHandler(sys.stdout),
        logging.FileHandler(f'benchmark_run_{datetime.now().strftime("%Y%m%d_%H%M%S")}.log')
    ]
)
logger = logging.getLogger(__name__)

wrk_path = "/users/xzhu/arpc/benchmark/scripts/wrk/wrk"
lua_path = "/users/xzhu/arpc/benchmark/meta-kv-trace/kvstore-wrk.lua"

manifest_dict = {
    "kv-store-thrift-tcp": "/users/xzhu/arpc/benchmark/kv-store-thrift-tcp/manifest/kvstore.yaml",
    "kv-store-thrift-http": "/users/xzhu/arpc/benchmark/kv-store-thrift-http/manifest/kvstore.yaml",
    "kv-store-grpc": "/users/xzhu/arpc/benchmark/kv-store-grpc/manifest/kvstore.yaml",
    "kv-store-grpc-istio": "/users/xzhu/arpc/benchmark/kv-store-grpc/manifest/kvstore-istio.yaml",
    "kv-store-symphony": "/users/xzhu/arpc/benchmark/kv-store-symphony/manifest/kvstore.yaml",
    "kv-store-symphony-proxy": "/users/xzhu/arpc/benchmark/kv-store-symphony/manifest/kvstore-proxy.yaml",
}

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

def run_wrk_and_collect_latency(application_name):
    """Run wrk benchmark and collect latency metrics."""
    
    logger.info(f"Running wrk for {application_name}")
    
    # Run wrk for latency test
    cmd = [wrk_path, "-d", "60s", "-t", "1", "-c", "1", "http://10.96.88.88:80", "-s", lua_path, "-L"]
    result = subprocess.run(
        " ".join(cmd),
        shell=True,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE
    )
    
    if result.returncode != 0:
        logger.error(f"Error running wrk: {result.stderr.decode('utf-8')}")
        return None
    print(result.stdout.decode("utf-8"))
    output_lines = result.stdout.decode("utf-8").split('\n')
    
    # Extract latency metrics
    latency_metrics = {}
    for line in output_lines:
        # Look for percentile latencies (50%, 75%, 90%, 99%, etc.)
        # Format: "    50%   49.00us" - first token ends with %, second is latency
        parts = line.strip().split()
        if len(parts) >= 2 and parts[0].endswith('%'):
            try:
                percentile = parts[0].rstrip('%')
                latency_str = parts[1]
                
                # Verify latency_str has a valid unit
                if not (latency_str.endswith('us') or latency_str.endswith('ms') or latency_str.endswith('s')):
                    continue
                
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
                
                latency_metrics[percentile] = latency_ms
            except (ValueError, IndexError):
                # Skip lines that don't match the expected format
                continue
    logger.debug(f"Raw latency metrics: {latency_metrics}")
    # Log all collected percentiles
    if latency_metrics:
        logger.info(f"Latency metrics for {application_name}:")
        # Sort percentiles numerically for better display
        sorted_percentiles = sorted(latency_metrics.keys(), key=lambda x: float(x))
        for p in sorted_percentiles:
            logger.info(f"  {p}%: {latency_metrics[p]:.2f}ms")
    
    return latency_metrics

def cleanup_all_resources():
    """Delete all Kubernetes resources in the current namespace."""
    logger.info("Cleaning up all resources using 'kubectl delete all --all'...")
    result = subprocess.run(
        ["kubectl", "delete", "all", "--all"],
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

# Store results for all manifests
results = {}

# Log script start
logger.info("=" * 60)
logger.info("Starting KV Store Benchmark Suite")
logger.info(f"Testing {len(manifest_dict)} manifest configurations")
logger.info("=" * 60)
logger.info("")

# Iterate over each manifest
for manifest_name, manifest_path in manifest_dict.items():
    logger.info("")
    logger.info("=" * 60)
    logger.info(f"Testing: {manifest_name}")
    logger.info("=" * 60)
    logger.info("")
    
    # Step 0: Clean up all existing resources
    cleanup_all_resources()
    
    # Step 1: Deploy the manifest
    if not deploy_manifest(manifest_path):
        logger.error(f"Failed to deploy {manifest_name}, skipping...")
        results[manifest_name] = {"status": "deployment_failed"}
        continue
    
    # Step 2: Wait for deployment to be ready
    if not wait_for_deployment_ready():
        logger.warning(f"Deployment for {manifest_name} did not become ready, skipping...")
        cleanup_manifest(manifest_path)
        results[manifest_name] = {"status": "not_ready"}
        continue
    
    # Step 3: Test application health
    if not test_application():
        logger.warning(f"Application {manifest_name} is not healthy, skipping benchmark...")
        cleanup_manifest(manifest_path)
        results[manifest_name] = {"status": "unhealthy"}
        continue
    
    # Step 4: Run wrk and collect latency
    # Using value_size=1 for the first test (can be modified to test multiple sizes)
    latency_metrics = run_wrk_and_collect_latency(manifest_name)
    
    # Step 5: Store results
    if latency_metrics:
        results[manifest_name] = {
            "status": "success",
            "latency_metrics": latency_metrics,
            "value_size": 1
        }
    else:
        results[manifest_name] = {
            "status": "wrk_failed"
        }
    
    # Step 6: Cleanup
    cleanup_manifest(manifest_path)
    
    # Wait a bit before next deployment
    time.sleep(5)

# Log summary of results
logger.info("")
logger.info("=" * 60)
logger.info("SUMMARY OF RESULTS")
logger.info("=" * 60)
logger.info("")
for manifest_name, result in results.items():
    logger.info(f"{manifest_name}:")
    if result.get("status") == "success":
        latency = result.get("latency_metrics", {})
        if latency:
            logger.info(f"  Latency metrics:")
            # Sort percentiles numerically for better display
            sorted_percentiles = sorted(latency.keys(), key=lambda x: float(x))
            for p in sorted_percentiles:
                logger.info(f"    {p}%: {latency[p]:.2f}ms")
        logger.info(f"  Status: {result['status']}")
    else:
        logger.info(f"  Status: {result['status']}")
    logger.info("")

# Optionally save results to a file
with open("benchmark_results.json", "w") as f:
    json.dump(results, f, indent=2)
logger.info("Results saved to benchmark_results.json")