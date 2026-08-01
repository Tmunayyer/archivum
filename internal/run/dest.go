package run

import (
	"io"
	"os"
	"path/filepath"
)

// DirDest is the production Dest: one flat destination directory. Copies
// are creates-only (O_EXCL) so even a race with another writer can error
// but never overwrite, and no code path removes or renames a source
// (ADR-0001).
type DirDest struct {
	Dir string
}

func (d DirDest) Exists(name string) bool {
	_, err := os.Stat(filepath.Join(d.Dir, name))
	return err == nil
}

func (d DirDest) Copy(srcPath, name string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(filepath.Join(d.Dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(out.Name())
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(out.Name())
		return err
	}
	return nil
}
