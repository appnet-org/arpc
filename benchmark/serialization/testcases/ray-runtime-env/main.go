package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"capnproto.org/go/capnp/v3"

	cp "github.com/appnet-org/arpc/benchmark/serialization/capnp"
	fb "github.com/appnet-org/arpc/benchmark/serialization/flatbuffers/benchmark"
	pb "github.com/appnet-org/arpc/benchmark/serialization/protobuf"
	syn "github.com/appnet-org/arpc/benchmark/serialization/symphony"
	flatbuffers "github.com/google/flatbuffers/go"
	"google.golang.org/protobuf/proto"
)

func main() {
	// Create benchmark results directory
	resultsDir := "results"
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		log.Fatalf("failed to create results directory: %v", err)
	}

	fmt.Println("Serialization Format Comparison")
	fmt.Println("================================")

	// Test data
	serializedRuntimeEnv := "{'pip': ['numpy==1.24.0', 'pandas==2.0.1', 'scikit-learn==1.2.2', 'tensorflow==2.12.0', 'torch==2.0.0'], 'env_vars': {'CUDA_VISIBLE_DEVICES': '0,1', 'OMP_NUM_THREADS': '8'}, 'working_dir': '/home/ray/project'}"
	workingDirUri := "s3://ray-runtime-environments/production/project-v1.2.3-20231015-abc123def456.tar.gz"
	pyModulesUris := []string{
		"s3://ray-modules/data-processing/pandas-wrapper-v2.1.0.zip",
		"s3://ray-modules/ml-models/custom-transformer-v1.5.2.zip",
		"s3://ray-modules/utils/logging-helpers-v3.0.1.zip",
		"s3://ray-modules/connectors/database-client-v4.2.0.zip",
		"s3://ray-modules/visualization/plotting-tools-v1.8.3.zip",
	}
	setupTimeoutSeconds := int32(1800)
	eagerInstall := true
	logFiles := []string{
		"/var/log/ray/runtime_env/setup_2024_01_15_143022.log",
		"/var/log/ray/runtime_env/pip_install_2024_01_15_143022.log",
		"/var/log/ray/runtime_env/conda_2024_01_15_143022.log",
		"/tmp/ray/session_latest/runtime_env_setup.out",
		"/tmp/ray/session_latest/runtime_env_setup.err",
	}

	fmt.Printf("Test data:\n")
	fmt.Printf("  serialized_runtime_env: %s\n", serializedRuntimeEnv)
	fmt.Printf("  working_dir_uri: %s\n", workingDirUri)
	fmt.Printf("  py_modules_uris: %v\n", pyModulesUris)
	fmt.Printf("  setup_timeout_seconds: %d\n", setupTimeoutSeconds)
	fmt.Printf("  eager_install: %v\n", eagerInstall)
	fmt.Printf("  log_files: %v\n\n", logFiles)

	// Test each serialization format
	testProtobuf(serializedRuntimeEnv, workingDirUri, pyModulesUris, setupTimeoutSeconds, eagerInstall, logFiles)
	testSymphony(serializedRuntimeEnv, workingDirUri, pyModulesUris, setupTimeoutSeconds, eagerInstall, logFiles)
	testCapnProto(serializedRuntimeEnv, workingDirUri, pyModulesUris, setupTimeoutSeconds, eagerInstall, logFiles)
	testFlatBuffers(serializedRuntimeEnv, workingDirUri, pyModulesUris, setupTimeoutSeconds, eagerInstall, logFiles)

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("PERFORMANCE MEASUREMENTS")
	fmt.Println(strings.Repeat("=", 50))

	// Time measurements
	fmt.Println("\n--- MARSHAL TIME MEASUREMENTS ---")
	measureMarshalTime(serializedRuntimeEnv, workingDirUri, pyModulesUris, setupTimeoutSeconds, eagerInstall, logFiles, resultsDir)

	fmt.Println("\n--- UNMARSHAL TIME MEASUREMENTS ---")
	measureUnmarshalTime(serializedRuntimeEnv, workingDirUri, pyModulesUris, setupTimeoutSeconds, eagerInstall, logFiles, resultsDir)

	// CPU measurements
	fmt.Println("\n--- MARSHAL CPU USAGE MEASUREMENTS ---")
	measureMarshalCPU(serializedRuntimeEnv, workingDirUri, pyModulesUris, setupTimeoutSeconds, eagerInstall, logFiles, resultsDir)

	fmt.Println("\n--- UNMARSHAL CPU USAGE MEASUREMENTS ---")
	measureUnmarshalCPU(serializedRuntimeEnv, workingDirUri, pyModulesUris, setupTimeoutSeconds, eagerInstall, logFiles, resultsDir)

	fmt.Printf("\n✓ All benchmark results saved to: %s/\n", resultsDir)
}

func testProtobuf(serializedRuntimeEnv, workingDirUri string, pyModulesUris []string, setupTimeoutSeconds int32, eagerInstall bool, logFiles []string) {
	fmt.Println("=== Protobuf ===")

	// Marshal
	pbReq := &pb.RuntimeEnvInfo{
		SerializedRuntimeEnv: serializedRuntimeEnv,
		Uris: &pb.RuntimeEnvUris{
			WorkingDirUri:  workingDirUri,
			PyModulesUris:  pyModulesUris,
		},
		RuntimeEnvConfig: &pb.RuntimeEnvConfig{
			SetupTimeoutSeconds: setupTimeoutSeconds,
			EagerInstall:        eagerInstall,
			LogFiles:            logFiles,
		},
	}

	pbBytes, err := proto.Marshal(pbReq)
	if err != nil {
		log.Fatalf("protobuf marshal error: %v", err)
	}

	fmt.Printf("Marshaled size: %d bytes\n", len(pbBytes))
	fmt.Printf("Marshaled data: %x\n", pbBytes)

	// Unmarshal
	pbDecoded := &pb.RuntimeEnvInfo{}
	err = proto.Unmarshal(pbBytes, pbDecoded)
	if err != nil {
		log.Fatalf("protobuf unmarshal error: %v", err)
	}

	fmt.Printf("Unmarshaled:\n")
	fmt.Printf("  serialized_runtime_env: %s\n", pbDecoded.SerializedRuntimeEnv)
	fmt.Printf("  uris.working_dir_uri: %s\n", pbDecoded.Uris.WorkingDirUri)
	fmt.Printf("  uris.py_modules_uris: %v\n", pbDecoded.Uris.PyModulesUris)
	fmt.Printf("  config.setup_timeout_seconds: %d\n", pbDecoded.RuntimeEnvConfig.SetupTimeoutSeconds)
	fmt.Printf("  config.eager_install: %v\n", pbDecoded.RuntimeEnvConfig.EagerInstall)
	fmt.Printf("  config.log_files: %v\n", pbDecoded.RuntimeEnvConfig.LogFiles)

	// Verify correctness
	if pbDecoded.SerializedRuntimeEnv == serializedRuntimeEnv &&
		pbDecoded.Uris.WorkingDirUri == workingDirUri &&
		len(pbDecoded.Uris.PyModulesUris) == len(pyModulesUris) &&
		pbDecoded.RuntimeEnvConfig.SetupTimeoutSeconds == setupTimeoutSeconds &&
		pbDecoded.RuntimeEnvConfig.EagerInstall == eagerInstall &&
		len(pbDecoded.RuntimeEnvConfig.LogFiles) == len(logFiles) {
		fmt.Println("✓ Round-trip successful")
	} else {
		fmt.Println("✗ Round-trip failed")
	}
	fmt.Println()
}

