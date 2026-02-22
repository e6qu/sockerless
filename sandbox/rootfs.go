package sandbox

import (
	"os"
	"path/filepath"
)

// PopulateRootfs creates an Alpine-like directory structure in dir.
func PopulateRootfs(dir string) error {
	dirs := []string{
		"bin", "sbin",
		"usr/bin", "usr/sbin", "usr/local/bin",
		"etc",
		"tmp", "var/tmp", "var/log",
		"dev",
		"home", "root",
		"proc", "sys",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			return err
		}
	}

	// /etc/passwd
	if err := os.WriteFile(filepath.Join(dir, "etc/passwd"),
		[]byte("root:x:0:0:root:/root:/bin/sh\nnobody:x:65534:65534:nobody:/:/sbin/nologin\n"), 0644); err != nil {
		return err
	}

	// /etc/group
	if err := os.WriteFile(filepath.Join(dir, "etc/group"),
		[]byte("root:x:0:\nnobody:x:65534:\n"), 0644); err != nil {
		return err
	}

	// /etc/hostname
	if err := os.WriteFile(filepath.Join(dir, "etc/hostname"),
		[]byte("localhost\n"), 0644); err != nil {
		return err
	}

	// /etc/hosts
	if err := os.WriteFile(filepath.Join(dir, "etc/hosts"),
		[]byte("127.0.0.1\tlocalhost\n::1\t\tlocalhost\n"), 0644); err != nil {
		return err
	}

	// /etc/resolv.conf
	if err := os.WriteFile(filepath.Join(dir, "etc/resolv.conf"),
		[]byte("nameserver 127.0.0.11\n"), 0644); err != nil {
		return err
	}

	// /dev/null — empty file that tools like `tail -f /dev/null` can open
	if err := os.WriteFile(filepath.Join(dir, "dev/null"), nil, 0666); err != nil {
		return err
	}

	return nil
}
