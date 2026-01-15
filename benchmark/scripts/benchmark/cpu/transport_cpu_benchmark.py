import subprocess
import os
import sys
import logging
import time
import json
import statistics
import re
import argparse
from datetime import datetime

# Create logs directory if it doesn't exist
log_dir = "logs"
os.makedirs(log_dir, exist_ok=True)

node_names = ["h2"]

# Configure logging
log_file = os.path.join(log_dir, f'transport_cpu_benchmark_{datetime.now().strftime("%Y%m%d_%H%M%S")}.log')
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
ARPC_DIR = "/users/xzhu/arpc"

# Use relative paths from the script directory
wrk_path = os.path.join(ARPC_DIR, "benchmark/scripts/wrk2/wrk")
lua_path = os.path.join(ARPC_DIR, "benchmark/meta-kv-trace/kvstore-wrk.lua")

if not os.path.exists(wrk_path):
    logger.error(f"Wrk2 not found at {wrk_path}")
    exit(1)

# All available transport variants
ALL_VARIANTS = ["udp", "reliable", "cc", "reliable-cc", "fc", "cc-fc", "reliable-fc", "reliable-cc-fc", "reliable-cc-fc-encryption", "quic"]

# Manifest paths for each variant
manifest_dict = {
    "kv-store-symphony-transport-udp": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-udp.yaml"),
    "kv-store-symphony-transport-reliable": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-reliable.yaml"),
    # "kv-store-symphony-transport-cc": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-cc.yaml"),
    "kv-store-symphony-transport-reliable-cc": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-reliable-cc.yaml"),
    # "kv-store-symphony-transport-fc": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-fc.yaml"),
    # "kv-store-symphony-transport-cc-fc": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-cc-fc.yaml"),
    # "kv-store-symphony-transport-reliable-fc": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-reliable-fc.yaml"),
    "kv-store-symphony-transport-reliable-cc-fc": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-reliable-cc-fc.yaml"),
    "kv-store-symphony-transport-reliable-cc-fc-encryption": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-reliable-cc-fc-encryption.yaml"),
    "kv-store-symphony-transport-quic": os.path.join(ARPC_DIR, "benchmark/scripts/benchmark/cpu/transport-manifest/kvstore-quic.yaml"),
}

