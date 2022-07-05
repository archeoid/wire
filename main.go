package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/ipv6"
)

var DATA_PORT = 42069
var SEND_PORT = 42070
var RECV_PORT = 42072

var CHUNK_SIZE int = 1024 * 1024

func min(a int, b int64) int {
	if int64(a) <= b {
		return a
	} else {
		return int(b)
	}
}

func copy_file(source, destination string) error {
	s, err := os.Open(source)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer d.Close()

	d.Chmod(0700)

	_, err = io.Copy(d, s)
	if err != nil {
		return err
	}

	return nil
}

func format_elapsed(ms float64) string {
	if ms < 1.0 {
		return fmt.Sprintf("%.3fms", ms)
	} else if ms < 1000.0 {
		return fmt.Sprintf("%.1fms", ms)
	} else if ms < 60000.0 {
		return fmt.Sprintf("%.3fs\n", ms/1000.0)
	} else {
		return fmt.Sprintf("%.3fm\n", ms/60000.0)
	}
}

func build_header(path, name string) (header []byte, header_size int64, file_size int64, err error) {
	name = filepath.ToSlash(name)

	name_size := len(name)

	header_size = int64(10 + name_size)
	header = make([]byte, header_size)
	copy(header[10:], name[:])

	binary.BigEndian.PutUint16(header[8:], (uint16)(name_size))

	i, err := os.Stat(path)
	if err != nil {
		return header, header_size, file_size, err
	}
	file_size = i.Size()

	binary.BigEndian.PutUint64(header[0:], (uint64)(file_size))

	return header, header_size, file_size, nil
}

func read_all(b *bufio.Reader, data []byte) error {
	total := 0
	len := len(data)
	for total != len {
		n, err := b.Read(data[total:])
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

func read_header(b *bufio.Reader) (file_size int64, path string, err error) {
	header := make([]byte, 10)

	err = read_all(b, header)
	if err != nil {
		return 0, "", err
	}

	file_size = int64(binary.BigEndian.Uint64(header[0:8]))
	path_size := int(binary.BigEndian.Uint16(header[8:10]))

	path_data := make([]byte, path_size)

	err = read_all(b, path_data)
	if err != nil {
		return 0, "", err
	}

	path = string(path_data)

	path = filepath.FromSlash(path)

	return file_size, path, nil
}

func read_into_channel(b *bufio.Reader, size int64, c chan []byte) (err error) {
	total := int64(0)
	var n int
	for {
		buffer := make([]byte, min(CHUNK_SIZE, size-total))
		n, err = b.Read(buffer[:])
		if err == io.EOF {
			return fmt.Errorf("link terminated")
		}
		if err != nil {
			return err
		}

		total += int64(n)
		c <- buffer[:n]

		if total == size {
			return nil
		}
	}
}

func write_from_channel(b *bufio.Writer, size int64, c chan []byte) (err error) {
	defer b.Flush()
	total := int64(0)
	for {
		chunk := <-c

		if chunk == nil {
			return fmt.Errorf("link terminated")
		}

		_, err = b.Write(chunk)
		if err != nil {
			return err
		}

		total += int64(len(chunk))
		if total == size {
			return nil
		}
	}
}

func read_file(path string, size int64, c chan []byte) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	b := bufio.NewReader(f)

	err = read_into_channel(b, size, c)
	if err != nil {
		fmt.Println(err)
	}

	f.Close()
}

func write_file(path string, size int64, c chan []byte) {
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
	defer f.Close()

	b := bufio.NewWriter(f)

	err = write_from_channel(b, size, c)
	if err != nil {
		//error during tranfer, delete the file
		os.Remove(path)
	}
}

func read_file_onto_channel(c chan []byte, path, name string) (size int64, err error) {
	header, header_size, file_size, err := build_header(path, name)
	if err != nil {
		return 0, err
	}
	c <- header[:]

	go read_file(path, file_size, c)

	return header_size + file_size, nil
}

func send_file(b *bufio.Writer, path, name string) error {
	fmt.Println(name)
	data := make(chan []byte, 10)
	size, err := read_file_onto_channel(data, path, name)
	if err != nil {
		return err
	}
	return write_from_channel(b, size, data)
}

func send_folder(b *bufio.Writer, base string) error {
	data := make(chan []byte, 10)
	parent := filepath.Dir(base)
	filepath.WalkDir(base, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			start := time.Now().UnixMicro()
			rel, _ := filepath.Rel(parent, path)
			size, err := read_file_onto_channel(data, path, rel)
			if err != nil {
				return err
			}
			err = write_from_channel(b, size, data)
			if err != nil {
				return err
			}

			elapsed := float64(time.Now().UnixMicro()-start) / 1000.0
			fmt.Printf("%s %s\n", path, format_elapsed(elapsed))
		}
		return nil
	})

	return nil

}

