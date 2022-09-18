package main

import (
	"fmt"
	"os"
	"runtime"
)

var DATA_PORT = 42069

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
		show_error(err, "find link-local failed")
		terminate()
	}

	switch command {
	case "s":
		if len(paths) == 0 {
			show_error(nil, "specify a file or folder")
			terminate()
		}
		remote := discover(local, link)
		send(paths, local, remote)
	case "r":
		if len(paths) != 0 {
			os.Chdir(paths[0])
		}

		wd, _ := os.Getwd()

		show_info(fmt.Sprintf("receiving into %s...", wd))

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
