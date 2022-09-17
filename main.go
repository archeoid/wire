package main

import (
	"fmt"
	"os"
	"runtime"
)

var DATA_PORT = 42069
var CHUNK_SIZE int = 1024 * 1024
var SEND_PORT = 42070
var RECV_PORT = 42072
var MULTICAST string = "ff02::1"
var REQUEST = "REQUEST"

func main() {
	self := os.Args[0]
	args := os.Args[1:]

	if len(args) == 0 {
		//on windows install by just running the binary
		if runtime.GOOS == "windows" {
			install(self)
		} else {
			help()
		}
		terminate()
	}

	command := args[0]
	paths := args[1:]

	//should always find an address if theres an ethernet interface with ipv6 enabled (and its up)
	//unlike ipv4, ipv6 has mandatory link-local address and are stateless (derived from the physical address)
	local, link, err := find_link_local_address()
	if err != nil {
		fmt.Println(err.Error())
		terminate()
	}

	switch command {
	case "s":
		if len(paths) == 0 {
			fmt.Println("specify a file or folder")
			terminate()
		}
		remote := discover(local, link)
		send(paths, local, remote)
	case "r":
		if len(paths) != 0 {
			os.Chdir(paths[0])
		}

		wd, _ := os.Getwd()

		fmt.Printf("\033[38;5;220mreceiving into %s...\033[0m\n", wd)
		go responder(local, link)
		receive(local)
	case "i":
		install(self)
	case "u":
		uninstall()
	case "h":
		help()
	}
}