func testSymphony(serializedRuntimeEnv, workingDirUri string, pyModulesUris []string, setupTimeoutSeconds int32, eagerInstall bool, logFiles []string) {
	fmt.Println("=== Symphony ===")

	// Marshal
	synReq := &syn.RuntimeEnvInfo{
		SerializedRuntimeEnv: serializedRuntimeEnv,
		Uris: &syn.RuntimeEnvUris{
			WorkingDirUri:  workingDirUri,
			PyModulesUris:  pyModulesUris,
		},
		RuntimeEnvConfig: &syn.RuntimeEnvConfig{
			SetupTimeoutSeconds: setupTimeoutSeconds,
			EagerInstall:        eagerInstall,
			LogFiles:            logFiles,
		},
	}

	synBytes, err := synReq.MarshalSymphony()
	if err != nil {
		log.Fatalf("symphony marshal error: %v", err)
	}

	fmt.Printf("Marshaled size: %d bytes\n", len(synBytes))
	fmt.Printf("Marshaled data: %x\n", synBytes)

	// Unmarshal
	synDecoded := &syn.RuntimeEnvInfo{}
	err = synDecoded.UnmarshalSymphony(synBytes)
	if err != nil {
		log.Fatalf("symphony unmarshal error: %v", err)
	}

	fmt.Printf("Unmarshaled:\n")
	fmt.Printf("  serialized_runtime_env: %s\n", synDecoded.SerializedRuntimeEnv)
	fmt.Printf("  uris.working_dir_uri: %s\n", synDecoded.Uris.WorkingDirUri)
	fmt.Printf("  uris.py_modules_uris: %v\n", synDecoded.Uris.PyModulesUris)
	fmt.Printf("  config.setup_timeout_seconds: %d\n", synDecoded.RuntimeEnvConfig.SetupTimeoutSeconds)
	fmt.Printf("  config.eager_install: %v\n", synDecoded.RuntimeEnvConfig.EagerInstall)
	fmt.Printf("  config.log_files: %v\n", synDecoded.RuntimeEnvConfig.LogFiles)

	// Verify correctness
	if synDecoded.SerializedRuntimeEnv == serializedRuntimeEnv &&
		synDecoded.Uris.WorkingDirUri == workingDirUri &&
		len(synDecoded.Uris.PyModulesUris) == len(pyModulesUris) &&
		synDecoded.RuntimeEnvConfig.SetupTimeoutSeconds == setupTimeoutSeconds &&
		synDecoded.RuntimeEnvConfig.EagerInstall == eagerInstall &&
		len(synDecoded.RuntimeEnvConfig.LogFiles) == len(logFiles) {
		fmt.Println("✓ Round-trip successful")
	} else {
		fmt.Println("✗ Round-trip failed")
	}
	fmt.Println()
}

