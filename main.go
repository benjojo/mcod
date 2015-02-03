package main

import (
	"flag"
	"fmt"
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
	BackendHost    *string
	EnableCache    *bool
}

var GState GlobalState

func main() {
	GState = GlobalState{}
	GState.EndServerState = "Offline"
	GState.EnableCache = flag.Bool("cachebanner", true, "disable this if in the future they change the handshake proto")
	listenport := flag.String("listen", ":25565", "The port / IP combo you want to listen on")
	GState.BackendHost = flag.String("backend", "localhost:25567", "The IP address that the MC server listens on when it's online")
	flag.Parse()

	KillTimer(true)
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
	// Check if we can go into cache logic for the server banners
	vhost_chunk := make([]byte, 1024)
	vhost_len := 0
	defer con.Close()

	if *GState.EnableCache {
		vhost_len, _ = con.Read(vhost_chunk)

		if vhost_chunk[vhost_len-3] == 0x01 && vhost_chunk[vhost_len-1] == 0x00 {
			err := HandleCachedBanner(con, vhost_chunk[:vhost_len], false)
			if err == nil {
				return
			}
		} else if vhost_chunk[vhost_len-1] == 0x01 {
			ping_chunk := make([]byte, 1024)
			con.Read(ping_chunk) // Begone
			err := HandleCachedBanner(con, vhost_chunk[:vhost_len], true)
			if err == nil {
				return
			}

		} else {
			log.Printf("Uncacheable request passing to server...")
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

	Scon, err := net.Dial("tcp", *GState.BackendHost)
	if err != nil {
		con.Close()
		GState.EndServerState = "Offline"
		return
	}

	if *GState.EnableCache {
		_, err = Scon.Write(vhost_chunk[0:vhost_len])
		if err != nil {
			log.Printf("Error in pulling connection online to backend, error was: %s", err)
			con.Close()
			return
		}
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

func HandleCachedBanner(con net.Conn, vhost_chunk []byte, appendping bool) (err error) {
	if len(GState.CachedBanner) != 0 && (GState.EndServerState == "Offline" || GState.EndServerState == "Starting") {
		log.Println("Serving the banner from cache")

		con.Write(GState.CachedBanner)
		return err
	} else if GState.EndServerState == "Online" {
		// Cache time!
		log.Println("Pulling a new copy of the banner from the origin and caching for future use")

		Scon, err := net.Dial("tcp", *GState.BackendHost)
		LazyHandle(err)

		defer Scon.Close()
		if err != nil {
			GState.EndServerState = "Offline"
			return err
		}

		if appendping {
			vhost_chunk = append(vhost_chunk, 0x01)
			vhost_chunk = append(vhost_chunk, 0x00)
		}

		_, err = Scon.Write(vhost_chunk)
		if err != nil {
			log.Printf("Error in pulling a cached banner. Error was: %s", err)
			return err
		}
		banner_chunk := make([]byte, 25565)
		read, err := Scon.Read(banner_chunk)
		if err != nil {
			log.Printf("Error in pulling a cached banner. Error was: %s", err)
			return err
		}
		GState.CachedBanner = banner_chunk[0:read]
		con.Write(banner_chunk[0:read])
		return err
	} else {
		return fmt.Errorf("Not cached")
	}
}
