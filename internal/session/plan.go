package session

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrPlanFile = errors.New("invalid or inaccessible private plan file")

// WritePlan creates the temporary file in the destination directory so rename cannot cross volumes.
func WritePlan(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, ".sa2-plan-"+rand.Text())
	f, err := privatePlanFile(tmp, true)
	if err != nil {
		return ErrPlanFile
	}
	defer os.Remove(tmp)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ErrPlanFile
	}
	if err = os.Rename(tmp, path); err != nil {
		return ErrPlanFile
	}
	return nil
}
func ReadPlan(path string) ([]byte, error) {
	f, err := privatePlanFile(path, false)
	if err != nil {
		return nil, ErrPlanFile
	}
	defer f.Close()
	const limit = 128 << 10
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(b) > limit {
		return nil, ErrPlanFile
	}
	return b, nil
}