func testCapnProto(serializedRuntimeEnv, workingDirUri string, pyModulesUris []string, setupTimeoutSeconds int32, eagerInstall bool, logFiles []string) {
	fmt.Println("=== Cap'n Proto ===")

	// Marshal
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		log.Fatalf("capnp message creation error: %v", err)
	}

	cpReq, err := cp.NewRootRuntimeEnvInfo(seg)
	if err != nil {
		log.Fatalf("capnp root error: %v", err)
	}

	cpReq.SetSerializedRuntimeEnv(serializedRuntimeEnv)

	// Set uris
	uris, err := cpReq.NewUris()
	if err != nil {
		log.Fatalf("capnp new uris error: %v", err)
	}
	uris.SetWorkingDirUri(workingDirUri)

	pyModulesList, err := uris.NewPyModulesUris(int32(len(pyModulesUris)))
	if err != nil {
		log.Fatalf("capnp new py modules list error: %v", err)
	}
	for i, uri := range pyModulesUris {
		pyModulesList.Set(i, uri)
	}

	// Set config
	config, err := cpReq.NewRuntimeEnvConfig()
	if err != nil {
		log.Fatalf("capnp new config error: %v", err)
	}
	config.SetSetupTimeoutSeconds(setupTimeoutSeconds)
	config.SetEagerInstall(eagerInstall)

	logFilesList, err := config.NewLogFiles(int32(len(logFiles)))
	if err != nil {
		log.Fatalf("capnp new log files list error: %v", err)
	}
	for i, logFile := range logFiles {
		logFilesList.Set(i, logFile)
	}

	cpBytes, err := msg.Marshal()
	if err != nil {
		log.Fatalf("capnp marshal error: %v", err)
	}

	fmt.Printf("Marshaled size: %d bytes\n", len(cpBytes))
	fmt.Printf("Marshaled data: %x\n", cpBytes)

	// Unmarshal
	cpMsg, err := capnp.Unmarshal(cpBytes)
	if err != nil {
		log.Fatalf("capnp unmarshal error: %v", err)
	}

	cpDecoded, err := cp.ReadRootRuntimeEnvInfo(cpMsg)
	if err != nil {
		log.Fatalf("capnp read root error: %v", err)
	}

	decodedSerializedRuntimeEnv, err := cpDecoded.SerializedRuntimeEnv()
	if err != nil {
		log.Fatalf("capnp read serialized_runtime_env error: %v", err)
	}

	decodedUris, err := cpDecoded.Uris()
	if err != nil {
		log.Fatalf("capnp read uris error: %v", err)
	}

	decodedWorkingDirUri, err := decodedUris.WorkingDirUri()
	if err != nil {
		log.Fatalf("capnp read working_dir_uri error: %v", err)
	}

	decodedPyModulesUris, err := decodedUris.PyModulesUris()
	if err != nil {
		log.Fatalf("capnp read py_modules_uris error: %v", err)
	}

	decodedConfig, err := cpDecoded.RuntimeEnvConfig()
	if err != nil {
		log.Fatalf("capnp read config error: %v", err)
	}

	decodedLogFiles, err := decodedConfig.LogFiles()
	if err != nil {
		log.Fatalf("capnp read log_files error: %v", err)
	}

	fmt.Printf("Unmarshaled:\n")
	fmt.Printf("  serialized_runtime_env: %s\n", decodedSerializedRuntimeEnv)
	fmt.Printf("  uris.working_dir_uri: %s\n", decodedWorkingDirUri)
	fmt.Printf("  uris.py_modules_uris length: %d\n", decodedPyModulesUris.Len())
	fmt.Printf("  config.setup_timeout_seconds: %d\n", decodedConfig.SetupTimeoutSeconds())
	fmt.Printf("  config.eager_install: %v\n", decodedConfig.EagerInstall())
	fmt.Printf("  config.log_files length: %d\n", decodedLogFiles.Len())

	// Verify correctness
	if decodedSerializedRuntimeEnv == serializedRuntimeEnv &&
		decodedWorkingDirUri == workingDirUri &&
		decodedPyModulesUris.Len() == len(pyModulesUris) &&
		decodedConfig.SetupTimeoutSeconds() == setupTimeoutSeconds &&
		decodedConfig.EagerInstall() == eagerInstall &&
		decodedLogFiles.Len() == len(logFiles) {
		fmt.Println("✓ Round-trip successful")
	} else {
		fmt.Println("✗ Round-trip failed")
	}
	fmt.Println()
}

func testFlatBuffers(serializedRuntimeEnv, workingDirUri string, pyModulesUris []string, setupTimeoutSeconds int32, eagerInstall bool, logFiles []string) {
	fmt.Println("=== FlatBuffers ===")

	// Marshal
	builder := flatbuffers.NewBuilder(0)

	// Create strings
	serializedRuntimeEnvOffset := builder.CreateString(serializedRuntimeEnv)
	workingDirUriOffset := builder.CreateString(workingDirUri)

	// Create py_modules_uris array
	pyModulesOffsets := make([]flatbuffers.UOffsetT, len(pyModulesUris))
	for i, uri := range pyModulesUris {
		pyModulesOffsets[i] = builder.CreateString(uri)
	}
	fb.RuntimeEnvUrisStartPyModulesUrisVector(builder, len(pyModulesUris))
	for i := len(pyModulesOffsets) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(pyModulesOffsets[i])
	}
	pyModulesVector := builder.EndVector(len(pyModulesUris))

	// Create log_files array
	logFilesOffsets := make([]flatbuffers.UOffsetT, len(logFiles))
	for i, logFile := range logFiles {
		logFilesOffsets[i] = builder.CreateString(logFile)
	}
	fb.RuntimeEnvConfigStartLogFilesVector(builder, len(logFiles))
	for i := len(logFilesOffsets) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(logFilesOffsets[i])
	}
	logFilesVector := builder.EndVector(len(logFiles))

	// Create RuntimeEnvUris
	fb.RuntimeEnvUrisStart(builder)
	fb.RuntimeEnvUrisAddWorkingDirUri(builder, workingDirUriOffset)
	fb.RuntimeEnvUrisAddPyModulesUris(builder, pyModulesVector)
	urisOffset := fb.RuntimeEnvUrisEnd(builder)

	// Create RuntimeEnvConfig
	fb.RuntimeEnvConfigStart(builder)
	fb.RuntimeEnvConfigAddSetupTimeoutSeconds(builder, setupTimeoutSeconds)
	fb.RuntimeEnvConfigAddEagerInstall(builder, eagerInstall)
	fb.RuntimeEnvConfigAddLogFiles(builder, logFilesVector)
	configOffset := fb.RuntimeEnvConfigEnd(builder)

	// Create RuntimeEnvInfo
	fb.RuntimeEnvInfoStart(builder)
	fb.RuntimeEnvInfoAddSerializedRuntimeEnv(builder, serializedRuntimeEnvOffset)
	fb.RuntimeEnvInfoAddUris(builder, urisOffset)
	fb.RuntimeEnvInfoAddRuntimeEnvConfig(builder, configOffset)
	fbReq := fb.RuntimeEnvInfoEnd(builder)
	builder.Finish(fbReq)

	fbBytes := builder.FinishedBytes()

	fmt.Printf("Marshaled size: %d bytes\n", len(fbBytes))
	fmt.Printf("Marshaled data: %x\n", fbBytes)

	// Unmarshal
	fbDecoded := fb.GetRootAsRuntimeEnvInfo(fbBytes, 0)

	decodedSerializedRuntimeEnv := string(fbDecoded.SerializedRuntimeEnv())

	decodedUris := fbDecoded.Uris(nil)
	decodedWorkingDirUri := string(decodedUris.WorkingDirUri())
	pyModulesCount := decodedUris.PyModulesUrisLength()

	decodedConfig := fbDecoded.RuntimeEnvConfig(nil)
	decodedSetupTimeoutSeconds := decodedConfig.SetupTimeoutSeconds()
	decodedEagerInstall := decodedConfig.EagerInstall()
	logFilesCount := decodedConfig.LogFilesLength()

	fmt.Printf("Unmarshaled:\n")
	fmt.Printf("  serialized_runtime_env: %s\n", decodedSerializedRuntimeEnv)
	fmt.Printf("  uris.working_dir_uri: %s\n", decodedWorkingDirUri)
	fmt.Printf("  uris.py_modules_uris length: %d\n", pyModulesCount)
	fmt.Printf("  config.setup_timeout_seconds: %d\n", decodedSetupTimeoutSeconds)
	fmt.Printf("  config.eager_install: %v\n", decodedEagerInstall)
	fmt.Printf("  config.log_files length: %d\n", logFilesCount)

	// Verify correctness
	if decodedSerializedRuntimeEnv == serializedRuntimeEnv &&
		decodedWorkingDirUri == workingDirUri &&
		pyModulesCount == len(pyModulesUris) &&
		decodedSetupTimeoutSeconds == setupTimeoutSeconds &&
		decodedEagerInstall == eagerInstall &&
		logFilesCount == len(logFiles) {
		fmt.Println("✓ Round-trip successful")
	} else {
		fmt.Println("✗ Round-trip failed")
	}
	fmt.Println()
}

