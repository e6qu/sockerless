package sandbox

import (
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
)

// builtinBase64 encodes or decodes base64 data from stdin.
func builtinBase64(args []string, hc interp.HandlerContext) error {
	decode := false
	for _, a := range args[1:] {
		if a == "-d" || a == "--decode" {
			decode = true
		}
	}
	input, err := io.ReadAll(hc.Stdin)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "base64: %v\n", err)
		return interp.ExitStatus(1)
	}
	if decode {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(input)))
		if err != nil {
			fmt.Fprintf(hc.Stderr, "base64: invalid input\n")
			return interp.ExitStatus(1)
		}
		_, _ = hc.Stdout.Write(decoded)
	} else {
		encoded := base64.StdEncoding.EncodeToString(input)
		fmt.Fprintln(hc.Stdout, encoded)
	}
	return nil
}

// builtinBasename prints the filename portion of a path.
func builtinBasename(args []string, hc interp.HandlerContext) error {
	if len(args) < 2 {
		fmt.Fprintf(hc.Stderr, "basename: missing operand\n")
		return interp.ExitStatus(1)
	}
	result := filepath.Base(args[1])
	if len(args) > 2 {
		suffix := args[2]
		result = strings.TrimSuffix(result, suffix)
	}
	fmt.Fprintln(hc.Stdout, result)
	return nil
}

// builtinDirname prints the directory portion of a path.
func builtinDirname(args []string, hc interp.HandlerContext) error {
	if len(args) < 2 {
		fmt.Fprintf(hc.Stderr, "dirname: missing operand\n")
		return interp.ExitStatus(1)
	}
	for _, arg := range args[1:] {
		// GNU dirname strips trailing slashes before calling Dir
		clean := strings.TrimRight(arg, "/")
		if clean == "" {
			clean = "/"
		}
		fmt.Fprintln(hc.Stdout, filepath.Dir(clean))
	}
	return nil
}

// builtinSeq prints a sequence of numbers.
func builtinSeq(args []string, hc interp.HandlerContext) error {
	nums := make([]float64, 0, 3)
	for _, a := range args[1:] {
		if a == "" || a[0] == '-' && len(a) > 1 && (a[1] < '0' || a[1] > '9') {
			continue
		}
		n, err := strconv.ParseFloat(a, 64)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "seq: invalid argument: %s\n", a)
			return interp.ExitStatus(1)
		}
		nums = append(nums, n)
	}
	var first, incr, last float64
	switch len(nums) {
	case 1:
		first, incr, last = 1, 1, nums[0]
	case 2:
		first, incr, last = nums[0], 1, nums[1]
	case 3:
		first, incr, last = nums[0], nums[1], nums[2]
	default:
		fmt.Fprintf(hc.Stderr, "seq: missing operand\n")
		return interp.ExitStatus(1)
	}
	if incr == 0 {
		fmt.Fprintf(hc.Stderr, "seq: zero increment\n")
		return interp.ExitStatus(1)
	}
	for i := first; (incr > 0 && i <= last) || (incr < 0 && i >= last); i += incr {
		if i == float64(int64(i)) {
			fmt.Fprintf(hc.Stdout, "%d\n", int64(i))
		} else {
			fmt.Fprintf(hc.Stdout, "%g\n", i)
		}
	}
	return nil
}

// builtinEnv prints exported environment variables.
func builtinEnv(args []string, hc interp.HandlerContext) error {
	hc.Env.Each(func(name string, vr expand.Variable) bool {
		if vr.Exported && vr.Kind == expand.String {
			fmt.Fprintf(hc.Stdout, "%s=%s\n", name, vr.Str)
		}
		return true
	})
	return nil
}
