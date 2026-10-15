//go:build !windows

package catagent

import "os"

func createSharedDirectoryLink(link, target string) error { return os.Symlink(target, link) }
