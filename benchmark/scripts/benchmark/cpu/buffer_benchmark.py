import subprocess
import os
import sys
import logging
import time
import json
import statistics
import re
from datetime import datetime

# Create logs directory if it doesn't exist
log_dir = "logs"
os.makedirs(log_dir, exist_ok=True)

node_names = ["h2"]

# Configure logging
log_file = os.path.join(log_dir, f'buffer_cpu_benchmark_{datetime.now().strftime("%Y%m%d_%H%M%S")}.log')
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(message)s',
    datefmt='%H:%M:%S',
    handlers=[
        logging.StreamHandler(sys.stdout),
        logging.FileHandler(log_file)
    ]
)
logger = logging.getLogger(__name__)

# Get the directory of this script
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
ARPC_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(SCRIPT_DIR))))

# Use relative paths from the script directory
wrk_path = os.path.join(ARPC_DIR, "benchmark/scripts/wrk2/wrk")

if not os.path.exists(wrk_path):
    logger.error(f"Wrk2 not found at {wrk_path}")
    exit(1)

if not os.path.exists(wrk_path):
    logger.error(f"Wrk2 not found at {wrk_path}")
    exit(1)

manifest_dict = {
    "kv-store-grpc-no-proxy": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/buffer-manifest/kvstore.yaml"),
    "kv-store-grpc-proxy-h2-buffering": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/buffer-manifest/kvstore-proxy-h2-buffering.yaml"),
    "kv-store-grpc-proxy-h2-streaming": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/buffer-manifest/kvstore-proxy-h2-streaming.yaml"),
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

def get_virtual_cores(node_names, core_count, duration):
    """Get virtual cores usage from nodes using mpstat."""
    average_cpu_usages = []
    median_cpu_usages = []
    for node_name in node_names:
        cmd = ["ssh", node_name, "mpstat", "1", str(duration)]
        result = subprocess.run(cmd, stdout=subprocess.PIPE)
        lines = result.stdout.decode("utf-8").split("\n")
        # Parse CPU usage for each interval and calculate average and median
        cpu_usages = []
        for line in lines:
            if "all" in line and "Average" not in line:
                usage_data = line.split()
                cpu_usage = 100.00 - float(
                    usage_data[-1]
                )  # Idle percentage subtracted from 100
                cpu_usages.append(cpu_usage)

        if cpu_usages:
            average_cpu_usage = sum(cpu_usages) / len(cpu_usages)
            median_cpu_usage = statistics.median(cpu_usages)
            average_cpu_usages.append(average_cpu_usage)
            median_cpu_usages.append(median_cpu_usage)

    average_total = round(sum(average_cpu_usages) * core_count / 100, 2)
    median_total = round(sum(median_cpu_usages) * core_count / 100, 2)

    return average_total, median_total

def run_wrk2_and_get_cpu(
    node_names,
    manifest_name,
    value_size,
    num_of_packet,
    cores_per_node=64,
    mpstat_duration=30,
    wrk2_duration=60,
    target_rate=10000,
):
    """Run wrk2 and collect CPU metrics for a specific value size."""
    logger.info(f"Running wrk2 for {manifest_name} with value_size={value_size} (num_of_packet={num_of_packet})")
    
    # Generate kv.lua file dynamically
    lua_file = "kv.lua"
    with open(lua_file, "w") as f:
        f.write(f'wrk.method = "GET"\n')
        f.write(f'wrk.path = "/?op=SET&key=82131353f9ddc8c6&key_size=48&value_size={value_size}"\n')
    
    cmd = [
        wrk_path,
        "-t 10",
        "-c 100",
        "http://10.96.88.88:80",
        f"-d {wrk2_duration}",
        f"-R {str(int(target_rate))}",
        f"-s {lua_file}",
    ]
    proc = subprocess.Popen(
        " ".join(cmd),
        shell=True,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )

    average_vcores, median_vcores = get_virtual_cores(
        node_names, cores_per_node, mpstat_duration
    )

    stdout_data, stderr_data = proc.communicate()

    # Check if there was an error running wrk2
    if proc.returncode != 0:
        logger.warning("Error executing wrk2 command:")
        logger.warning(stdout_data.decode())
        logger.warning(stderr_data.decode())
        return None

    # Parse the output
    output = stdout_data.decode()

    # Extract achieved requests per second
    req_sec_pattern = r"Requests/sec:\s+(\d+\.?\d*)"
    req_sec_match = re.search(req_sec_pattern, output)
    recorded_rps = float(req_sec_match.group(1)) if req_sec_match else 0.0

    # Extract non-2xx/3xx responses if present
    error_count = 0
    for line in output.splitlines():
        if "Non-2xx or 3xx responses:" in line:
            parts = line.split("Non-2xx or 3xx responses:")
            if len(parts) == 2:
                try:
                    error_count = int(parts[1].strip())
                    logger.info(f"Found {error_count} non-2xx or 3xx responses")
                except ValueError:
                    logger.warning(f"Could not parse error count from line: {line}")

    # Check if the target request rate is achieved
    if recorded_rps < target_rate * 0.95:
        logger.warning(
            "Warning: the target request rate is not achieved. "
            f"Target: {target_rate}, achieved: {recorded_rps}."
        )

    cpu_metrics = {
        "average_vcores": float(average_vcores),
        "median_vcores": float(median_vcores),
        "recorded_rps": float(recorded_rps),
        "value_size": int(value_size),
        "num_of_packet": int(num_of_packet),
    }

    logger.info(
        f"CPU metrics for {manifest_name} (value_size={value_size}, num_of_packet={num_of_packet}): "
        f"average_vcores={cpu_metrics['average_vcores']}, "
        f"median_vcores={cpu_metrics['median_vcores']}, "
        f"recorded_rps={cpu_metrics['recorded_rps']}"
    )

    return {
        "cpu_metrics": cpu_metrics,
        "error_count": error_count,
    }

def run_buffer_cpu_benchmark(manifest_name, manifest_path):
    """Run CPU benchmark for a manifest across different value sizes."""
    # Step 0: Clean up all existing resources
    cleanup_all_resources()
    
    # Step 1: Deploy the manifest
    if not deploy_manifest(manifest_path):
        logger.error(f"Failed to deploy {manifest_path}, skipping...")
        return None
    
    # Step 2: Wait for deployment to be ready
    if not wait_for_deployment_ready():
        logger.warning(f"Deployment for {manifest_path} did not become ready, skipping...")
        cleanup_manifest(manifest_path)
        return None
    
    # Step 3: Test application health
    if not test_application():
        logger.warning(f"Application {manifest_path} is not healthy, skipping benchmark...")
        cleanup_manifest(manifest_path)
        return None
    
    # Step 4: Run wrk2 and collect CPU metrics for different value sizes
    results = []
    for i in range(0, 51, 5): 
        # First generates the value_size
        if i == 0:
            value_size = 1
        else: 
            value_size = 1400 * (i - 1)
        
        num_of_packet = value_size // 1400 + 1
        
        wrk_result = run_wrk2_and_get_cpu(
            node_names, 
            manifest_name, 
            value_size, 
            num_of_packet
        )
        
        if wrk_result is not None:
            results.append(wrk_result)
        else:
            logger.warning(f"Failed to collect CPU metrics for value_size={value_size}")
    
    # Step 5: Cleanup
    cleanup_manifest(manifest_path)
    
    return results

if __name__ == "__main__":
    # Store results for all manifests
    all_results = {}
    
    # Log script start
    logger.info("=" * 60)
    logger.info("Starting Buffer CPU Benchmark Suite")
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
        
        results = run_buffer_cpu_benchmark(manifest_name, manifest_path)
        
        if results is not None:
            all_results[manifest_name] = {
                "status": "success",
                "results": results
            }
        else:
            all_results[manifest_name] = {
                "status": "failed"
            }
        
        logger.info(f"Buffer CPU benchmark for {manifest_name} completed")
        
        # Wait a bit before next deployment
        time.sleep(5)
    
    # Log summary of results
    logger.info("")
    logger.info("=" * 60)
    logger.info("SUMMARY OF RESULTS")
    logger.info("=" * 60)
    logger.info("")
    for manifest_name, result in all_results.items():
        logger.info(f"{manifest_name}:")
        status = result.get("status")
        if status == "success":
            results_list = result.get("results", [])
            logger.info(f"  Status: {status}")
            logger.info(f"  Number of test runs: {len(results_list)}")
            for idx, test_result in enumerate(results_list):
                cpu_metrics = test_result.get("cpu_metrics", {})
                if cpu_metrics:
                    logger.info(f"  Test {idx + 1} (value_size={cpu_metrics.get('value_size', 'N/A')}, num_of_packet={cpu_metrics.get('num_of_packet', 'N/A')}):")
                    logger.info(f"    average_vcores: {cpu_metrics.get('average_vcores', 'N/A')}")
                    logger.info(f"    median_vcores: {cpu_metrics.get('median_vcores', 'N/A')}")
                    logger.info(f"    recorded_rps: {cpu_metrics.get('recorded_rps', 'N/A')}")
        else:
            logger.info(f"  Status: {status}")
        logger.info("")
    
    # Save results to a file
    results_file = os.path.join(log_dir, f"buffer_cpu_benchmark_results_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json")
    with open(results_file, "w") as f:
        json.dump(all_results, f, indent=2)
    logger.info(f"Results saved to {results_file}")
