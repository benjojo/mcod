package main

import (
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
}

var GState GlobalState

func main() {
	GState = GlobalState{}
	GState.EndServerState = "Offline"
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
}

func LazyHandle(err error) {
	if err != nil {
		log.Fatal(err.Error())
	}
}