// measureMarshalTime uses Go's testing.Benchmark for advanced timing measurements
func measureMarshalTime(serializedRuntimeEnv, workingDirUri string, pyModulesUris []string, setupTimeoutSeconds int32, eagerInstall bool, logFiles []string, resultsDir string) {
	fmt.Println("Measuring marshal time using Go's testing.Benchmark:")

	// Create timing results log file
	logFile, err := os.Create(filepath.Join(resultsDir, "marshal_timing_results.log"))
	if err != nil {
		log.Fatalf("could not create timing log file: %v", err)
	}
	defer logFile.Close()

	logFile.WriteString("=== MARSHAL TIME MEASUREMENTS ===\n")
	logFile.WriteString(fmt.Sprintf("Test data: serialized_runtime_env=%s, working_dir_uri=%s\n", serializedRuntimeEnv, workingDirUri))
	logFile.WriteString(fmt.Sprintf("Timestamp: %s\n\n", time.Now().Format(time.RFC3339)))

	// Protobuf benchmark
	pbResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pbReq := &pb.RuntimeEnvInfo{
				SerializedRuntimeEnv: serializedRuntimeEnv,
				Uris: &pb.RuntimeEnvUris{
					WorkingDirUri:  workingDirUri,
					PyModulesUris:  pyModulesUris,
				},
				RuntimeEnvConfig: &pb.RuntimeEnvConfig{
					SetupTimeoutSeconds: setupTimeoutSeconds,
					EagerInstall:        eagerInstall,
					LogFiles:            logFiles,
				},
			}
			_, _ = proto.Marshal(pbReq)
		}
	})
	pbResultStr := formatBenchmarkResult(pbResult)
	fmt.Printf("Protobuf:    %s\n", pbResultStr)
	logFile.WriteString(fmt.Sprintf("Protobuf:    %s\n", pbResultStr))

	// Symphony benchmark
	synResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			synReq := &syn.RuntimeEnvInfo{
				SerializedRuntimeEnv: serializedRuntimeEnv,
				Uris: &syn.RuntimeEnvUris{
					WorkingDirUri:  workingDirUri,
					PyModulesUris:  pyModulesUris,
				},
				RuntimeEnvConfig: &syn.RuntimeEnvConfig{
					SetupTimeoutSeconds: setupTimeoutSeconds,
					EagerInstall:        eagerInstall,
					LogFiles:            logFiles,
				},
			}
			_, _ = synReq.MarshalSymphony()
		}
	})
	synResultStr := formatBenchmarkResult(synResult)
	fmt.Printf("Symphony:    %s\n", synResultStr)
	logFile.WriteString(fmt.Sprintf("Symphony:    %s\n", synResultStr))

	// Cap'n Proto benchmark
	cpResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			cpReq, _ := cp.NewRootRuntimeEnvInfo(seg)
			cpReq.SetSerializedRuntimeEnv(serializedRuntimeEnv)

			uris, _ := cpReq.NewUris()
			uris.SetWorkingDirUri(workingDirUri)
			pyModulesList, _ := uris.NewPyModulesUris(int32(len(pyModulesUris)))
			for i, uri := range pyModulesUris {
				pyModulesList.Set(i, uri)
			}

			config, _ := cpReq.NewRuntimeEnvConfig()
			config.SetSetupTimeoutSeconds(setupTimeoutSeconds)
			config.SetEagerInstall(eagerInstall)
			logFilesList, _ := config.NewLogFiles(int32(len(logFiles)))
			for i, logFile := range logFiles {
				logFilesList.Set(i, logFile)
			}

			_, _ = msg.Marshal()
		}
	})
	cpResultStr := formatBenchmarkResult(cpResult)
	fmt.Printf("Cap'n Proto: %s\n", cpResultStr)
	logFile.WriteString(fmt.Sprintf("Cap'n Proto: %s\n", cpResultStr))

	// FlatBuffers benchmark
	fbResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			builder := flatbuffers.NewBuilder(0)

			serializedRuntimeEnvOffset := builder.CreateString(serializedRuntimeEnv)
			workingDirUriOffset := builder.CreateString(workingDirUri)

			pyModulesOffsets := make([]flatbuffers.UOffsetT, len(pyModulesUris))
			for i, uri := range pyModulesUris {
				pyModulesOffsets[i] = builder.CreateString(uri)
			}
			fb.RuntimeEnvUrisStartPyModulesUrisVector(builder, len(pyModulesUris))
			for i := len(pyModulesOffsets) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(pyModulesOffsets[i])
			}
			pyModulesVector := builder.EndVector(len(pyModulesUris))

			logFilesOffsets := make([]flatbuffers.UOffsetT, len(logFiles))
			for i, logFile := range logFiles {
				logFilesOffsets[i] = builder.CreateString(logFile)
			}
			fb.RuntimeEnvConfigStartLogFilesVector(builder, len(logFiles))
			for i := len(logFilesOffsets) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(logFilesOffsets[i])
			}
			logFilesVector := builder.EndVector(len(logFiles))

			fb.RuntimeEnvUrisStart(builder)
			fb.RuntimeEnvUrisAddWorkingDirUri(builder, workingDirUriOffset)
			fb.RuntimeEnvUrisAddPyModulesUris(builder, pyModulesVector)
			urisOffset := fb.RuntimeEnvUrisEnd(builder)

			fb.RuntimeEnvConfigStart(builder)
			fb.RuntimeEnvConfigAddSetupTimeoutSeconds(builder, setupTimeoutSeconds)
			fb.RuntimeEnvConfigAddEagerInstall(builder, eagerInstall)
			fb.RuntimeEnvConfigAddLogFiles(builder, logFilesVector)
			configOffset := fb.RuntimeEnvConfigEnd(builder)

			fb.RuntimeEnvInfoStart(builder)
			fb.RuntimeEnvInfoAddSerializedRuntimeEnv(builder, serializedRuntimeEnvOffset)
			fb.RuntimeEnvInfoAddUris(builder, urisOffset)
			fb.RuntimeEnvInfoAddRuntimeEnvConfig(builder, configOffset)
			fbReq := fb.RuntimeEnvInfoEnd(builder)
			builder.Finish(fbReq)
			_ = builder.FinishedBytes()
		}
	})
	fbResultStr := formatBenchmarkResult(fbResult)
	fmt.Printf("FlatBuffers: %s\n", fbResultStr)
	logFile.WriteString(fmt.Sprintf("FlatBuffers: %s\n", fbResultStr))

	fmt.Printf("✓ Marshal timing results saved to: %s\n", filepath.Join(resultsDir, "marshal_timing_results.log"))
}