func expand_paths(paths []string) []string {
	out := make([]string, 0)
	for _, path := range paths {
		matches, err := filepath.Glob(path)
		if err != nil {
			out = append(out, path)
		} else {
			out = append(out, matches...)
		}
	}
	return out
}

func send(paths []string, local, remote string) {
	var err error

	raddr, _ := net.ResolveTCPAddr("tcp6", fmt.Sprintf("%s:%d", remote, DATA_PORT))
	laddr, _ := net.ResolveTCPAddr("tcp6", fmt.Sprintf("%s:%d", local, DATA_PORT))
	laddr.Port = 0 //OS will assign a random port

	conn, err := net.DialTCP("tcp6", laddr, raddr)
	if err != nil {
		fmt.Println(err)
		terminate()
	}
	defer conn.Close()

	b := bufio.NewWriter(conn)

	//expand wildcards for windows because its shit
	paths = expand_paths(paths)

	for _, path := range paths {
		i, err := os.Stat(path)
		if err != nil {
			break
		}
		is_dir := i.IsDir()

		if !is_dir {
			err = send_file(b, path, i.Name())
		} else {
			err = send_folder(b, path)
		}

		if err != nil {
			break
		}
	}

	if err != nil {
		fmt.Println(err)
		terminate()
	}

}

func read_files(conn net.Conn) {
	defer conn.Close()
	data := make(chan []byte, 10)

	b := bufio.NewReader(conn)

	for {
		size, path, err := read_header(b)
		if err == io.EOF {
			return
		} else if err != nil {
			fmt.Println(err)
			return
		}

		start := time.Now().UnixMicro()

		go write_file(path, size, data)

		err = read_into_channel(b, size, data)
		if err != nil {
			data <- nil
			fmt.Printf("%s FAILED\n", path)
			return
		}

		elapsed := float64(time.Now().UnixMicro()-start) / 1000.0

		fmt.Printf("%s %s\n", path, format_elapsed(elapsed))
	}
}

func receive(local string) {
	ln, err := net.Listen("tcp6", fmt.Sprintf("%s:%d", local, DATA_PORT))
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

func find_link_local_address() (ip string, i net.Interface, err error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", i, err
	}

	for _, i := range ifaces {
		name := strings.ToLower(i.Name)
		if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "enp") {
			addrs, _ := i.Addrs()
			for _, a := range addrs {
				ip := net.ParseIP(strings.Split(a.String(), "/")[0])
				if ip.IsLinkLocalUnicast() && strings.Contains(ip.String(), ":") {
					return fmt.Sprintf("[%s%%%d]", ip.String(), i.Index), i, nil
				}
			}
		}
	}
	return "", i, fmt.Errorf("ethernet interface not found")
}

var MULTICAST string = "ff02::1"
var MULTICAST_L string = "[ff02::1]"
var REQUEST = "REQUEST"

func bind_multicast(local string, port int, i net.Interface) (conn *ipv6.PacketConn) {
	c, err := net.ListenPacket("udp6", fmt.Sprintf("%s:%d", local, port))
	if err != nil {
		fmt.Println(err.Error())
		terminate()
	}

	p := ipv6.NewPacketConn(c)
	p.SetMulticastLoopback(false)

	multicast := &net.UDPAddr{IP: net.ParseIP(MULTICAST)}

	p.JoinGroup(&i, multicast)

	return p

}

func responder(local string, i net.Interface) {
	r := bind_multicast(MULTICAST_L, RECV_PORT, i)
	s := bind_multicast(local, SEND_PORT, i)

	data := make([]byte, 64)

	multicast := &net.UDPAddr{IP: net.ParseIP(MULTICAST), Port: RECV_PORT + 1}

	local_ip := strings.Split(local[1:len(local)-1], "%")[0]

	for {
		r.ReadFrom(data)
		s.WriteTo([]byte(local_ip), nil, multicast)
	}
}

func discover(local string, i net.Interface) (remote string) {
	r := bind_multicast(MULTICAST_L, RECV_PORT+1, i)
	s := bind_multicast(local, SEND_PORT+1, i)
	defer r.Close()
	defer s.Close()

	multicast := &net.UDPAddr{IP: net.ParseIP(MULTICAST), Port: RECV_PORT}

	data := make([]byte, 64)

	for {
		s.WriteTo([]byte(REQUEST), nil, multicast)
		r.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
		n, _, _, _ := r.ReadFrom(data)
		if n != 0 {
			data = data[:n]
			break
		}
	}

	remote = string(data)

	if remote == "REQUEST" {
		fmt.Println("peer is in send mode")
		terminate()
	}

	remote = fmt.Sprintf("[%s%%%d]", remote, i.Index)

	return remote
}

func help() {
	fmt.Println("wire r\n\treceive mode\nwire s PATH\n\tsend PATH\nwire i\n\tinstall\nwire u\n\tuninstall")
}

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
		fmt.Printf("receiving into %s...\n", wd)
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
