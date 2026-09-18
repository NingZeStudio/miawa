//go:build !windows

package stats

import (
	"syscall"
)

func GetDiskUsage(path string) (*DiskInfo, error) {
	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return nil, err
	}

	total := int64(fs.Blocks) * int64(fs.Bsize)
	free := int64(fs.Bfree) * int64(fs.Bsize)

	return &DiskInfo{
		Total: total,
		Free:  free,
		Used:  total - free,
	}, nil
}