// measureUnmarshalTime uses Go's testing.Benchmark for unmarshal timing measurements
func measureUnmarshalTime(serializedRuntimeEnv, workingDirUri string, pyModulesUris []string, setupTimeoutSeconds int32, eagerInstall bool, logFiles []string, resultsDir string) {
	fmt.Println("Measuring unmarshal time using Go's testing.Benchmark:")

	// Create timing results log file
	logFile, err := os.Create(filepath.Join(resultsDir, "unmarshal_timing_results.log"))
	if err != nil {
		log.Fatalf("could not create unmarshal timing log file: %v", err)
	}
	defer logFile.Close()

	logFile.WriteString("=== UNMARSHAL TIME MEASUREMENTS ===\n")
	logFile.WriteString(fmt.Sprintf("Test data: serialized_runtime_env=%s, working_dir_uri=%s\n", serializedRuntimeEnv, workingDirUri))
	logFile.WriteString(fmt.Sprintf("Timestamp: %s\n\n", time.Now().Format(time.RFC3339)))

	// Prepare serialized data for each format
	pbReq := &pb.RuntimeEnvInfo{
		SerializedRuntimeEnv: serializedRuntimeEnv,
		Uris: &pb.RuntimeEnvUris{
			WorkingDirUri:  workingDirUri,
			PyModulesUris:  pyModulesUris,
		},
		RuntimeEnvConfig: &pb.RuntimeEnvConfig{
			SetupTimeoutSeconds: setupTimeoutSeconds,
			EagerInstall:        eagerInstall,
			LogFiles:            logFiles,
		},
	}
	pbBytes, _ := proto.Marshal(pbReq)

	synReq := &syn.RuntimeEnvInfo{
		SerializedRuntimeEnv: serializedRuntimeEnv,
		Uris: &syn.RuntimeEnvUris{
			WorkingDirUri:  workingDirUri,
			PyModulesUris:  pyModulesUris,
		},
		RuntimeEnvConfig: &syn.RuntimeEnvConfig{
			SetupTimeoutSeconds: setupTimeoutSeconds,
			EagerInstall:        eagerInstall,
			LogFiles:            logFiles,
		},
	}
	synBytes, _ := synReq.MarshalSymphony()

	msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
	cpReq, _ := cp.NewRootRuntimeEnvInfo(seg)
	cpReq.SetSerializedRuntimeEnv(serializedRuntimeEnv)
	uris, _ := cpReq.NewUris()
	uris.SetWorkingDirUri(workingDirUri)
	pyModulesList, _ := uris.NewPyModulesUris(int32(len(pyModulesUris)))
	for i, uri := range pyModulesUris {
		pyModulesList.Set(i, uri)
	}
	config, _ := cpReq.NewRuntimeEnvConfig()
	config.SetSetupTimeoutSeconds(setupTimeoutSeconds)
	config.SetEagerInstall(eagerInstall)
	logFilesList, _ := config.NewLogFiles(int32(len(logFiles)))
	for i, logFile := range logFiles {
		logFilesList.Set(i, logFile)
	}
	cpBytes, _ := msg.Marshal()

	builder := flatbuffers.NewBuilder(0)
	serializedRuntimeEnvOffset := builder.CreateString(serializedRuntimeEnv)
	workingDirUriOffset := builder.CreateString(workingDirUri)
	pyModulesOffsets := make([]flatbuffers.UOffsetT, len(pyModulesUris))
	for i, uri := range pyModulesUris {
		pyModulesOffsets[i] = builder.CreateString(uri)
	}
	fb.RuntimeEnvUrisStartPyModulesUrisVector(builder, len(pyModulesUris))
	for i := len(pyModulesOffsets) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(pyModulesOffsets[i])
	}
	pyModulesVector := builder.EndVector(len(pyModulesUris))
	logFilesOffsets := make([]flatbuffers.UOffsetT, len(logFiles))
	for i, logFile := range logFiles {
		logFilesOffsets[i] = builder.CreateString(logFile)
	}
	fb.RuntimeEnvConfigStartLogFilesVector(builder, len(logFiles))
	for i := len(logFilesOffsets) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(logFilesOffsets[i])
	}
	logFilesVector := builder.EndVector(len(logFiles))
	fb.RuntimeEnvUrisStart(builder)
	fb.RuntimeEnvUrisAddWorkingDirUri(builder, workingDirUriOffset)
	fb.RuntimeEnvUrisAddPyModulesUris(builder, pyModulesVector)
	urisOffset := fb.RuntimeEnvUrisEnd(builder)
	fb.RuntimeEnvConfigStart(builder)
	fb.RuntimeEnvConfigAddSetupTimeoutSeconds(builder, setupTimeoutSeconds)
	fb.RuntimeEnvConfigAddEagerInstall(builder, eagerInstall)
	fb.RuntimeEnvConfigAddLogFiles(builder, logFilesVector)
	configOffset := fb.RuntimeEnvConfigEnd(builder)
	fb.RuntimeEnvInfoStart(builder)
	fb.RuntimeEnvInfoAddSerializedRuntimeEnv(builder, serializedRuntimeEnvOffset)
	fb.RuntimeEnvInfoAddUris(builder, urisOffset)
	fb.RuntimeEnvInfoAddRuntimeEnvConfig(builder, configOffset)
	fbReq := fb.RuntimeEnvInfoEnd(builder)
	builder.Finish(fbReq)
	fbBytes := builder.FinishedBytes()

	// Protobuf unmarshal benchmark
	pbResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pbDecoded := &pb.RuntimeEnvInfo{}
			_ = proto.Unmarshal(pbBytes, pbDecoded)
		}
	})
	pbResultStr := formatBenchmarkResult(pbResult)
	fmt.Printf("Protobuf:    %s\n", pbResultStr)
	logFile.WriteString(fmt.Sprintf("Protobuf:    %s\n", pbResultStr))

	// Symphony unmarshal benchmark
	synResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			synDecoded := &syn.RuntimeEnvInfo{}
			_ = synDecoded.UnmarshalSymphony(synBytes)
		}
	})
	synResultStr := formatBenchmarkResult(synResult)
	fmt.Printf("Symphony:    %s\n", synResultStr)
	logFile.WriteString(fmt.Sprintf("Symphony:    %s\n", synResultStr))

	// Cap'n Proto unmarshal benchmark
	cpResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cpMsg, _ := capnp.Unmarshal(cpBytes)
			cpDecoded, _ := cp.ReadRootRuntimeEnvInfo(cpMsg)
			_, _ = cpDecoded.SerializedRuntimeEnv()
			decodedUris, _ := cpDecoded.Uris()
			_, _ = decodedUris.WorkingDirUri()
			_, _ = decodedUris.PyModulesUris()
			decodedConfig, _ := cpDecoded.RuntimeEnvConfig()
			_ = decodedConfig.SetupTimeoutSeconds()
			_ = decodedConfig.EagerInstall()
			_, _ = decodedConfig.LogFiles()
		}
	})
	cpResultStr := formatBenchmarkResult(cpResult)
	fmt.Printf("Cap'n Proto: %s\n", cpResultStr)
	logFile.WriteString(fmt.Sprintf("Cap'n Proto: %s\n", cpResultStr))

	// FlatBuffers unmarshal benchmark
	fbResult := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			fbDecoded := fb.GetRootAsRuntimeEnvInfo(fbBytes, 0)
			_ = string(fbDecoded.SerializedRuntimeEnv())
			decodedUris := fbDecoded.Uris(nil)
			_ = string(decodedUris.WorkingDirUri())
			_ = decodedUris.PyModulesUrisLength()
			decodedConfig := fbDecoded.RuntimeEnvConfig(nil)
			_ = decodedConfig.SetupTimeoutSeconds()
			_ = decodedConfig.EagerInstall()
			_ = decodedConfig.LogFilesLength()
		}
	})
	fbResultStr := formatBenchmarkResult(fbResult)
	fmt.Printf("FlatBuffers: %s\n", fbResultStr)
	logFile.WriteString(fmt.Sprintf("FlatBuffers: %s\n", fbResultStr))

	fmt.Printf("✓ Unmarshal timing results saved to: %s\n", filepath.Join(resultsDir, "unmarshal_timing_results.log"))
}

