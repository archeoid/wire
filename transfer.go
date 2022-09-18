package main

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
)

type transfer struct {
	name   string
	path   string
	number int

	header_size uint16
	size        int64
	progress    int64

	start int64
	data  chan []byte

	q *queue
}

func (t transfer) build_header() []byte {
	header_size := 10 + len(t.name)
	header := make([]byte, header_size)

	binary.BigEndian.PutUint16(header[0:2], (uint16)(header_size))
	binary.BigEndian.PutUint64(header[2:10], (uint64)(t.size))
	copy(header[10:], t.name[:])

	return header
}

func from_wire(reader io.Reader) (transfer, error) {
	var t transfer
	t.data = make(chan []byte, 10)

	var header_size_data []byte = make([]byte, 2)
	err := read_into_buffer(reader, header_size_data)
	if err != nil {
		return t, err
	}

	t.header_size = binary.BigEndian.Uint16(header_size_data)

	header_data := make([]byte, t.header_size-2)
	err = read_into_buffer(reader, header_data)
	if err != nil {
		return t, err
	}

	t.size = int64(binary.BigEndian.Uint64(header_data[0:8]))
	name := string(header_data[8:])
	t.name = filepath.FromSlash(name)
	t.path = t.name

	return t, nil
}

func from_file(path, name string) (transfer, error) {
	var t transfer

	t.data = make(chan []byte, 10)
	t.name = filepath.ToSlash(name)
	t.path = path

	i, err := os.Stat(t.path)
	if err != nil {
		return t, err
	}
	t.size = i.Size()

	return t, nil
}

func to_disk(reader io.Reader, t transfer, display func(transfer)) (err error) {
	var writer *bufio.Writer
	writer, err = open_file_for_writing(t.path)
	defer writer.Flush()
	if err != nil {
		return err
	}

	return do_read_write(reader, writer, t, display)
}

func to_wire(writer io.Writer, t transfer, display func(transfer)) (err error) {
	var reader *bufio.Reader
	reader, err = open_file_for_reading(t.path)
	if err != nil {
		return err
	}

	return do_read_write(reader, writer, t, display)
}

func do_read_write(reader io.Reader, writer io.Writer, t transfer, display func(transfer)) error {
	read_progress := make(chan int, 1)
	write_progress := make(chan int, 1)
	errors := make(chan error, 1)

	go read_into_channel(reader, t.size, t.data, read_progress, errors)
	go write_from_channel(writer, t.size, t.data, write_progress, errors)

	t.start = get_time()
	display(t)

	done := 0
	for done != 2 {
		select {
		case m := <-read_progress:
			if m == -1 {
				err := <-errors
				return err
			}
			if m == 0 {
				done++
			}
		case m := <-write_progress:
			if m == -1 {
				err := <-errors
				return err
			}
			if m == 0 {
				done++
			}
			if m > 0 {
				t.progress += int64(m)
				display(t)
			}
		}
	}

	return nil
}
