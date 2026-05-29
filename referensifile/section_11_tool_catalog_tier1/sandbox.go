package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/provider"
)

// SandboxTool menyediakan kapabilitas eksekusi shell command di dalam container Docker yang ephemeral.
type SandboxTool struct {
	root            string
	defaultTimeout  time.Duration
	maxOutputLength int
}

type sandboxArgs struct {
	Command        string `json:"command" validate:"required"`
	Image          string `json:"image,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

func NewSandboxTool(root string) *SandboxTool {
	return &SandboxTool{
		root:            root,
		defaultTimeout:  60 * time.Second,
		maxOutputLength: 64 * 1024,
	}
}

// Definition mengembalikan definisi sandbox tool.
func (t *SandboxTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "run_sandbox_command",
		Description: `Execute a shell command inside an ephemeral Docker container.
Sangat berguna untuk menjalankan skrip Python murni, menginstal dependencies analisis tanpa mengotori host, mengeksekusi tes yang berbahaya, atau memanipulasi data berat menggunakan pustaka eksternal.

Aturan pemaikaian:
  - image: base image Docker (contoh: "python:3.10-slim", "node:18-alpine", "ubuntu:22.04"). Jika tidak disuplai, default adalah "ubuntu:latest".
  - command: Perintah shell yang akan dijalankan di dalam container.
  - Workspace direktori otomatis di-mount ke /workspace dan menjadi working directory container.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute inside the container.",
				},
				"image": map[string]any{
					"type":        "string",
					"description": "Docker image to use. Default is ubuntu:latest.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds. Max 300s.",
				},
			},
			"required": []string{"command"},
		},
	}
}

// Execute menjalankan perintah di dalam instansi Docker terisolasi.
func (t *SandboxTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args sandboxArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode sandbox arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	if strings.TrimSpace(args.Command) == "" {
		return Result{}, fmt.Errorf("sandbox command is required")
	}

	image := "ubuntu:latest"
	if strings.TrimSpace(args.Image) != "" {
		image = strings.TrimSpace(args.Image)
	}

	timeout := t.defaultTimeout
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
		if timeout > 5*time.Minute {
			timeout = 5 * time.Minute
		}
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Setup docker run command
	dockerArgs := []string{
		"run", "--rm",
		"-v", t.root + ":/workspace",
		"-w", "/workspace",
		image,
		"sh", "-c", args.Command,
	}

	cmd := exec.CommandContext(cctx, "docker", dockerArgs...)
	outBytes, err := cmd.CombinedOutput()
	outputText := string(outBytes)

	// Bersihkan output
	outputText = filterBashOutput(outputText)

	metadata := map[string]any{
		"command":         args.Command,
		"image":           image,
		"timeout_seconds": int(timeout.Seconds()),
		"isolator":        "docker",
	}

	if outputText == "" {
		outputText = "(no output)"
	}

	if cctx.Err() == context.DeadlineExceeded {
		return Result{
			Output:   outputText,
			Metadata: metadata,
		}, fmt.Errorf("command timed out after %s", timeout)
	}

	if err != nil {
		metadata["exit_code"] = cmd.ProcessState.ExitCode()
		return Result{
			Output:   outputText,
			Metadata: metadata,
		}, fmt.Errorf("container escape/execution failed: %w", err)
	}

	return Result{
		Output:   outputText,
		Metadata: metadata,
	}, nil
}