// formatBenchmarkResult formats testing.BenchmarkResult for display
func formatBenchmarkResult(result testing.BenchmarkResult) string {
	nsPerOp := result.NsPerOp()
	allocsPerOp := result.AllocsPerOp()
	bytesPerOp := result.AllocedBytesPerOp()

	return fmt.Sprintf("%d iterations, %s/op, %d allocs/op, %d B/op",
		result.N,
		formatDuration(time.Duration(nsPerOp)),
		allocsPerOp,
		bytesPerOp)
}

// formatDuration formats duration with appropriate units
func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%dns", d.Nanoseconds())
}

// measureMarshalCPU uses Go's CPU profiler to generate CPU profiles for marshal operations
func measureMarshalCPU(serializedRuntimeEnv, workingDirUri string, pyModulesUris []string, setupTimeoutSeconds int32, eagerInstall bool, logFiles []string, resultsDir string) {
	const iterations = 200000 // More iterations for better CPU profiling

	fmt.Println("Measuring CPU usage using Go's CPU profiler:")
	fmt.Println("Generating CPU profile files for analysis...")

	// Profile Protobuf
	profileMarshalFunction(filepath.Join(resultsDir, "protobuf_marshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			pbReq := &pb.RuntimeEnvInfo{
				SerializedRuntimeEnv: serializedRuntimeEnv,
				Uris: &pb.RuntimeEnvUris{
					WorkingDirUri:  workingDirUri,
					PyModulesUris:  pyModulesUris,
				},
				RuntimeEnvConfig: &pb.RuntimeEnvConfig{
					SetupTimeoutSeconds: setupTimeoutSeconds,
					EagerInstall:        eagerInstall,
					LogFiles:            logFiles,
				},
			}
			_, _ = proto.Marshal(pbReq)
		}
	})
	fmt.Printf("✓ Protobuf CPU profile saved to: %s\n", filepath.Join(resultsDir, "protobuf_marshal.prof"))

	// Profile Symphony
	profileMarshalFunction(filepath.Join(resultsDir, "symphony_marshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			synReq := &syn.RuntimeEnvInfo{
				SerializedRuntimeEnv: serializedRuntimeEnv,
				Uris: &syn.RuntimeEnvUris{
					WorkingDirUri:  workingDirUri,
					PyModulesUris:  pyModulesUris,
				},
				RuntimeEnvConfig: &syn.RuntimeEnvConfig{
					SetupTimeoutSeconds: setupTimeoutSeconds,
					EagerInstall:        eagerInstall,
					LogFiles:            logFiles,
				},
			}
			_, _ = synReq.MarshalSymphony()
		}
	})
	fmt.Printf("✓ Symphony CPU profile saved to: %s\n", filepath.Join(resultsDir, "symphony_marshal.prof"))

	// Profile Cap'n Proto
	profileMarshalFunction(filepath.Join(resultsDir, "capnproto_marshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
			cpReq, _ := cp.NewRootRuntimeEnvInfo(seg)
			cpReq.SetSerializedRuntimeEnv(serializedRuntimeEnv)

			uris, _ := cpReq.NewUris()
			uris.SetWorkingDirUri(workingDirUri)
			pyModulesList, _ := uris.NewPyModulesUris(int32(len(pyModulesUris)))
			for i, uri := range pyModulesUris {
				pyModulesList.Set(i, uri)
			}

			config, _ := cpReq.NewRuntimeEnvConfig()
			config.SetSetupTimeoutSeconds(setupTimeoutSeconds)
			config.SetEagerInstall(eagerInstall)
			logFilesList, _ := config.NewLogFiles(int32(len(logFiles)))
			for i, logFile := range logFiles {
				logFilesList.Set(i, logFile)
			}

			_, _ = msg.Marshal()
		}
	})
	fmt.Printf("✓ Cap'n Proto CPU profile saved to: %s\n", filepath.Join(resultsDir, "capnproto_marshal.prof"))

	// Profile FlatBuffers
	profileMarshalFunction(filepath.Join(resultsDir, "flatbuffers_marshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			builder := flatbuffers.NewBuilder(0)

			serializedRuntimeEnvOffset := builder.CreateString(serializedRuntimeEnv)
			workingDirUriOffset := builder.CreateString(workingDirUri)

			pyModulesOffsets := make([]flatbuffers.UOffsetT, len(pyModulesUris))
			for i, uri := range pyModulesUris {
				pyModulesOffsets[i] = builder.CreateString(uri)
			}
			fb.RuntimeEnvUrisStartPyModulesUrisVector(builder, len(pyModulesUris))
			for i := len(pyModulesOffsets) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(pyModulesOffsets[i])
			}
			pyModulesVector := builder.EndVector(len(pyModulesUris))

			logFilesOffsets := make([]flatbuffers.UOffsetT, len(logFiles))
			for i, logFile := range logFiles {
				logFilesOffsets[i] = builder.CreateString(logFile)
			}
			fb.RuntimeEnvConfigStartLogFilesVector(builder, len(logFiles))
			for i := len(logFilesOffsets) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(logFilesOffsets[i])
			}
			logFilesVector := builder.EndVector(len(logFiles))

			fb.RuntimeEnvUrisStart(builder)
			fb.RuntimeEnvUrisAddWorkingDirUri(builder, workingDirUriOffset)
			fb.RuntimeEnvUrisAddPyModulesUris(builder, pyModulesVector)
			urisOffset := fb.RuntimeEnvUrisEnd(builder)

			fb.RuntimeEnvConfigStart(builder)
			fb.RuntimeEnvConfigAddSetupTimeoutSeconds(builder, setupTimeoutSeconds)
			fb.RuntimeEnvConfigAddEagerInstall(builder, eagerInstall)
			fb.RuntimeEnvConfigAddLogFiles(builder, logFilesVector)
			configOffset := fb.RuntimeEnvConfigEnd(builder)

			fb.RuntimeEnvInfoStart(builder)
			fb.RuntimeEnvInfoAddSerializedRuntimeEnv(builder, serializedRuntimeEnvOffset)
			fb.RuntimeEnvInfoAddUris(builder, urisOffset)
			fb.RuntimeEnvInfoAddRuntimeEnvConfig(builder, configOffset)
			fbReq := fb.RuntimeEnvInfoEnd(builder)
			builder.Finish(fbReq)
			_ = builder.FinishedBytes()
		}
	})
	fmt.Printf("✓ FlatBuffers CPU profile saved to: %s\n", filepath.Join(resultsDir, "flatbuffers_marshal.prof"))

	fmt.Println("\nTo analyze marshal CPU profiles, use:")
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "protobuf_marshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "symphony_marshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "capnproto_marshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "flatbuffers_marshal.prof"))
	fmt.Println("\nIn pprof, use commands like: top, list, web, png")
}

