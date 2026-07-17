package main

import "github.com/baydogan/zerotolerance/server"

const socketPath = "/tmp/ztd.sock"

func main() {
	start()
}

func start() {
	server.Start()
}
