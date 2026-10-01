package localadmin

import "embed"

// WebDistRoot go:embed 子目录前缀。
const WebDistRoot = "web/dist"

//go:embed web/dist
var buildFS embed.FS

//go:embed web/dist/index.html
var indexPage []byte
