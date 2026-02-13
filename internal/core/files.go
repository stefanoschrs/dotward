package core

import (
	"fmt"
	"os"
)

// SecureDelete attempts to overwrite a file before deleting it.
func SecureDelete(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		info, statErr := f.Stat()
		if statErr == nil {
			zeros := make([]byte, 4096)
			remaining := info.Size()
			for remaining > 0 {
				chunk := int64(len(zeros))
				if remaining < chunk {
					chunk = remaining
				}
				if _, writeErr := f.Write(zeros[:chunk]); writeErr != nil {
					break
				}
				remaining -= chunk
			}
			_ = f.Sync()
		}
		_ = f.Close()
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to remove file %q: %w", path, err)
	}
	return nil
}
