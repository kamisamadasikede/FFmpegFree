package live

import "path/filepath"

var Global = NewManager(filepath.Join("public", "archive"))
