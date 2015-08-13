package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

type GlobalState struct {
	EndServerState string
	Lock           sync.Mutex
	UsersConnected int
	CachedBanner   []byte
	ServerList     map[string]interface{}
	BackendHost    *string
}

var GState GlobalState

func main() {
	GState = GlobalState{}
	GState.EndServerState = "Offline"
	listenport := flag.String("listen", ":25565", "The port / IP combo you want to listen on")
	GState.BackendHost = flag.String("backend", "localhost:25567", "The IP address that the MC server listens on when it's online")
	flag.Parse()

	lis, err := net.Listen("tcp", *listenport)
	LazyHandle(err)

	for {
		con, err := lis.Accept()
		if err != nil {
			log.Printf("Huh. Unable to accept a connection :( (%s)", err.Error())
			continue
		}
		go HandleConnection(con)
	}
}

func HandleConnection(con net.Conn) {
	defer con.Close()
	packet_id := uint(0)
	data := []byte{}
	packet := []byte{}
	r := bufio.NewReader(con)

	// Read incoming packet
	packet_id, data, packet = ReadPacket(r)
	i := 0

	if packet_id == 0x00 {
		// Handshake
		log.Printf("<%s> Received handshake", con.RemoteAddr().String())

		// Protocol version
		_, bytes_read := ReadVarint(data[i:])
		if bytes_read <= 0 {
			log.Printf("<%s> An error occured when reading protocol version of handshake packet: %d", con.RemoteAddr().String(), bytes_read)
			return
		}
		i += bytes_read

		// Address
		_, bytes_read = ReadString(data[i:])
		if bytes_read <= 0 {
			log.Printf("<%s> An error occured when reading server address of handshake packet: %d", con.RemoteAddr().String(), bytes_read)
			return
		}
		i += bytes_read

		// Port
		//port := binary.BigEndian.Uint16(data[i:i + 2])
		i += 2

		// Next state
		next_state, bytes_read := ReadVarint(data[i:])
		if (bytes_read <= 0) {
			log.Printf("<%s> An error occured when reading next state of handshake packet: %d", con.RemoteAddr().String(), bytes_read)
			return
		}

		if next_state == 0x01 {
			// Server list request
			log.Printf("<%s> Received server list request", con.RemoteAddr().String())

			// Consume request packet
			packet_id2, data2, _ := ReadPacket(r)
			if packet_id2 == 0x00 && len(data2) == 0 {
				log.Printf("<%s> Received request", con.RemoteAddr().String())

				// Generating response
				server_list_json := GetServerList()
				response := MakePacket(0x00, MakeString(server_list_json))
				con.Write(response)
				log.Printf("<%s> Sent response", con.RemoteAddr().String())
			}

			// Prepare for ping request
			packet_id2, _, packet2 := ReadPacket(r)
			i = 0
			if packet_id2 == 0x01 {
				// Ping
				log.Printf("<%s> Received ping", con.RemoteAddr().String())

				// Send same packet back to the client
				con.Write(packet2)
				log.Printf("<%s> Sent pong", con.RemoteAddr().String())

				// Done
				return
			}
		} else if next_state == 0x02 {
			// Login request
			log.Printf("<%s> Received login request", con.RemoteAddr().String())

			if GState.EndServerState == "Offline" {
				GState.Lock.Lock()
				if GState.EndServerState == "Offline" {
					GState.EndServerState = "Starting"
					go RunStartScript()
					log.Printf("<%s> Server starting, aborting request", con.RemoteAddr().String())
					con.Write(MakePacket(0x00, MakeString("\"Server is now starting up! Please wait a few minutes before reconnecting.\"")))
				}
				GState.Lock.Unlock()
			}
			if GState.EndServerState == "Starting" {
				log.Printf("<%s> Server is still starting, aborting request", con.RemoteAddr().String())
				con.Write(MakePacket(0x00, MakeString("\"Server is still starting! Please wait before reconnecting.\"")))
				return
			}

			// Proceed to relay packets to the real server
			log.Printf("<%s> Player logging in", con.RemoteAddr().String())
			Scon, err := net.Dial("tcp", *GState.BackendHost)
			defer Scon.Close()

			if err != nil {
				GState.EndServerState = "Offline"
				log.Printf("<%s> Error while connecting to the backend server: %s", con.RemoteAddr().String(), err)
				con.Write(MakePacket(0x00, MakeString("\"Could not connect to the backend server. Please notify the server administrator.\"")))
				return
			}

			_, err = Scon.Write(packet)
			if err != nil {
				log.Printf("<%s> Error while relaying data to the backend server: %s", con.RemoteAddr().String(), err)
				con.Write(MakePacket(0x00, MakeString("\"Could not relay data to the backend server. Please notify the server administrator.\"")))
				return
			}

			GState.UsersConnected++
			log.Printf("There are now %d people connected", GState.UsersConnected)
			go io.Copy(Scon, con)
			io.Copy(con, Scon)
			GState.UsersConnected--
			log.Printf("There are now %d people connected", GState.UsersConnected)

			if GState.UsersConnected == 0 {
				// Set a reminder to check in 1 min and then shut the server down
				go KillTimer(1)
			}
		}
	}
}

