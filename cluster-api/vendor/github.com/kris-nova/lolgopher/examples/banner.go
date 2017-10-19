package main

import (
	"github.com/CrowdSurge/banner"
	lol "github.com/kris-nova/lolgopher"
)

func main() {
	w := lol.NewLolWriter()
	w.Write([]byte(banner.PrintS("lolgopher")))
}
