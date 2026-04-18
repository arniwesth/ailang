package eval_harness

import "testing"

func TestClassifyFSharpExecution(t *testing.T) {
	tests := []struct {
		name      string
		stderr    string
		exitCode  int
		compileOk bool
		runtimeOk bool
	}{
		{
			name:      "success",
			stderr:    "",
			exitCode:  0,
			compileOk: true,
			runtimeOk: true,
		},
		{
			name:      "warning only",
			stderr:    "warning FS1182: The value 'x' is unused",
			exitCode:  0,
			compileOk: true,
			runtimeOk: true,
		},
		{
			name:      "compile error",
			stderr:    "error FS0039: The value or constructor 'foo' is not defined",
			exitCode:  1,
			compileOk: false,
			runtimeOk: false,
		},
		{
			name:      "runtime exception",
			stderr:    "Unhandled exception. System.Exception: boom",
			exitCode:  1,
			compileOk: true,
			runtimeOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compileOk, runtimeOk := classifyFSharpExecution(tt.stderr, tt.exitCode)
			if compileOk != tt.compileOk {
				t.Fatalf("compileOk = %v, want %v", compileOk, tt.compileOk)
			}
			if runtimeOk != tt.runtimeOk {
				t.Fatalf("runtimeOk = %v, want %v", runtimeOk, tt.runtimeOk)
			}
		})
	}
}