def parse_arguments():
    """Parse command line arguments."""
    parser = argparse.ArgumentParser(
        description='Run KV Store transport variant CPU benchmarks',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Run all variants (default)
  python transport_cpu_benchmark.py
  
  # Run specific variants
  python transport_cpu_benchmark.py --variants udp reliable cc
  
  # Run only flow control variants
  python transport_cpu_benchmark.py --variants fc cc-fc reliable-fc
  
  # Run all variants with congestion control
  python transport_cpu_benchmark.py --variants cc reliable-cc cc-fc reliable-cc-fc
  
  # Run the full-featured variant
  python transport_cpu_benchmark.py --variants reliable-cc-fc reliable-cc-fc-encryption
  
  # Customize benchmark parameters
  python transport_cpu_benchmark.py --target-rate 2000 --duration 90
        """
    )
    parser.add_argument(
        '--variants',
        nargs='+',
        choices=ALL_VARIANTS,
        default=ALL_VARIANTS,
        help='Transport variants to test (default: all)'
    )
    parser.add_argument(
        '--target-rate',
        type=int,
        default=1000,
        help='Target requests per second (default: 1000)'
    )
    parser.add_argument(
        '--duration',
        type=int,
        default=60,
        help='Duration of wrk2 benchmark in seconds (default: 60)'
    )
    parser.add_argument(
        '--mpstat-duration',
        type=int,
        default=30,
        help='Duration of mpstat measurement in seconds (default: 30)'
    )
    return parser.parse_args()

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
    cores_per_node=64,
    mpstat_duration=30,
    wrk2_duration=60,
    target_rate=1000,
):
    """Run wrk2 and collect CPU metrics."""
    logger.info(f"Running wrk2 for {manifest_name} with target_rate={target_rate}")
    
    cmd = [
        wrk_path,
        "-t", "10",
        "-c", "100",
        "http://10.96.88.88:80",
        "-d", str(wrk2_duration),
        "-R", str(int(target_rate)),
        "-s", lua_path,
    ]
    logger.info(f"Command: {' '.join(cmd)}")
    
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
    logger.debug(f"wrk2 output:\n{output}")

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
        "target_rate": int(target_rate),
    }

    logger.info(
        f"CPU metrics for {manifest_name}: "
        f"average_vcores={cpu_metrics['average_vcores']}, "
        f"median_vcores={cpu_metrics['median_vcores']}, "
        f"recorded_rps={cpu_metrics['recorded_rps']}"
    )

    return {
        "cpu_metrics": cpu_metrics,
        "error_count": error_count,
    }

def run_transport_cpu_benchmark(manifest_name, manifest_path, target_rate, wrk2_duration, mpstat_duration):
    """Run CPU benchmark for a transport variant."""
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
    
    # Step 4: Wait a bit before running benchmark
    time.sleep(15)
    
    # Step 5: Run wrk2 and collect CPU metrics
    wrk_result = run_wrk2_and_get_cpu(
        node_names,
        manifest_name,
        mpstat_duration=mpstat_duration,
        wrk2_duration=wrk2_duration,
        target_rate=target_rate,
    )
    
    # Step 6: Cleanup
    cleanup_manifest(manifest_path)
    
    return wrk_result

def main():
    """Main function to run the benchmark."""
    # Parse command line arguments
    args = parse_arguments()
    
    # Filter manifest_dict based on selected variants
    selected_manifests = {}
    for variant in args.variants:
        key = f"kv-store-symphony-transport-{variant}"
        if key in manifest_dict:
            selected_manifests[key] = manifest_dict[key]
    
    # Check if all selected manifests exist
    for manifest_name, manifest_path in selected_manifests.items():
        if not os.path.exists(manifest_path):
            logger.error(f"Manifest {manifest_path} does not exist")
            exit(1)
        logger.info(f"Manifest {manifest_path} exists")
    
    # Store results for all manifests
    all_results = {}
    
    # Log script start
    logger.info("=" * 60)
    logger.info("Starting Transport CPU Benchmark Suite")
    logger.info(f"Testing {len(selected_manifests)} transport configurations: {', '.join(args.variants)}")
    logger.info(f"Target rate: {args.target_rate} RPS, Duration: {args.duration}s, mpstat: {args.mpstat_duration}s")
    logger.info("=" * 60)
    logger.info("")
    
    # Iterate over each manifest
    for manifest_name, manifest_path in selected_manifests.items():
        logger.info("")
        logger.info("=" * 60)
        logger.info(f"Testing: {manifest_name}")
        logger.info("=" * 60)
        logger.info("")
        
        result = run_transport_cpu_benchmark(
            manifest_name,
            manifest_path,
            args.target_rate,
            args.duration,
            args.mpstat_duration,
        )
        
        if result is not None:
            if result.get("error_count", 0) > 0:
                all_results[manifest_name] = {
                    "status": "failure",
                    "cpu_metrics": result.get("cpu_metrics", {}),
                    "error_count": result.get("error_count", 0),
                }
            else:
                all_results[manifest_name] = {
                    "status": "success",
                    "cpu_metrics": result.get("cpu_metrics", {}),
                    "error_count": result.get("error_count", 0),
                }
        else:
            all_results[manifest_name] = {
                "status": "failed"
            }
        
        logger.info(f"Transport CPU benchmark for {manifest_name} completed")
        
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
            cpu_metrics = result.get("cpu_metrics", {})
            logger.info(f"  Status: {status}")
            if cpu_metrics:
                logger.info(f"  average_vcores: {cpu_metrics.get('average_vcores', 'N/A')}")
                logger.info(f"  median_vcores: {cpu_metrics.get('median_vcores', 'N/A')}")
                logger.info(f"  recorded_rps: {cpu_metrics.get('recorded_rps', 'N/A')}")
                logger.info(f"  target_rate: {cpu_metrics.get('target_rate', 'N/A')}")
        elif status == "failure":
            cpu_metrics = result.get("cpu_metrics", {})
            error_count = result.get("error_count", 0)
            logger.error(f"  Status: {status}")
            logger.error(f"  Non-2xx or 3xx responses: {error_count}")
            if cpu_metrics:
                logger.info(f"  average_vcores: {cpu_metrics.get('average_vcores', 'N/A')}")
                logger.info(f"  median_vcores: {cpu_metrics.get('median_vcores', 'N/A')}")
                logger.info(f"  recorded_rps: {cpu_metrics.get('recorded_rps', 'N/A')}")
                logger.info(f"  target_rate: {cpu_metrics.get('target_rate', 'N/A')}")
        else:
            logger.info(f"  Status: {status}")
        logger.info("")
    
    # Save results to a file
    results_file = os.path.join(log_dir, f"transport_cpu_benchmark_results_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json")
    with open(results_file, "w") as f:
        json.dump(all_results, f, indent=2)
    logger.info(f"Results saved to {results_file}")

if __name__ == "__main__":
    main()

