package main

import (
	"log"
	"net"

	socks "github.com/reusee/socks5-server"
	//"github.com/reusee/van"
	"../van"
)

func startLocal(remoteAddr, socksAddr string) {
	// connect to remote
	client, err := van.NewClient(remoteAddr)
	if err != nil {
		log.Fatalf("van.NewClient %v", err)
	}
	defer client.Close()
	p("Connected.\n")

	// set conns
	for i := 0; i < 8; i++ {
		client.NewTransport()
	}
	client.OnSignal("RemoveTransport", func() {
		client.NewTransport()
	})

	// start socks server
	socksServer, err := socks.New(socksAddr)
	if err != nil {
		log.Fatalf("socks5.NewServer %v", err)
	}
	defer socksServer.Close()
	p("Socks5 server started.\n")

	// globals
	socksClientConns := make(map[uint32]net.Conn)
	hostPorts := make(map[uint32]string)

	// handle socks client
	socksServer.OnSignal("client", func(args ...interface{}) {
		go func() {
			conn := client.NewConn()
			socksClientConn := args[0].(net.Conn)
			socksClientConns[conn.Id] = socksClientConn
			hostPort := args[1].(string)
			hostPorts[conn.Id] = string(hostPort)
			// send hostport
			client.Send(conn, obfuscate([]byte(hostPort)))
			// read from socks conn and send to remote
			for {
				data := make([]byte, 2048)
				n, err := socksClientConn.Read(data)
				if err != nil { // socks conn closed
					// send a zero-lengthed packet
					client.Finish(conn)
					return
				}
				data = data[:n]
				client.Send(conn, obfuscate(data))
			}
		}()
	})

	// handle packet from remote
	for {
		packet := <-client.Recv
		switch packet.Type {
		case van.DATA:
			data := obfuscate(packet.Data)
			socksClientConns[packet.Conn.Id].Write(data)
		case van.FIN:
			socksClientConns[packet.Conn.Id].Close()
		}
	}
}
