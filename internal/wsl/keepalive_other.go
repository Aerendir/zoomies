//go:build !unix

package wsl

import (
	"context"
	"errors"
)

func startDetached(context.Context, []string) (int, error) {
	return 0, errors.New("the WSL2 keepalive only runs on Linux")
}

func processAlive(int) bool { return false }
