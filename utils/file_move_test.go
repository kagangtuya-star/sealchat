package utils

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMoveFileSameDevice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := MoveFile(src, dst); err != nil {
		t.Fatalf("move file: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src should be removed, got: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected dst content: %s", string(data))
	}
}

func TestMoveFileCrossDeviceFallback(t *testing.T) {
	oldRename := moveFileRename
	moveFileRename = func(oldPath, newPath string) error {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: syscall.EXDEV}
	}
	defer func() {
		moveFileRename = oldRename
	}()

	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")

	if err := os.WriteFile(src, []byte("world"), 0o640); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := MoveFile(src, dst); err != nil {
		t.Fatalf("move file with EXDEV fallback: %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src should be removed, got: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "world" {
		t.Fatalf("unexpected dst content: %s", string(data))
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("unexpected dst perm: %#o", info.Mode().Perm())
	}
}
