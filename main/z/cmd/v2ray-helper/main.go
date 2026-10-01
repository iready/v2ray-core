//go:build darwin

package main

import (
	"log"

	"github.com/v2fly/v2ray-core/v5/main/z/helper"
)

func main() {
	log.Printf("v2ray helper starting on %s", helper.SocketPath)
	if err := helper.NewServer().ListenAndServe(helper.SocketPath); err != nil {
		log.Fatal(err)
	}
}