func ReadPacket(r *bufio.Reader) (uint, []byte, []byte) {
	length, err := binary.ReadUvarint(r)
	if err != nil {
		if err != io.EOF {
			log.Printf("An error occured when reading packet length: %d", err)
		}
		return 0, nil, nil
	}
	packet := make([]byte, length)
	bytes_read, _ := io.ReadFull(r, packet)

	if int(length) != bytes_read {
		full_packet := append(MakeVarint(int(length)), packet...)
		log.Printf("Received unknown packet, proceeding as legacy packet 0x%x", length)
		return uint(length), packet, full_packet
	}

	// Read packet id
	packet_id, bytes_read := ReadVarint(packet)
	if bytes_read <= 0 {
		log.Printf("An error occured when reading packet id of packet: %d", bytes_read)
		return 0, nil, nil
	}
	i := bytes_read

	if length == 0 {
		return uint(packet_id), []byte{}, append(MakeVarint(int(length)), packet...)
	} else {
		return uint(packet_id), packet[i:], append(MakeVarint(int(length)), packet...)
	}
}

func MakePacket(packet_id int, data []byte) ([]byte) {
	packet := append(MakeVarint(packet_id), data...)
	return append(MakeVarint(len(packet)), packet...)
}

func ReadVarint(data []byte) (int, int) {
	value, bytes_read := binary.Uvarint(data)
	if bytes_read <= 0 {
		log.Printf("An error occured while reading varint: %d", bytes_read)
		return 0, bytes_read
	}
	return int(value), bytes_read
}

func MakeVarint(value int) ([]byte) {
	temp := make([]byte, 10)
	bytes_written := binary.PutUvarint(temp, uint64(value))
	return temp[:bytes_written]
}

func ReadString(data []byte) (string, int) {
	length, bytes_read := ReadVarint(data)
	if bytes_read <= 0 {
		log.Printf("An error occured while reading string: %d", bytes_read)
		return "", bytes_read
	}
	return string(data[bytes_read:bytes_read + length]), bytes_read + length
}

func MakeString(str string) ([]byte) {
	data := []byte(str)
	return append(MakeVarint(len(data)), data...)
}

func KillTimer(minutes int) {
	time.Sleep(time.Duration(minutes) * time.Minute)

	if GState.UsersConnected == 0 {
		log.Printf("Shutting down server due to idle...")
		cmd := exec.Command("./StopServer")
		err := cmd.Start()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Waiting for command to finish...")
		err = cmd.Wait()
		log.Printf("Server offline")
		GState.EndServerState = "Offline"
	}
}

func RunStartScript() {
	log.Printf("Starting Server Script")
	cmd := exec.Command("./StartServer")
	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Waiting for command to finish...")
	err = cmd.Wait()
	log.Printf("Server online")
	GState.EndServerState = "Online"

	// Set command to shut down server in 5 minutes if no one connects
	go KillTimer(5)
}

func LazyHandle(err error) {
	if err != nil {
		log.Fatal(err.Error())
	}
}

func GetServerList() (string) {
	if GState.ServerList == nil {
		// Construct our placeholder server list from scratch
		GState.ServerList = map[string]interface{} {
			"version": map[string]interface{} {
				"name": "unknown",
				"protocol": 4,
			},
			"players": map[string]interface{} {
				"max": 0,
				"online": 0,
			},
			"description": map[string]interface{} {
				"text": "Join to initialize description",
			},
		}
	}

	old_motd := GetServerDescription()

	if GState.EndServerState == "Offline" {
		SetServerDescription(old_motd + " (idle)")
	} else if GState.EndServerState == "Starting" {
		SetServerDescription(old_motd + " (starting)")
	} else if GState.EndServerState == "Online" {
		// Server is online
		scon, err := net.Dial("tcp", *GState.BackendHost)
		LazyHandle(err)
		r := bufio.NewReader(scon)
		defer scon.Close()

		if err != nil {
			GState.EndServerState = "Offline"
		} else {
			// Create our own handshake to request the real server list
			// Protocol version
			handshake := MakeVarint(4)

			// Address
			handshake = append(handshake, MakeString("localhost")...)

			// Port
			_, port_str, _ := net.SplitHostPort(*GState.BackendHost)
			port, _ := strconv.Atoi(port_str)
			temp_port := make([]byte, 2)
			binary.BigEndian.PutUint16(temp_port, uint16(port))
			handshake = append(handshake, temp_port...)

			// Next state
			handshake = append(handshake, MakeVarint(1)...)

			// Make packet and send handshake and request
			handshake = MakePacket(0x00, handshake)
			scon.Write(handshake)

			request := MakePacket(0x00, nil)
			scon.Write(request)

			log.Println("Sent server list request to server")

			// Receive data
			_, data, _ := ReadPacket(r)

			server_list_json, _ := ReadString(data)
			var json_data interface{}
			json.Unmarshal([]byte(server_list_json), &json_data)

			// Check if it's a valid server list, otherwise we assume the server is still starting
			switch json_data.(type) {
			case map[string]interface{}:
				// Appears to be valid, update server description
				log.Println("Received server list response from server")
				GState.ServerList = json_data.(map[string]interface{})
				return server_list_json
			default:
				// Not a valid response, server is not ready yet
				SetServerDescription(old_motd + " (readying up)")
			}
		}
	}

	server_list_json, _ := json.Marshal(GState.ServerList)

	// Reset our description again
	SetServerDescription(old_motd)

	return string(server_list_json)
}

func GetServerDescription() (string) {
	switch GState.ServerList["description"].(type) {
	case string:
		// In case the description is a string directly
		return GState.ServerList["description"].(string)
	default:
		// In other cases description should be an object that contains text
		return GState.ServerList["description"].(map[string]interface{})["text"].(string)
	}
}

func SetServerDescription(text string) {
	switch GState.ServerList["description"].(type) {
	case string:
		// In case the description is a string directly
		GState.ServerList["description"] = text
	default:
		// In other cases description should be an object that contains text
		GState.ServerList["description"].(map[string]interface{})["text"] = text
	}
}
