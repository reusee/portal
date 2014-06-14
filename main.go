package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"

	"net/http"
	_ "net/http/pprof"

	socks "github.com/reusee/socks5-server"
	"github.com/reusee/van"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const (
	CONNECT = byte(4)
	DATA    = byte(2)
	CLOSE   = byte(6)
)

var p = fmt.Printf

func main() {
	if len(os.Args) == 1 {
		goto usage
	}

	for _, arg := range os.Args[1:] {
		parts := strings.Split(arg, "-")
		switch parts[0] {

		// server
		case "server":
			// start server
			server, err := van.NewServer(parts[1])
			if err != nil {
				log.Fatalf("van.NewServer %v", err)
			}
			defer server.Close()

			// handle session
			for session := range server.NewSession {
				// read packets
				go func() { //TODO exit
					dataFromLocal := make(map[int64]chan []byte)
					closeFromLocal := make(map[int64]chan bool)
					for packet := range session.Recv {
						var sessionId int64
						reader := bytes.NewReader(packet)
						binary.Read(reader, binary.LittleEndian, &sessionId) // read session id
						packetType, _ := reader.ReadByte()                   // read packet type
						switch packetType {

						// new connect
						case CONNECT:
							// read host port
							hostPort, _ := ioutil.ReadAll(reader)
							p("CONNECT %s\n", hostPort)
							// set receive chan
							dataFromLocal[sessionId] = make(chan []byte, 512)
							closeFromLocal[sessionId] = make(chan bool, 1)
							// connect to target host and read
							go func() {
								conn, err := net.DialTimeout("tcp", string(hostPort), time.Second*8)
								if err != nil { // target error
									p("CONNECT %s ERROR %v\n", hostPort, err)
									buf := new(bytes.Buffer)
									binary.Write(buf, binary.LittleEndian, sessionId)
									buf.WriteByte(CLOSE)
									session.Send(buf.Bytes())
									return
								}
								// send local data to target
								go func() {
									for {
										select {
										case data := <-dataFromLocal[sessionId]:
											p("FROM LOCAL %d TO %s\n", len(data), hostPort)
											conn.Write(data)
										case <-closeFromLocal[sessionId]:
											conn.Close()
											return
										}
									}
								}()
								for {
									data := make([]byte, 1500)
									n, err := conn.Read(data)
									if err != nil { // target error
										p("TARGET %s READ ERROR %v\n", hostPort, err)
										// send close packet
										buf := new(bytes.Buffer)
										binary.Write(buf, binary.LittleEndian, sessionId)
										buf.WriteByte(CLOSE)
										session.Send(buf.Bytes())
										return
									}
									data = data[:n]
									p("FROM %s DATA %d\n", hostPort, n)
									buf := new(bytes.Buffer)
									binary.Write(buf, binary.LittleEndian, sessionId)
									buf.WriteByte(DATA)
									buf.Write(data)
									session.Send(buf.Bytes())
								}
							}()

						// data from local
						case DATA:
							data, _ := ioutil.ReadAll(reader)
							dataFromLocal[sessionId] <- data

						// close from local
						case CLOSE:
							closeFromLocal[sessionId] <- true
						}
					}
				}()
			}

		// local
		case "local":
			// connect to remote
			client, err := van.NewClient(parts[1])
			if err != nil {
				log.Fatalf("van.NewClient %v", err)
			}
			defer client.Close()

			// start socks server
			socksServer, err := socks.New(parts[2])
			if err != nil {
				log.Fatalf("socks5.NewServer %v", err)
			}
			defer socksServer.Close()

			// globals
			socksConns := make(map[int64]net.Conn)
			hostPorts := make(map[int64]string)

			// handle socks client
			socksServer.OnSignal("client", func(args ...interface{}) {
				go func() {
					conn := args[0].(net.Conn)
					hostPort := args[1].(string)
					p("SOCKS CLIENT TO %s\n", hostPort)
					// pick a session id
					sessionId := rand.Int63()
					hostPorts[sessionId] = string(hostPort)
					// set socksConns
					socksConns[sessionId] = conn
					// send a connect packet
					buf := new(bytes.Buffer)
					binary.Write(buf, binary.LittleEndian, sessionId)
					buf.WriteByte(CONNECT)
					buf.Write([]byte(hostPort))
					client.Send(buf.Bytes())
					// read from socks conn and send to remote
					for {
						data := make([]byte, 1500)
						n, err := conn.Read(data)
						if err != nil { // socks conn closed
							// send a close packet
							buf := new(bytes.Buffer)
							binary.Write(buf, binary.LittleEndian, sessionId)
							buf.WriteByte(CLOSE)
							client.Send(buf.Bytes())
							return
						}
						data = data[:n]
						p("FROM LOCAL %d TO %s\n", n, hostPort)
						buf := new(bytes.Buffer)
						binary.Write(buf, binary.LittleEndian, sessionId)
						buf.WriteByte(DATA)
						buf.Write(data)
						client.Send(buf.Bytes())
					}
				}()
			})

			// handle packet from remote
			for packet := range client.Recv {
				reader := bytes.NewReader(packet)
				var sessionId int64
				binary.Read(reader, binary.LittleEndian, &sessionId) // read session id
				packetType, _ := reader.ReadByte()                   // read packet type
				switch packetType {
				// data from remote
				case DATA:
					data, _ := ioutil.ReadAll(reader)
					p("FROM TARGET %s TO LOCAL %d\n", hostPorts[sessionId], len(data))
					socksConns[sessionId].Write(data)
				case CLOSE:
					p("REMOTE %s CLOSE\n", hostPorts[sessionId])
					socksConns[sessionId].Close()
				}
			}

		case "pprof":
			go http.ListenAndServe(parts[1], nil)

		default:
			goto usage
		}
	}

	return

usage:
	p("usage: %s [server-addr:port] / [local-remote:port-local:port]", os.Args[0])
	return
}
