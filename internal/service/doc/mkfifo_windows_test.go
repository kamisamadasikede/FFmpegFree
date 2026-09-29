//go:build windows

package doc

import "errors"

func mkfifo(string) error { return errors.New("windows 没有 FIFO") }
