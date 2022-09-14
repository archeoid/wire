package main

import (
	"bufio"
	"encoding/binary"
	"os"
	"path/filepath"
)

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
