package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

var connections int = 0
var guard sync.Mutex

func receive_display_progress(t transfer) {
	// fancy display showing progress
	// only suitable for 1 connection sending 1 file at a time

	progress := float64(t.progress) / float64(t.size)

	fmt.Printf("\033[G\033[J%s ", t.name)

	if t.progress != t.size {
		set_progress_color(progress)
		fmt.Printf("%5.1f%%", 100.0*progress)
		reset_color()
	}
	if t.progress == t.size {
		elapsed := float64(get_time()-t.start) / 1000000.0
		set_timing_color(elapsed)
		fmt.Printf("%s\n", format_elapsed(elapsed))
		reset_color()
	}
}

func receive_display_basic(t transfer) {
	// basic display for multiple concurrent connections
	// need to guard terminal output to avoid mangling text

	guard.Lock()
	defer guard.Unlock()

	if t.progress == t.size {
		fmt.Printf("%s ", t.name)
		elapsed := float64(get_time()-t.start) / 1000000.0
		set_timing_color(elapsed)
		fmt.Printf("%s\n", format_elapsed(elapsed))
		reset_color()
	}
}

func receive_display(t transfer) {
	if connections == 1 {
		receive_display_progress(t)
	} else {
		receive_display_basic(t)
	}
}

func add_connection_display() {
	guard.Lock()
	defer guard.Unlock()

	prev := connections
	connections++
	if prev == 1 {
		title_color()
		fmt.Printf("\033[G\033[JSWITCHING TO BASIC DISPLAY\n")
		reset_color()
	}
}

func remove_connection_display() {
	guard.Lock()
	defer guard.Unlock()

	prev := connections
	connections--
	if prev == 2 {
		title_color()
		fmt.Printf("\033[G\033[JSWITCHING TO PROGESS DISPLAY\n")
		reset_color()
	}
}

func receive_all(conn net.Conn) error {
	var err error
	reader := bufio.NewReader(conn)
	add_connection_display()

	for {
		if _, err = reader.Peek(1); err != nil {
			break
		}

		var t transfer
		if t, err = from_wire(reader); err != nil {
			break
		}

		if err = to_disk(reader, t, receive_display); err != nil {
			break
		}
	}

	if err == io.EOF {
		err = nil
	}

	if err != nil {
		if connections == 1 {
			fmt.Println()
		}
		show_error(err, "FAIL")
	}

	remove_connection_display()
	return err
}

func receive(local string) {
	addr, _ := net.ResolveTCPAddr("tcp6", fmt.Sprintf("[%s]:%d", local, DATA_PORT))
	ln, err := net.ListenTCP("tcp6", addr)
	if err != nil {
		show_error(err, "listening failed")
		terminate()
	}
	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			show_error(err, "accepting failed")
			continue
		}
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Second)

		go receive_all(conn)
	}
}
