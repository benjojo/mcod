package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os/exec"
	"sync"
	"time"
)

type GlobalState struct {
	EndServerState string
	Lock           sync.Mutex
	UsersConnected int
	CachedBanner   []byte
	EnableCache    *bool
}

var GState GlobalState

func main() {
	GState = GlobalState{}
	GState.EndServerState = "Offline"
	GState.EnableCache = flag.Bool("cachebanner", true, "disable this if in the future they change the handshake proto")
	flag.Parse()

	KillTimer(true)
	lis, err := net.Listen("tcp", ":25565")
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
	// Uh quick figure out if it's a banner request or a real thing.

	vhost_chunk := make([]byte, 1024)
	vhost_len, err := con.Read(vhost_chunk)
	first_chunk := make([]byte, 1024)
	first_len, err := con.Read(first_chunk)

	//quick sanity check here

	if *GState.EnableCache && vhost_len != 0 && vhost_chunk[1] == 0x00 && vhost_chunk[vhost_len-1] == 0x01 {
		// kk, so the vhost is probs valid
		if (first_chunk[0] == 0x01 && first_chunk[1] == 0x00) || first_len == 0 {
			if len(GState.CachedBanner) != 0 && (GState.EndServerState == "Offline" || GState.EndServerState == "Starting") {
				log.Println("Serving the banner from cache")

				con.Write(GState.CachedBanner)
				con.Close()
				return
			} else if GState.EndServerState == "Online" {
				// Cache time!
				log.Println("Pulling a new copy of the banner from the origin and caching for future use")

				Scon, err := net.Dial("tcp", "localhost:25567")
				defer Scon.Close()
				defer con.Close()
				if err != nil {
					GState.EndServerState = "Offline"
					return
				}

				_, err = Scon.Write(vhost_chunk[0:vhost_len])
				if err != nil {
					log.Printf("Error in pulling a cached banner. Error was: %s", err)
					return
				}
				_, err = Scon.Write(first_chunk[0:first_len])
				if err != nil {
					log.Printf("Error in pulling a cached banner. Error was: %s", err)
					return
				}
				banner_chunk := make([]byte, 25565)
				read, err := Scon.Read(banner_chunk)
				if err != nil {
					log.Printf("Error in pulling a cached banner. Error was: %s", err)
					return
				}
				GState.CachedBanner = banner_chunk[0:read]
				con.Write(banner_chunk[0:read])
				return
			}
		}
	}

	log.Printf("New Player Joining...")

	if GState.EndServerState == "Offline" {
		GState.Lock.Lock()
		if GState.EndServerState == "Offline" {

			GState.EndServerState = "Starting"
			RunStartScript()

		}
		GState.Lock.Unlock()

	}
	if GState.EndServerState == "Starting" {
		log.Printf("Player is waiting for server to start...")
		GState.Lock.Lock()
		GState.Lock.Unlock()
	}

	Scon, err := net.Dial("tcp", "localhost:25567")
	if err != nil {
		con.Close()
		GState.EndServerState = "Offline"
		return
	}

	_, err = Scon.Write(vhost_chunk[0:vhost_len])
	if err != nil {
		log.Printf("Error in pulling connection online to backend, error was: %s", err)
		con.Close()
		return
	}
	_, err = Scon.Write(first_chunk[0:first_len])
	if err != nil {
		log.Printf("Error in pulling connection online to backend, error was: %s", err)
		con.Close()
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
		go KillTimer(false)
	}

	con.Close()
}

func KillTimer(force bool) {
	if !force {
		time.Sleep(time.Minute)
	}
	if GState.UsersConnected == 0 {
		log.Printf("Shutting down server due to idle...")
		cmd := exec.Command("killall", "java")
		err := cmd.Start()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Waiting for command to finish...")
		err = cmd.Wait()
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
	log.Printf("Command finished with error: %v", err)
	GState.EndServerState = "Online"
}

func LazyHandle(err error) {
	if err != nil {
		log.Fatal(err.Error())
	}
}
