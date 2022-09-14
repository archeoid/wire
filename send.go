package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"math"
	"net"
	"os"
	"path/filepath"
	"sync"
)

var guard sync.Mutex

func read_file(path string, size int64, channel chan []byte) {
	//open the file and read `size` bytes of data onto the channel

	start := GetTime()

	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}

	reader := bufio.NewReader(f)

	err = read_into_channel(reader, size, channel, true)
	if err != nil {
		fmt.Println(err)
	}

	elapsed := float64(GetTime()-start) / 1000000.0

	fmt.Print("\033[6D")
	set_timing_color(elapsed)
	fmt.Printf("%s", format_elapsed(elapsed))
	fmt.Print("\033[0m\033[J")
	fmt.Print("\n")

	f.Close()

	guard.Unlock()
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
	channel := make(chan []byte, 2)

	var total int = 0

	parent := filepath.Dir(base)

	//get total files
	err := filepath.WalkDir(base, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total++
		}
		return nil
	})

	if err != nil {
		return err
	}

	var number int = 0
	var width int = int(math.Floor(math.Log10(float64(total))) + 1)

	err = filepath.WalkDir(base, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			number++

			//keep the top level folder
			//    if sending '/very/cool/folder' then our peer will receive 'folder' etc
			rel, _ := filepath.Rel(parent, path)

			guard.Lock()
			set_progress_color(float64(number) / float64(total))
			fmt.Printf("[%*d/%*d] ", width, number, width, total)
			reset_color()
			fmt.Printf("%s   0.0%%", rel)

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
		}
		return nil
	})

	return err

}

func expand_path(path string) []string {
	//expand wildcards in the arguments etc
	out := make([]string, 0)

	matches, err := filepath.Glob(path)
	if err != nil || len(matches) == 0 {
		out = append(out, path)
	} else {
		out = append(out, matches...)
	}

	return out
}

func expand_paths(paths []string) []string {
	//expand wildcards in the arguments etc
	out := make([]string, 0)
	for _, path := range paths {
		out = append(out, expand_path(path)...)
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
