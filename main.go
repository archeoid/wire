package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
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
		return fmt.Sprintf("%.3fs", ms/1000.0)
	} else {
		return fmt.Sprintf("%.3fm", ms/60000.0)
	}
}

func build_header(path, name string) (header []byte, header_size int64, file_size int64, err error) {
	//header format {file_size uint64, name_size uint16, name string}
	//file_size excludes the header

	//convert to unix style paths
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

func read_into_buffer(reader *bufio.Reader, buffer []byte) error {
	total := 0
	len := len(buffer)
	for total != len {
		n, err := reader.Read(buffer[total:])
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

func read_header(reader *bufio.Reader) (file_size int64, path string, err error) {
	//see build_header
	header := make([]byte, 10)

	err = read_into_buffer(reader, header)
	if err != nil {
		return 0, "", err
	}

	file_size = int64(binary.BigEndian.Uint64(header[0:8]))
	path_size := int(binary.BigEndian.Uint16(header[8:10]))

	path_data := make([]byte, path_size)

	err = read_into_buffer(reader, path_data)
	if err != nil {
		return 0, "", err
	}

	path = string(path_data)

	//header uses unix style paths, if on windows this will convert back
	path = filepath.FromSlash(path)

	return file_size, path, nil
}

func read_into_channel(reader *bufio.Reader, size int64, channel chan []byte) (err error) {
	//read `size` bytes from the Reader into the channel
	//    data will be consumed concurrently by write_from_channel
	total := int64(0)
	var n int
	for {
		buffer := make([]byte, min(CHUNK_SIZE, size-total))
		n, err = reader.Read(buffer[:])
		if err == io.EOF {
			//connection ended early (total < size)
			return fmt.Errorf("link terminated")
		}
		if err != nil {
			return err
		}

		total += int64(n)
		channel <- buffer[:n]

		if total == size {
			return nil
		}

		if total > size {
			//should never happen
			return fmt.Errorf("reader misaligned")
		}
	}
}

func write_from_channel(writer *bufio.Writer, size int64, channel chan []byte) (err error) {
	defer writer.Flush()
	total := int64(0)
	for {
		chunk := <-channel

		if chunk == nil {
			//signaled by the producer that theres an error
			return fmt.Errorf("link terminated")
		}

		_, err = writer.Write(chunk)
		if err != nil {
			return err
		}

		total += int64(len(chunk))
		if total == size {
			return nil
		}

		if total > size {
			//channel should aligned (in read_into_channel) so different files dont overlap
			return fmt.Errorf("channel misaligned")
		}
	}
}

func read_file(path string, size int64, channel chan []byte) {
	//open the file and read `size` bytes of data onto the channel
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	reader := bufio.NewReader(f)

	err = read_into_channel(reader, size, channel)
	if err != nil {
		fmt.Println(err)
	}

	f.Close()
}

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

func read_file_onto_channel(channel chan []byte, path, name string) (size int64, err error) {
	header, header_size, file_size, err := build_header(path, name)
	if err != nil {
		return 0, err
	}
	channel <- header[:]

	go read_file(path, file_size, channel)

	//return the number of bytes that will be writted to channel (and need to be read)
	//    file_size excludes the header size so add it
	return header_size + file_size, nil
}

func send_file(writer *bufio.Writer, path, name string) error {
	fmt.Println(name)
	data := make(chan []byte, 10)

	//non-blocking, starts a goroutine to read in the background
	size, err := read_file_onto_channel(data, path, name)
	if err != nil {
		return err
	}

	//blocks until file sent or errored
	return write_from_channel(writer, size, data)
}

func send_folder(writer *bufio.Writer, base string) error {
	//send files sequentially over a single connection via a single channel
	//disk and network IO is done concurrently
	channel := make(chan []byte, 10)

	parent := filepath.Dir(base)

	err := filepath.WalkDir(base, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			start := time.Now().UnixMicro()

			//keep the top level folder
			//    if sending '/very/cool/folder' then our peer will receive 'folder' etc
			rel, _ := filepath.Rel(parent, path)

			//spin up a goroutine to read the file in the background
			size, err := read_file_onto_channel(channel, path, rel)
			if err != nil {
				return err
			}

			//send the data concurrently to reading it
			err = write_from_channel(writer, size, channel)
			if err != nil {
				return err
			}

			elapsed := float64(time.Now().UnixMicro()-start) / 1000.0
			fmt.Printf("%s %s\n", rel, format_elapsed(elapsed))
		}
		return nil
	})

	return err

}

func expand_paths(paths []string) []string {
	//expand wildcards in the arguments etc
	out := make([]string, 0)
	for _, path := range paths {
		matches, err := filepath.Glob(path)
		if err != nil || len(matches) == 0 {
			out = append(out, path)
		} else {
			out = append(out, matches...)
		}
	}
	return out
}

func send(paths []string, local, remote string) {
	var err error

	raddr, _ := net.ResolveTCPAddr("tcp6", fmt.Sprintf("[%s]:%d", remote, DATA_PORT))
	laddr, _ := net.ResolveTCPAddr("tcp6", fmt.Sprintf("[%s]:%d", local, DATA_PORT))
	laddr.Port = 0 //OS will assign a random port

	conn, err := net.DialTCP("tcp6", laddr, raddr)
	if err != nil {
		fmt.Println(err)
		terminate()
	}
	defer conn.Close()

	writer := bufio.NewWriter(conn)

	//expand wildcards for windows because its shit
	paths = expand_paths(paths)

	for _, path := range paths {
		var i fs.FileInfo
		i, err = os.Stat(path)
		if err != nil {
			break
		}
		is_dir := i.IsDir()

		if !is_dir {
			err = send_file(writer, path, i.Name())
		} else {
			err = send_folder(writer, path)
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

		start := time.Now().UnixMicro()

		go write_file_from_channel(path, size, channel)

		err = read_into_channel(reader, size, channel)
		if err != nil {
			//networking error encountered, signal an error to write_file
			channel <- nil
			fmt.Printf("%s FAILED\n", path)
			return
		}

		elapsed := float64(time.Now().UnixMicro()-start) / 1000.0

		fmt.Printf("%s %s\n", path, format_elapsed(elapsed))
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

func find_link_local_address() (ip string, i net.Interface, err error) {
	//find an interface that looks like ethernet and has an ipv6 link local address
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", i, err
	}

	for _, i := range ifaces {
		name := strings.ToLower(i.Name)
		//guess if an interface is ethernet by looking at its name
		//    windows has "Ethernet #", linux has "eth#" or "enp###"
		if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "enp") {
			addrs, _ := i.Addrs()
			for _, a := range addrs {
				ip := net.ParseIP(strings.Split(a.String(), "/")[0])

				//if its link local and ipv6 then use it
				if ip.IsLinkLocalUnicast() && strings.Contains(ip.String(), ":") {
					return fmt.Sprintf("%s%%%d", ip.String(), i.Index), i, nil
				}
			}
		}
	}
	return "", i, fmt.Errorf("ethernet interface not found")
}

var MULTICAST string = "ff02::1"
var REQUEST = "REQUEST"

func bind_multicast(address string, port int, i net.Interface) (conn *ipv6.PacketConn) {
	//bind to address:port and join the link-local multicast group

	c, err := net.ListenPacket("udp6", fmt.Sprintf("[%s]:%d", address, port))
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
	//to recieve a multicast we need to bind to the multicast address
	//to send a multicast we need to bind to the link local address
	r := bind_multicast(MULTICAST, RECV_PORT, i)
	s := bind_multicast(local, SEND_PORT, i)

	data := make([]byte, 64)

	//multicast to our peer thats currently in discover mode
	destination := &net.UDPAddr{IP: net.ParseIP(MULTICAST), Port: RECV_PORT + 1}

	//strip the zone identifier (peer will add their own)
	local_ip := strings.Split(local, "%")[0]

	for {
		r.ReadFrom(data) //blocks until a peer makes a connection
		s.WriteTo([]byte(local_ip), nil, destination)
	}
}

func discover(local string, i net.Interface) (remote string) {
	//use different ports to the responder so we can recieve and send concurrently
	r := bind_multicast(MULTICAST, RECV_PORT+1, i)
	s := bind_multicast(local, SEND_PORT+1, i)
	defer r.Close()
	defer s.Close()

	//multicast to our peer who's currently in responder mode
	destination := &net.UDPAddr{IP: net.ParseIP(MULTICAST), Port: RECV_PORT}

	data := make([]byte, 64)

	for {
		s.WriteTo([]byte(REQUEST), nil, destination)
		//keep sending requests every 100 milliseconds until we get a response
		r.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
		n, _, _, _ := r.ReadFrom(data)
		if n != 0 {
			data = data[:n]
			break
		}
	}

	//add our interfaces zone identifier since link-local addresses are routable over any interface
	remote = fmt.Sprintf("%s%%%d", string(data), i.Index)

	return remote
}

func help() {
	fmt.Println("wire r\n\treceive mode\nwire s PATH...\n\tsend PATH/s\nwire i\n\tinstall\nwire u\n\tuninstall")
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
