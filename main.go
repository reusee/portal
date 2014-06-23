package main

import (
	"fmt"

	"os"
	"strings"

	_ "net/http/pprof"
)

var p = fmt.Printf

func main() {
	if len(os.Args) == 1 {
		goto usage
	}

	for _, arg := range os.Args[1:] {
		parts := strings.Split(arg, "-")
		switch parts[0] {
		case "server":
			startServer(parts[1])
		case "local":
			startLocal(parts[1], parts[2])
		default:
			goto usage
		}
	}

	return

usage:
	p("usage: %s [server-addr:port] / [local-remote:port-local:port]", os.Args[0])
	return
}

func obfuscate(data []byte) []byte {
	for i, _ := range data {
		data[i] ^= 0xDE
	}
	return data
}