// profileMarshalFunction profiles a marshal function and saves the CPU profile
func profileMarshalFunction(filename string, marshalFunc func()) {
	// Create profile file
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("could not create CPU profile file %s: %v", filename, err)
	}
	defer f.Close()

	// Start CPU profiling
	if err := pprof.StartCPUProfile(f); err != nil {
		log.Fatalf("could not start CPU profile: %v", err)
	}
	defer pprof.StopCPUProfile()

	// Run the marshal function (this is where CPU profiling happens)
	marshalFunc()
}

// measureUnmarshalCPU uses Go's CPU profiler to generate CPU profiles for unmarshal operations
func measureUnmarshalCPU(serializedRuntimeEnv, workingDirUri string, pyModulesUris []string, setupTimeoutSeconds int32, eagerInstall bool, logFiles []string, resultsDir string) {
	const iterations = 200000 // More iterations for better CPU profiling

	fmt.Println("Measuring unmarshal CPU usage using Go's CPU profiler:")
	fmt.Println("Generating unmarshal CPU profile files for analysis...")

	// Prepare serialized data for each format
	pbReq := &pb.RuntimeEnvInfo{
		SerializedRuntimeEnv: serializedRuntimeEnv,
		Uris: &pb.RuntimeEnvUris{
			WorkingDirUri:  workingDirUri,
			PyModulesUris:  pyModulesUris,
		},
		RuntimeEnvConfig: &pb.RuntimeEnvConfig{
			SetupTimeoutSeconds: setupTimeoutSeconds,
			EagerInstall:        eagerInstall,
			LogFiles:            logFiles,
		},
	}
	pbBytes, _ := proto.Marshal(pbReq)

	synReq := &syn.RuntimeEnvInfo{
		SerializedRuntimeEnv: serializedRuntimeEnv,
		Uris: &syn.RuntimeEnvUris{
			WorkingDirUri:  workingDirUri,
			PyModulesUris:  pyModulesUris,
		},
		RuntimeEnvConfig: &syn.RuntimeEnvConfig{
			SetupTimeoutSeconds: setupTimeoutSeconds,
			EagerInstall:        eagerInstall,
			LogFiles:            logFiles,
		},
	}
	synBytes, _ := synReq.MarshalSymphony()

	msg, seg, _ := capnp.NewMessage(capnp.SingleSegment(nil))
	cpReq, _ := cp.NewRootRuntimeEnvInfo(seg)
	cpReq.SetSerializedRuntimeEnv(serializedRuntimeEnv)
	uris, _ := cpReq.NewUris()
	uris.SetWorkingDirUri(workingDirUri)
	pyModulesList, _ := uris.NewPyModulesUris(int32(len(pyModulesUris)))
	for i, uri := range pyModulesUris {
		pyModulesList.Set(i, uri)
	}
	config, _ := cpReq.NewRuntimeEnvConfig()
	config.SetSetupTimeoutSeconds(setupTimeoutSeconds)
	config.SetEagerInstall(eagerInstall)
	logFilesList, _ := config.NewLogFiles(int32(len(logFiles)))
	for i, logFile := range logFiles {
		logFilesList.Set(i, logFile)
	}
	cpBytes, _ := msg.Marshal()

	builder := flatbuffers.NewBuilder(0)
	serializedRuntimeEnvOffset := builder.CreateString(serializedRuntimeEnv)
	workingDirUriOffset := builder.CreateString(workingDirUri)
	pyModulesOffsets := make([]flatbuffers.UOffsetT, len(pyModulesUris))
	for i, uri := range pyModulesUris {
		pyModulesOffsets[i] = builder.CreateString(uri)
	}
	fb.RuntimeEnvUrisStartPyModulesUrisVector(builder, len(pyModulesUris))
	for i := len(pyModulesOffsets) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(pyModulesOffsets[i])
	}
	pyModulesVector := builder.EndVector(len(pyModulesUris))
	logFilesOffsets := make([]flatbuffers.UOffsetT, len(logFiles))
	for i, logFile := range logFiles {
		logFilesOffsets[i] = builder.CreateString(logFile)
	}
	fb.RuntimeEnvConfigStartLogFilesVector(builder, len(logFiles))
	for i := len(logFilesOffsets) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(logFilesOffsets[i])
	}
	logFilesVector := builder.EndVector(len(logFiles))
	fb.RuntimeEnvUrisStart(builder)
	fb.RuntimeEnvUrisAddWorkingDirUri(builder, workingDirUriOffset)
	fb.RuntimeEnvUrisAddPyModulesUris(builder, pyModulesVector)
	urisOffset := fb.RuntimeEnvUrisEnd(builder)
	fb.RuntimeEnvConfigStart(builder)
	fb.RuntimeEnvConfigAddSetupTimeoutSeconds(builder, setupTimeoutSeconds)
	fb.RuntimeEnvConfigAddEagerInstall(builder, eagerInstall)
	fb.RuntimeEnvConfigAddLogFiles(builder, logFilesVector)
	configOffset := fb.RuntimeEnvConfigEnd(builder)
	fb.RuntimeEnvInfoStart(builder)
	fb.RuntimeEnvInfoAddSerializedRuntimeEnv(builder, serializedRuntimeEnvOffset)
	fb.RuntimeEnvInfoAddUris(builder, urisOffset)
	fb.RuntimeEnvInfoAddRuntimeEnvConfig(builder, configOffset)
	fbReq := fb.RuntimeEnvInfoEnd(builder)
	builder.Finish(fbReq)
	fbBytes := builder.FinishedBytes()

	// Profile Protobuf unmarshal
	profileMarshalFunction(filepath.Join(resultsDir, "protobuf_unmarshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			pbDecoded := &pb.RuntimeEnvInfo{}
			_ = proto.Unmarshal(pbBytes, pbDecoded)
		}
	})
	fmt.Printf("✓ Protobuf unmarshal CPU profile saved to: %s\n", filepath.Join(resultsDir, "protobuf_unmarshal.prof"))

	// Profile Symphony unmarshal
	profileMarshalFunction(filepath.Join(resultsDir, "symphony_unmarshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			synDecoded := &syn.RuntimeEnvInfo{}
			_ = synDecoded.UnmarshalSymphony(synBytes)
		}
	})
	fmt.Printf("✓ Symphony unmarshal CPU profile saved to: %s\n", filepath.Join(resultsDir, "symphony_unmarshal.prof"))

	// Profile Cap'n Proto unmarshal
	profileMarshalFunction(filepath.Join(resultsDir, "capnproto_unmarshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			cpMsg, _ := capnp.Unmarshal(cpBytes)
			cpDecoded, _ := cp.ReadRootRuntimeEnvInfo(cpMsg)
			_, _ = cpDecoded.SerializedRuntimeEnv()
			decodedUris, _ := cpDecoded.Uris()
			_, _ = decodedUris.WorkingDirUri()
			_, _ = decodedUris.PyModulesUris()
			decodedConfig, _ := cpDecoded.RuntimeEnvConfig()
			_ = decodedConfig.SetupTimeoutSeconds()
			_ = decodedConfig.EagerInstall()
			_, _ = decodedConfig.LogFiles()
		}
	})
	fmt.Printf("✓ Cap'n Proto unmarshal CPU profile saved to: %s\n", filepath.Join(resultsDir, "capnproto_unmarshal.prof"))

	// Profile FlatBuffers unmarshal
	profileMarshalFunction(filepath.Join(resultsDir, "flatbuffers_unmarshal.prof"), func() {
		for i := 0; i < iterations; i++ {
			fbDecoded := fb.GetRootAsRuntimeEnvInfo(fbBytes, 0)
			_ = string(fbDecoded.SerializedRuntimeEnv())
			decodedUris := fbDecoded.Uris(nil)
			_ = string(decodedUris.WorkingDirUri())
			_ = decodedUris.PyModulesUrisLength()
			decodedConfig := fbDecoded.RuntimeEnvConfig(nil)
			_ = decodedConfig.SetupTimeoutSeconds()
			_ = decodedConfig.EagerInstall()
			_ = decodedConfig.LogFilesLength()
		}
	})
	fmt.Printf("✓ FlatBuffers unmarshal CPU profile saved to: %s\n", filepath.Join(resultsDir, "flatbuffers_unmarshal.prof"))

	fmt.Println("\nTo analyze unmarshal CPU profiles, use:")
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "protobuf_unmarshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "symphony_unmarshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "capnproto_unmarshal.prof"))
	fmt.Printf("  go tool pprof %s\n", filepath.Join(resultsDir, "flatbuffers_unmarshal.prof"))
	fmt.Println("\nIn pprof, use commands like: top, list, web, png")
}
