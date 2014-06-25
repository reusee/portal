package main

import (
	"log"
	"net"
	"time"

	"github.com/reusee/van"
	//"../van"
)

func startServer(addr string, debug string) {
	// start server
	server, err := van.NewServer(addr)
	if err != nil {
		log.Fatalf("van.NewServer %v", err)
	}
	defer server.Close()

	// debug
	if debug != "" {
		server.StartDebug(debug)
	}

	// handle session
	for {
		session := <-server.NewSession
		// read packets
		go func() { //TODO exit
			packetFromLocal := make(map[uint32]chan *van.Packet)
			for {
				packet := <-session.Recv

				// check conn existence
				if _, ok := packetFromLocal[packet.Conn.Id]; !ok {
					hostPort := string(obfuscate(packet.Data))
					p("CONNECT %s\n", hostPort)
					// set receive chan
					packetFromLocal[packet.Conn.Id] = make(chan *van.Packet, 512)

					// connect to target host and read
					go func() {
						conn, err := net.DialTimeout("tcp", hostPort, time.Second*16)
						if err != nil { // target error
							p("CONNECT %s ERROR %v\n", hostPort, err)
							session.Finish(packet.Conn)
							return
						}

						// send local data to target
						go func() {
							for {
								packet := <-packetFromLocal[packet.Conn.Id]
								switch packet.Type {
								case van.DATA:
									p("FROM LOCAL %d TO %s\n", len(packet.Data), hostPort)
									conn.Write(obfuscate(packet.Data))
								case van.FIN:
									conn.Close()
									return
								}
							}
						}()

						// send target data to local
						for {
							data := make([]byte, 2048)
							n, err := conn.Read(data)
							if err != nil { // target error
								p("TARGET %s READ ERROR %v\n", hostPort, err)
								// send close packet
								session.Finish(packet.Conn)
								return
							}
							data = data[:n]
							p("FROM %s DATA %d\n", hostPort, n)
							session.Send(packet.Conn, obfuscate(data))
						}
					}()

				} else { // data from local
					packetFromLocal[packet.Conn.Id] <- packet
				}
			}
		}()
	}
}