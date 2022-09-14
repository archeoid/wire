package main

import (
	"bufio"
	"fmt"
	"io"
)

func min(a int, b int64) int {
	if int64(a) <= b {
		return a
	} else {
		return int(b)
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

func read_into_channel(reader *bufio.Reader, size int64, channel chan []byte, print bool) (err error) {
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

		if print {
			if size == 0 {
				set_progress_color(1.0)
				fmt.Printf("\033[6D%5.1f%%", 100.0)
				reset_color()
			} else {
				progress := float64(total) / float64(size)
				set_progress_color(progress)
				fmt.Printf("\033[6D%5.1f%%", 100.0*progress)
				reset_color()
			}
		}

		if total == size {
			return nil
		}

		if total > size {
			//should never happen
			return fmt.Errorf("reader misaligned")
		}
	}
}
