//go:build !linux

package disk

import (
	"os"
	"time"
)

func statfs(path string) (int64, int64, error) {
	probe := path + string(os.PathSeparator) + ".hermit-statfs-" + time.Now().Format("150405")
	f, err := os.Create(probe)
	if err != nil {
		return 0, 0, err
	}
	defer func() { f.Close(); os.Remove(probe) }()
	return 0, 0, nil
}
