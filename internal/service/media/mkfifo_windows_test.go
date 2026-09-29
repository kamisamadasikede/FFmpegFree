//go:build windows

package media

import "errors"

func syscallMkfifo(string) error { return errors.New("windows 没有 mkfifo") }
