package service

import (
	"errors"
	"testing"
)

func TestClassifyFailureReason(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		logText string
		code    string
		detail  string
	}{
		{
			name:    "builder image missing",
			logText: `Error: 127.0.0.1:5000/refinery-builder:latest: image not known`,
			code:    "builder_image_missing",
			detail:  "refinery-builder",
		},
		{
			name:    "runtime stdlib missing",
			logText: "Fatal Python error: Py_Initialize: Unable to get the locale encoding\nModuleNotFoundError: No module named 'encodings'",
			code:    "runtime_stdlib_missing",
			detail:  "encodings",
		},
		{
			name:    "compiler version too old",
			logText: "../meson.build:25:4: ERROR: Problem encountered: NumPy requires GCC >= 9.3",
			code:    "compiler_version_too_old",
			detail:  "gcc>=9.3",
		},
		{
			name:    "missing module",
			logText: "ModuleNotFoundError: No module named 'numpy'",
			code:    "missing_module",
			detail:  "numpy",
		},
		{
			name:    "missing header",
			logText: "fatal error: Python.h: No such file or directory",
			code:    "missing_header",
			detail:  "Python.h",
		},
		{
			name:    "missing library",
			logText: "ld: cannot find -lssl",
			code:    "missing_library",
			detail:  "ssl",
		},
		{
			name:    "pkg config missing",
			logText: "No package 'libffi' found",
			code:    "pkg_config_missing",
			detail:  "libffi",
		},
		{
			name:    "cmake missing",
			logText: "Could not find Foo (missing: Foo_DIR)",
			code:    "cmake_missing",
			detail:  "Foo",
		},
		{
			name:    "cmake failure",
			logText: "CMake Error at CMakeLists.txt:1 (project):",
			code:    "cmake_failure",
		},
		{
			name:    "linker error",
			logText: "undefined reference to `PyExc_SystemError'",
			code:    "linker_error",
		},
		{
			name:    "rust toolchain missing",
			logText: "rustc: command not found",
			code:    "rust_toolchain_missing",
			detail:  "rustc",
		},
		{
			name:    "compiler missing",
			logText: "gcc: command not found",
			code:    "compiler_missing",
			detail:  "gcc",
		},
		{
			name:    "build tool missing",
			logText: "make: command not found",
			code:    "build_tool_missing",
			detail:  "make",
		},
		{
			name:    "error fallback",
			err:     errors.New("linker command failed"),
			logText: "",
			code:    "linker_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyFailureReason(tt.err, tt.logText)
			if got.Code != tt.code {
				t.Fatalf("code=%q want %q", got.Code, tt.code)
			}
			if tt.detail != "" && got.Detail != tt.detail {
				t.Fatalf("detail=%q want %q", got.Detail, tt.detail)
			}
		})
	}
}
