package testutils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// containerTestDataDir is where the driver's testdata directory is mounted inside the
	// driver container; all olake inputs and outputs (streams.json, state.json, stats.json,
	// logs) live under it since the CLI writes next to --config.
	containerTestDataDir = "/testdata"
)

// driverImageRef returns the image the harness runs, `olake/source-<driver>:local` as
// built by `make docker.<driver>.build`; OLAKE_DRIVER_IMAGE overrides it.
func driverImageRef(driver string) string {
	if ref := os.Getenv("OLAKE_DRIVER_IMAGE"); ref != "" {
		return ref
	}
	return fmt.Sprintf("olake/source-%s:local", driver)
}

var (
	ensureImageOnce sync.Once
	ensureImageErr  error
)

// ensureDriverImage makes sure the driver image exists locally, building it via
// `make docker.<driver>.build` when missing. CI pre-builds the image; this fallback keeps
// local runs one-command. Guarded by sync.Once so parallel tests trigger the (slow) build
// at most once and all share its result.
func ensureDriverImage(t *testing.T, cfg *TestConfig) string {
	t.Helper()
	ref := driverImageRef(cfg.Driver)
	ensureImageOnce.Do(func() {
		if err := exec.Command("docker", "image", "inspect", ref).Run(); err == nil {
			return
		}
		t.Logf("driver image %s not found locally, building it with `make docker.%s.build`", ref, cfg.Driver)
		// wall-clock via trackPhaseTiming, not cmd.ProcessState.SystemTime() (that reports make's
		// kernel CPU time — a misleading ~87ms even when the docker build actually took far longer).
		defer trackPhaseTiming(t, "driver-image", ref)()
		// Called plain: the image's platform comes from drivers/platforms.conf via
		// local_driver_platforms, so cross-arch pins (db2 is amd64-only) stay in make.
		cmd := exec.Command("make", fmt.Sprintf("docker.%s.build", cfg.Driver))
		cmd.Dir = cfg.HostRootPath
		if out, err := cmd.CombinedOutput(); err != nil {
			ensureImageErr = fmt.Errorf("failed to build driver image %s (the iceberg jar must be built first, see destination/iceberg/olake-iceberg-java-writer): %w\n%s", ref, err, out)
		}
	})
	require.NoError(t, ensureImageErr, "driver image unavailable")
	return ref
}

// dockerRunArgs builds the `docker run` argument list that invokes the driver image exactly
// as a user would: the image's ENTRYPOINT (./olake) runs with olakeArgs appended. The
// driver's testdata directory is mounted at /testdata so the config/catalog/state files are
// shared with the host and the CLI writes its outputs (streams.json, state.json, ...) back
// there. extraFlags carries per-invocation docker flags (host gateway, network, name).
func dockerRunArgs(cfg *TestConfig, extraFlags []string, olakeArgs []string) []string {
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:%s", cfg.HostTestDataPath, containerTestDataDir),
		"-e", "TELEMETRY_DISABLED=true",
	}
	if cfg.ImagePlatform != "" {
		args = append(args, "--platform", cfg.ImagePlatform)
	}
	args = append(args, extraFlags...)
	args = append(args, driverImageRef(cfg.Driver))
	return append(args, olakeArgs...)
}

// runOlake runs the driver image once, exactly like a real user would:
//
//	docker run --rm -v <testdata>:/testdata olake/source-<driver>:local <olakeArgs...>
//
// It exercises the image's real ENTRYPOINT (no exec-into-a-parked-container) and returns the
// container's exit code and combined stdout+stderr. err is non-nil only when docker itself
// fails to launch — a non-zero olake exit is reported via the code, mirroring a user's
// experience at the CLI.
func runOlake(ctx context.Context, t *testing.T, cfg *TestConfig, olakeArgs ...string) (int, []byte, error) {
	t.Helper()
	ensureDriverImage(t, cfg)
	defer trackPhaseTiming(t, cfg.Driver, olakeArgs[0]+" run")()

	args := dockerRunArgs(cfg, []string{"--add-host", "host.docker.internal:host-gateway"}, olakeArgs)
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return dockerExitResult(out, err, olakeArgs[0])
}

// dockerExitResult normalizes `docker run`'s outcome into (exitCode, output, err): a non-zero
// container exit is a normal result carried in exitCode; only a failure to launch docker
// itself is returned as err.
func dockerExitResult(out []byte, err error, what string) (int, []byte, error) {
	if err == nil {
		return 0, out, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), out, nil
	}
	return -1, out, fmt.Errorf("docker run (%s) failed to execute: %w", what, err)
}

// syncArgs builds the `olake sync ...` argument vector run against the driver image.
func syncArgs(config TestConfig, useState bool, destinationType string, flags ...string) []string {
	args := []string{"sync", "--config", config.SourcePath, "--catalog", config.CatalogPath}
	switch destinationType {
	case "iceberg":
		args = append(args, "--destination", config.IcebergDestinationPath)
	case "parquet":
		args = append(args, "--destination", config.ParquetDestinationPath)
	}
	if useState {
		args = append(args, "--state", config.StatePath)
	}
	return append(args, flags...)
}

// discoverArgs builds the `olake discover ...` argument vector run against the driver image.
func discoverArgs(config TestConfig, flags ...string) []string {
	return append([]string{"discover", "--config", config.SourcePath}, flags...)
}
