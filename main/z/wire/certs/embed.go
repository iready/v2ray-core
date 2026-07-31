// Package certembed 内嵌 mTLS 材料（由 build.sh 在编译前注入）。
package certembed

import _ "embed"

//go:embed root_ca.pem
var RootCAPEM []byte

//go:embed client.p12
var ClientP12 []byte

//go:embed client.p12.pass
var ClientP12Pass []byte
