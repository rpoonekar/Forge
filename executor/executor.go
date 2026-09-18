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
//
// Instead of exec.Command("sh", "-c", command) running directly on your Mac,
// this creates a fresh container, runs the command inside it, captures the
// output, and destroys the container. Each task gets a completely isolated
// environment.
type DockerExecutor struct {
	cli   *client.Client
	image string // e.g., "alpine:latest"
}

// New creates a DockerExecutor connected to the local Docker daemon.
//
// client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
// creates a Docker client that:
//   - client.FromEnv: reads DOCKER_HOST from the environment (or uses the default socket)
//   - client.WithAPIVersionNegotiation(): automatically matches the server's API version
//
// The image parameter is which Docker image to use (e.g., "alpine:latest").
func New(imageName string) (*DockerExecutor, error) {
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}

	// If DOCKER_HOST is not set and /var/run/docker.sock does not exist,
	// check the default macOS Docker Desktop user socket (~/.docker/run/docker.sock).
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

// EnsureImage pulls the image if it's not already available locally.
// Call this once at startup.
func (e *DockerExecutor) EnsureImage(ctx context.Context) error {
	log.Printf("Pulling image %s...", e.image)
	reader, err := e.cli.ImagePull(ctx, e.image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", e.image, err)
	}
	// Must read the response body to completion, otherwise the pull won't finish
	io.Copy(io.Discard, reader)
	reader.Close()
	log.Printf("Image %s ready", e.image)
	return nil
}

// Run executes a command inside an ephemeral Docker container and returns
// the combined stdout+stderr output and the exit code.
//
// This is the core method you need to implement. It replaces:
//
//	cmd := exec.Command("sh", "-c", command)
//	output, err := cmd.CombinedOutput()
//
// The container lifecycle is 5 steps:
//
//  1. CREATE the container
//     resp, err := e.cli.ContainerCreate(ctx,
//     &container.Config{
//     Image: e.image,
//     Cmd:   []string{"sh", "-c", command},
//     },
//     nil,  // host config (not needed)
//     nil,  // network config (not needed)
//     nil,  // platform (not needed)
//     "",   // container name (empty = auto-generated)
//     )
//     The response contains resp.ID — the container's unique ID.
//
//  2. START the container
//     err := e.cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
//     This begins execution. The container runs in the background.
//
//  3. WAIT for the container to finish
//     statusCh, errCh := e.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
//     select {
//     case err := <-errCh:
//     // error waiting
//     case status := <-statusCh:
//     exitCode = int(status.StatusCode)
//     }
//     ContainerWait returns two channels. The select waits for either
//     an error or the container to finish. status.StatusCode is the exit code.
//
//  4. GET LOGS (stdout + stderr)
//     logReader, err := e.cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{
//     ShowStdout: true,
//     ShowStderr: true,
//     })
//     Docker log streams have an 8-byte header per line (for multiplexing
//     stdout/stderr). Use StdCopy to strip it:
//
//     import "github.com/docker/docker/pkg/stdcopy"
//     var buf bytes.Buffer
//     stdcopy.StdCopy(&buf, &buf, logReader)
//     output = buf.String()
//
//  5. REMOVE the container (cleanup)
//     e.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{})
//     Always remove — even if earlier steps failed. Use defer.
//
// TODO (Step 1): Implement this method using the 5 steps above.
//
// Hints:
//   - After step 1 (create), immediately defer step 5 (remove) so cleanup
//     always happens, even if start or wait fails.
//   - If create fails, return early — nothing to remove.
//   - The method should return a *Result with Output and ExitCode.
func (e *DockerExecutor) Run(ctx context.Context, command string) (*Result, error) {
	// TODO: Implement the container lifecycle (create → start → wait → logs → remove)
	resp, err := e.cli.ContainerCreate(ctx, &container.Config{
		Image: e.image,
		Cmd:   []string{"sh", "-c", command},
	}, nil, nil, nil, "")

	if err != nil {
		return nil, fmt.Errorf("Failed to create container: %w", err)
	}

	defer e.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{})

	err = e.cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
	if err != nil {
		return nil, fmt.Errorf("Failed to start container: %w", err)
	}

	statusCh, errCh := e.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var exitCode int
	select {
	case err := <-errCh:
		// error waiting
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
	output := buf.String()

	return &Result{Output: output, ExitCode: exitCode}, nil
}
