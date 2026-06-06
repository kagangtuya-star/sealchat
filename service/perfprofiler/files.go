package perfprofiler

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writeJSONFile(path string, value any) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func appendJSONL(path string, value any) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	return enc.Encode(value)
}

func readJSONL(path string, out *[]SamplePoint) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var point SamplePoint
		if err := json.Unmarshal(scanner.Bytes(), &point); err != nil {
			return err
		}
		*out = append(*out, point)
	}
	return scanner.Err()
}

func listArtifacts(root string) ([]Artifact, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return []Artifact{}, nil
		}
		return nil, err
	}
	var items []Artifact
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".tmp") || d.Name() == "current.json" || d.Name() == "current-state.json" || strings.HasPrefix(d.Name(), "samples-") {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		items = append(items, Artifact{
			Name:       d.Name(),
			Kind:       detectArtifactKind(d.Name(), path),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UnixMilli(),
			Path:       path,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ModifiedAt > items[j].ModifiedAt
	})
	return items, nil
}

func cleanupExpiredFiles(root string, retention time.Duration) error {
	if retention <= 0 {
		return nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-retention)
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() == "current.json" || d.Name() == "current-state.json" {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		if info.ModTime().Before(cutoff) {
			return os.Remove(path)
		}
		return nil
	})
}

func detectArtifactKind(name, path string) string {
	switch {
	case strings.Contains(path, string(filepath.Separator)+"sessions"+string(filepath.Separator)) && strings.HasSuffix(name, ".pb.gz"):
		return "session-cpu"
	case strings.Contains(path, string(filepath.Separator)+"sessions"+string(filepath.Separator)) && strings.HasSuffix(name, ".json"):
		return "session-meta"
	case strings.Contains(path, string(filepath.Separator)+"snapshots"+string(filepath.Separator)) && strings.HasPrefix(name, "cpu-"):
		return "snapshot-cpu"
	case strings.Contains(path, string(filepath.Separator)+"snapshots"+string(filepath.Separator)) && strings.HasPrefix(name, "heap-"):
		return "snapshot-heap"
	case strings.Contains(path, string(filepath.Separator)+"snapshots"+string(filepath.Separator)) && strings.HasPrefix(name, "allocs-"):
		return "snapshot-allocs"
	case strings.Contains(path, string(filepath.Separator)+"snapshots"+string(filepath.Separator)) && strings.HasPrefix(name, "goroutine-"):
		return "snapshot-goroutine"
	case strings.Contains(path, string(filepath.Separator)+"snapshots"+string(filepath.Separator)) && strings.HasPrefix(name, "summary-"):
		return "snapshot-summary"
	default:
		return "artifact"
	}
}
