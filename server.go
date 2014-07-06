package main

import (
	"fmt"
	"log"
	"net"
	"sort"
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
	p("Started.\n")

	// debug
	if debug != "" {
		server.StartDebug(debug)
	}

	// handle session
	for {
		session := <-server.NewSession
		hostPorts := make(map[uint32]string)
		session.AddDebugEntry(func() (ret []string) {
			ret = append(ret, "<Hostports>")
			var ids []int
			for id, _ := range hostPorts {
				ids = append(ids, int(id))
			}
			sort.Ints(ids)
			for _, id := range ids {
				ret = append(ret, fmt.Sprintf("%10d %s", id, hostPorts[uint32(id)]))
			}
			return
		})
		// read packets
		go func() { //TODO exit
			packetFromLocal := make(map[uint32]chan *van.Packet)
			for {
				packet := <-session.Recv

				// check conn existence
				if _, ok := packetFromLocal[packet.Conn.Id]; !ok {
					hostPort := string(obfuscate(packet.Data))
					hostPorts[packet.Conn.Id] = hostPort
					// set receive chan
					packetFromLocal[packet.Conn.Id] = make(chan *van.Packet, 512)

					// connect to target host and read
					go func() {
						conn, err := net.DialTimeout("tcp", hostPort, time.Second*16)
						if err != nil { // target error
							session.Finish(packet.Conn)
							delete(hostPorts, packet.Conn.Id)
							return
						}

						// send local data to target
						go func() {
							for {
								packet := <-packetFromLocal[packet.Conn.Id]
								switch packet.Type {
								case van.DATA:
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
								// send close packet
								session.Finish(packet.Conn)
								delete(hostPorts, packet.Conn.Id)
								return
							}
							data = data[:n]
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
