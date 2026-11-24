package session

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrPlanFile = errors.New("invalid or inaccessible private plan file")

var ErrPlanExists = errors.New("plan output already exists")

const maxPlanSize = 128 << 10

func CheckNewPlanPath(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return ErrPlanExists
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ErrPlanFile
	}
	return nil
}

// Publish the complete private file without replacing a plan from another preview.
func WritePlan(path string, data []byte) error {
	if len(data) > maxPlanSize {
		return ErrPlanFile
	}
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
	if err = os.Link(tmp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrPlanExists
		}
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
	b, err := io.ReadAll(io.LimitReader(f, maxPlanSize+1))
	if err != nil || len(b) > maxPlanSize {
		return nil, ErrPlanFile
	}
	return b, nil
}
