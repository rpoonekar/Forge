package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// Result holds the output of a task execution.
type Result struct {
	Output   string
	ExitCode int
}

// DockerExecutor runs commands inside ephemeral Docker containers.
// Each task gets a completely isolated, reproducible environment
// that is automatically destroyed upon completion.
type DockerExecutor struct {
	cli   *client.Client
	image string // e.g., "alpine:latest"
}

// New creates a DockerExecutor connected to the local Docker daemon.
// Checks the standard environment/socket locations, with fallback
// detection for the macOS Docker Desktop user socket (~/.docker/run/docker.sock).
func New(imageName string) (*DockerExecutor, error) {
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}

	// Fallback detection for macOS Docker Desktop user socket if DOCKER_HOST is unset
	if os.Getenv("DOCKER_HOST") == "" {
		if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
			if home, err := os.UserHomeDir(); err == nil {
				userSock := filepath.Join(home, ".docker", "run", "docker.sock")
				if _, err := os.Stat(userSock); err == nil {
					opts = append(opts, client.WithHost("unix://"+userSock))
				}
			}
		}
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerExecutor{
		cli:   cli,
		image: imageName,
	}, nil
}

// Close closes the Docker client connection.
func (e *DockerExecutor) Close() error {
	return e.cli.Close()
}

// EnsureImage pulls the base image if it is not already cached locally.
func (e *DockerExecutor) EnsureImage(ctx context.Context) error {
	log.Printf("Pulling image %s...", e.image)
	reader, err := e.cli.ImagePull(ctx, e.image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", e.image, err)
	}
	io.Copy(io.Discard, reader)
	reader.Close()
	log.Printf("Image %s ready", e.image)
	return nil
}

// Run executes a command inside an ephemeral Docker container and returns
// the combined stdout/stderr output and exit code. The container is automatically
// destroyed when execution finishes.
func (e *DockerExecutor) Run(ctx context.Context, command string) (*Result, error) {
	resp, err := e.cli.ContainerCreate(ctx, &container.Config{
		Image: e.image,
		Cmd:   []string{"sh", "-c", command},
	}, nil, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Ensure container is always cleaned up even if start or wait fails
	defer e.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{})

	err = e.cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	statusCh, errCh := e.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var exitCode int
	select {
	case err := <-errCh:
		return nil, fmt.Errorf("failed while waiting for container %s: %w", resp.ID, err)
	case status := <-statusCh:
		exitCode = int(status.StatusCode)
		if status.Error != nil {
			return nil, fmt.Errorf("container exited with error: %s", status.Error.Message)
		}
	}

	logReader, err := e.cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}
	defer logReader.Close()

	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, logReader); err != nil {
		return nil, fmt.Errorf("failed to read container logs: %w", err)
	}

	return &Result{Output: buf.String(), ExitCode: exitCode}, nil
}
