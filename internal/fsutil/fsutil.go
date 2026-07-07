// Package fsutil provides the small file-write primitives shared by the
// whole app, including the atomic write used for every config/progress file.
package fsutil

import "os"

func WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func Delete(path string) error {
	return os.Remove(path)
}

func Rename(old, new string) error {
	return os.Rename(old, new)
}

// WriteFileAtomic writes via a .tmp file + rename so readers never observe a
// half-written file.
func WriteFileAtomic(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := WriteFile(tmpPath, data); err != nil {
		return err
	}
	if err := Rename(tmpPath, path); err != nil {
		Delete(tmpPath)
		return err
	}
	return nil
}
