package helper

var (
	Label                 = "com.v2ray.helper"
	SocketPath            = "/var/run/com.v2ray.helper.sock"
	AllowedUIDPath        = "/var/run/com.v2ray.helper.uid"
	AllowedUIDPersistPath = "/var/db/com.v2ray.helper.uid"
)

// Default split routes for macOS (avoid 0.0.0.0/8 above default gateway).
var DefaultDarwinRoutes = []string{
	"1.0.0.0/8",
	"2.0.0.0/7",
	"4.0.0.0/6",
	"8.0.0.0/5",
	"16.0.0.0/4",
	"32.0.0.0/3",
	"64.0.0.0/2",
	"128.0.0.0/1",
}
