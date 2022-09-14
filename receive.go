package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

func write_file_from_channel(path string, size int64, channel chan []byte) {
	//creates the full directory structure if it doesnt exist
	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		fmt.Println(err)
		return
	}

	f, err := os.Create(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	writer := bufio.NewWriter(f)

	err = write_from_channel(writer, size, channel)

	f.Close()
	if err != nil {
		fmt.Println(err.Error())
		//error during transfer, delete the (malformed) file
		//    network errors will be signalled by a nil on the channel (caught as "link terminated")
		os.Remove(path)
	}
}

func read_files(conn net.Conn) {
	//read all files off the connection until it is closed
	//non-errored connections terminate with read_header returning EOF
	defer conn.Close()
	channel := make(chan []byte)

	reader := bufio.NewReader(conn)

	for {
		size, path, err := read_header(reader)
		if err == io.EOF {
			return
		} else if err != nil {
			fmt.Println(err)
			return
		}

		start := GetTime()

		go write_file_from_channel(path, size, channel)

		err = read_into_channel(reader, size, channel, false)
		if err != nil {
			//networking error encountered, signal an error to write_file
			channel <- nil
			fmt.Printf("%s FAILED\n", path)
			return
		}

		elapsed := float64(GetTime()-start) / 1000000.0

		guard.Lock()
		fmt.Printf("%s ", path)
		set_timing_color(elapsed)
		fmt.Printf("%s\n", format_elapsed(elapsed))
		reset_color()
		guard.Unlock()
	}
}

func receive(local string) {
	ln, err := net.Listen("tcp6", fmt.Sprintf("[%s]:%d", local, DATA_PORT))
	if err != nil {
		fmt.Println(err)
		terminate()
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}
		go read_files(conn)
	}
}
