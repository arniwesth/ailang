package eval_harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	fsharpCompileErrRe = regexp.MustCompile(`error FS\d{4}:`)
	fsharpRuntimeErrRe = regexp.MustCompile(`System\.\w+Exception|Unhandled exception|StackOverflowException`)
)

func classifyFSharpExecution(stderr string, exitCode int) (compileOk, runtimeOk bool) {
	compileOk = !fsharpCompileErrRe.MatchString(stderr)
	runtimeErr := fsharpRuntimeErrRe.MatchString(stderr)
	runtimeOk = compileOk && (exitCode == 0 || !runtimeErr)
	return compileOk, runtimeOk
}

// FSharpRunner executes F# scripts via dotnet fsi.
type FSharpRunner struct {
	spec *BenchmarkSpec
}

// Language returns "fsharp".
func (r *FSharpRunner) Language() string {
	return "fsharp"
}

// Run executes F# code and classifies compile/runtime status from FSI output.
func (r *FSharpRunner) Run(code string, timeout time.Duration) (*RunResult, error) {
	tmpDir, err := os.MkdirTemp("", "eval_fsharp_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if r.spec != nil {
		for name, content := range r.spec.InputFiles {
			fpath := filepath.Join(tmpDir, name)
			if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
				return nil, fmt.Errorf("failed to create dir for input file %s: %w", name, err)
			}
			if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("failed to write input file %s: %w", name, err)
			}
		}
	}

	scriptPath := filepath.Join(tmpDir, "solution.fsx")
	if err := os.WriteFile(scriptPath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write code: %w", err)
	}

	cmdArgs := []string{"fsi", scriptPath}
	if r.spec != nil {
		cmdArgs = append(cmdArgs, r.spec.CliArgs...)
	}

	start := time.Now()
	cmd := exec.Command("dotnet", cmdArgs...)
	cmd.Dir = tmpDir

	if r.spec != nil && r.spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(r.spec.Stdin)
	}

	SetProcessGroup(cmd)

	stdout := NewLimitedWriter(MaxOutputSize)
	stderr := NewLimitedWriter(MaxOutputSize)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return &RunResult{
			Stderr:    err.Error(),
			ExitCode:  -1,
			Duration:  time.Since(start),
			CompileOk: false,
			RuntimeOk: false,
		}, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(timeout):
		_ = KillProcessGroup(cmd.Process.Pid)
		<-done
		return &RunResult{
			Stdout:    stdout.String(),
			Stderr:    "execution timed out",
			ExitCode:  -1,
			Duration:  timeout,
			CompileOk: true,
			RuntimeOk: false,
			TimedOut:  true,
		}, nil
	case err := <-done:
		duration := time.Since(start)
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}

		stderrStr := stderr.String()
		compileOk, runtimeOk := classifyFSharpExecution(stderrStr, exitCode)

		return &RunResult{
			Stdout:    stdout.String(),
			Stderr:    stderrStr,
			ExitCode:  exitCode,
			Duration:  duration,
			CompileOk: compileOk,
			RuntimeOk: runtimeOk,
		}, nil
	}
}
