package main

import (
	"log"
	"net"
	"time"

	"github.com/reusee/van"
)

func startServer(addr string) {
	// start server
	server, err := van.NewServer(addr)
	if err != nil {
		log.Fatalf("van.NewServer %v", err)
	}
	defer server.Close()

	// handle session
	for {
		session := <-server.NewSession
		// read packets
		go func() { //TODO exit
			dataFromLocal := make(map[int64]chan []byte)
			for {
				packet := <-session.Recv
				packet.Data = obfuscate(packet.Data)
				if packet.Type != van.DATA {
					continue
				}

				// check conn existence
				if _, ok := dataFromLocal[packet.Conn.Id]; !ok {
					hostPort := string(packet.Data)
					p("CONNECT %s\n", hostPort)
					// set receive chan
					dataFromLocal[packet.Conn.Id] = make(chan []byte, 512)

					// connect to target host and read
					go func() {
						conn, err := net.DialTimeout("tcp", hostPort, time.Second*16)
						if err != nil { // target error
							p("CONNECT %s ERROR %v\n", hostPort, err)
							session.Send(packet.Conn, []byte{})
							return
						}

						// send local data to target
						go func() {
							for {
								data := <-dataFromLocal[packet.Conn.Id]
								if len(data) > 0 {
									p("FROM LOCAL %d TO %s\n", len(data), hostPort)
									conn.Write(data)
								} else {
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
								session.Send(packet.Conn, []byte{})
								return
							}
							data = data[:n]
							p("FROM %s DATA %d\n", hostPort, n)
							session.Send(packet.Conn, obfuscate(data))
						}
					}()

				} else { // data from local
					dataFromLocal[packet.Conn.Id] <- packet.Data
				}
			}
		}()
	}
}
