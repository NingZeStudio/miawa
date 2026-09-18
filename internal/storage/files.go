package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type FileNode struct {
	Name     string     `json:"name"`
	IsDir    bool       `json:"is_dir"`
	Size     int64      `json:"size"`
	Children []FileNode `json:"children,omitempty"`
}

// ListTree 递归列出从 base/path 开始的目录，并具有防止逃逸的安全性。
func ListTree(base string, relPath string) (FileNode, error) {
	root := filepath.Join(base, relPath)
	clean := filepath.Clean(root)
	// 确保 clean 以 base 开头
	if !isSubPath(base, clean) {
		return FileNode{}, errors.New("无效路径")
	}
	fi, err := os.Stat(clean)
	if err != nil {
		return FileNode{}, err
	}
	return buildNode(clean, fi)
}

func buildNode(path string, fi os.FileInfo) (FileNode, error) {
	n := FileNode{Name: fi.Name(), IsDir: fi.IsDir(), Size: fi.Size()}
	if !fi.IsDir() {
		return n, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return n, err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return n, err
		}
		childPath := filepath.Join(path, e.Name())
		child, err := buildNode(childPath, info)
		if err != nil {
			return n, err
		}
		n.Children = append(n.Children, child)
	}
	return n, nil
}

func isSubPath(base, target string) bool {
	base = filepath.Clean(base)
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return !startsWithDotDot(rel)
}

func startsWithDotDot(p string) bool {
	return p == ".." || (len(p) > 2 && p[:3] == ".."+string(filepath.Separator))
}

// ReadInfoJSON 读取 info.json 到通用 map 以供 UI 使用。
func ReadInfoJSON(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return v, nil
}
