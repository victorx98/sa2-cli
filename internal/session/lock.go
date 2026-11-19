package session

import (
	"context"
	"encoding/hex"
	"time"
)

type Locker struct {
	dir string
}

func NewLocker(dir string) *Locker {
	return &Locker{dir: dir}
}

func (l *Locker) Acquire(ctx context.Context, key string) (func(), error) {
	if len(key) != 64 {
		return nil, ErrUnsafeLock
	}
	if _, err := hex.DecodeString(key); err != nil {
		return nil, ErrUnsafeLock
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := openLock(l.dir, key+".lock")
	if err != nil {
		return nil, ErrUnsafeLock
	}
	closeFile := func() { _ = file.Close() }

	wait := time.NewTimer(15 * time.Second)
	defer wait.Stop()
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	for {
		acquired, err := tryLock(file)
		if err != nil {
			closeFile()
			return nil, ErrUnsafeLock
		}
		if acquired {
			return closeFile, nil
		}
		select {
		case <-ctx.Done():
			closeFile()
			return nil, ctx.Err()
		case <-wait.C:
			closeFile()
			return nil, ErrBusy
		case <-poll.C:
		}
	}
}
