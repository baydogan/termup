package main

import (
	"flag"
	"log"
	"os"
)

// defaultAddr is used when neither --addr nor ZT_ADDR is set.
// Local default is the unix socket (file permissions act as auth);
// remote uses an http:// TCP address.
const defaultAddr = "unix:///tmp/ztd.sock"

// resolveAddr picks the server address using the precedence:
// --addr flag > ZT_ADDR env > default local socket.
func resolveAddr(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("ZT_ADDR"); v != "" {
		return v
	}
	return defaultAddr
}

func main() {
	// TODO: replace stdlib flag wiring with Cobra when zt subcommands land.
	addrFlag := flag.String("addr", "", "ztd server address (unix:///path or http://host:port)")
	flag.Parse()

	addr := resolveAddr(*addrFlag)

	// TODO: build api/client from addr and run subcommands (status/list/watch).
	log.Printf("resolved server address: %s", addr)
}
