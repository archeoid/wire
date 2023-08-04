package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var CHUNK_SIZE int = 1024 * 1024

func min(a int, b int64) int {
	if int64(a) <= b {
		return a
	} else {
		return int(b)
	}
}

func read_into_buffer(reader io.Reader, buffer []byte) error {
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

func write_from_buffer(writer io.Writer, buffer []byte) error {
	total := 0
	len := len(buffer)
	for total != len {
		n, err := writer.Write(buffer[total:])
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

func read_into_channel(reader io.Reader, size int64, channel chan []byte, progress chan int, errors chan error) {
	total := int64(0)
	for {
		n := min(CHUNK_SIZE, size-total)
		buffer := make([]byte, n)
		err := read_into_buffer(reader, buffer)

		if err != nil {
			progress <- -1
			channel <- nil
			errors <- fmt.Errorf("link terminated")
			return
		}

		total += int64(n)
		channel <- buffer
		progress <- n

		if total == size {
			progress <- 0
			return
		}

		if total > size {
			progress <- -1
			channel <- nil
			errors <- fmt.Errorf("reader misaligned")
			return
		}
	}
}

func write_from_channel(writer io.Writer, size int64, channel chan []byte, progress chan int, errors chan error) {
	total := int64(0)
	for {
		chunk := <-channel

		if chunk == nil {
			progress <- -1
			errors <- fmt.Errorf("link terminated")
			return
		}

		err := write_from_buffer(writer, chunk)
		if err != nil {
			progress <- -1
			errors <- err
			return
		}
		progress <- len(chunk)

		total += int64(len(chunk))
		if total == size {
			progress <- 0
			return
		}

		if total > size {
			progress <- -1
			errors <- fmt.Errorf("channel misaligned")
			return
		}
	}
}

func open_file_for_writing(path string) (*bufio.Writer, error) {
	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	writer := bufio.NewWriter(f)

	return writer, nil
}

func open_file_for_reading(path string) (*bufio.Reader, error) {
	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(f)

	return reader, nil
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

func set_timing_color(ms float64) {
	if ms <= 0.2 {
		fmt.Print("\033[38;5;213m")
	} else if ms < 1.0 {
		fmt.Print("\033[38;5;177m")
	} else if ms < 1000.0 {
		fmt.Print("\033[38;5;141m")
	} else if ms < 60000.0 {
		fmt.Print("\033[38;5;69m")
	} else {
		fmt.Print("\033[38;5;33m")
	}
}

func set_progress_color(progress float64) {
	stage := int(progress * 5.99)
	switch stage {
	case 0:
		fmt.Print("\033[38;5;220m")
	case 1:
		fmt.Print("\033[38;5;184m")
	case 2:
		fmt.Print("\033[38;5;148m")
	case 3:
		fmt.Print("\033[38;5;112m")
	case 4:
		fmt.Print("\033[38;5;76m")
	case 5:
		fmt.Print("\033[38;5;40m")
	}
}

func reset_color() {
	fmt.Print("\033[0m")
}

func error_color() {
	fmt.Print("\033[38;5;160m")
}

func title_color() {
	fmt.Print("\033[38;5;21m")
}

func info_color() {
	fmt.Print("\033[38;5;220m")
}

func show_error(err error, msg string) {
	error_color()
	if msg != "" {
		fmt.Printf("%s\n", msg)
	}
	if err != nil {
		fmt.Printf("%s\n", err.Error())
	}
	reset_color()
}

func show_info(info string) {
	info_color()
	fmt.Println(info)
	reset_color()
}

func help() {
	show_info("wire r\n\treceive mode\nwire s PATH\n\tsend PATH/s\nwire wr OR wire ws\n\twireless modes\nwire i\n\tinstall\nwire u\n\tuninstall")
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
