package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"math"
	"net"
	"os"
	"path/filepath"
)

type queue struct {
	pending []*transfer
	total   int
}

func new_queue() queue {
	var q queue
	q.pending = make([]*transfer, 0)
	return q
}

func (q *queue) enqueue_transfer(t *transfer) {
	q.total++
	t.number = q.total
	t.q = q
	q.pending = append(q.pending, t)
}

func (q *queue) enqueue_folder(folder string) error {
	parent := filepath.Dir(folder)
	err := filepath.WalkDir(folder, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			name, _ := filepath.Rel(parent, path)

			t, _ := from_file(path, name)
			q.enqueue_transfer(&t)
		}
		return nil
	})

	return err
}

func (q *queue) enqueue_path(path string) {
	//expand any wildcards
	paths := expand_path(path)

	for _, path := range paths {
		var i fs.FileInfo
		i, err := os.Stat(path)
		if err != nil {
			continue
		}
		is_dir := i.IsDir()

		var t transfer
		if !is_dir {
			if t, err = from_file(path, i.Name()); err != nil {
				continue
			}
			q.enqueue_transfer(&t)
		} else {
			q.enqueue_folder(path)
		}
	}
}

func send_display(t transfer) {
	progress := float64(t.progress) / float64(t.size)

	if t.progress == t.size {
		fmt.Printf("\033[6D\033[A\033[J")
	}

	if t.progress == 0 || t.progress == t.size {
		total_progress := float64(t.number) / float64(t.q.total)
		width := int(math.Floor(math.Log10(float64(t.q.total))) + 1)

		set_progress_color(total_progress)
		fmt.Printf("[%*d/%*d] ", width, t.number, width, t.q.total)
		reset_color()
		fmt.Printf("%s", t.name)
	}
	if t.progress == 0 {
		fmt.Println()
	}
	if t.progress != t.size {
		set_progress_color(progress)
		fmt.Printf("\033[6D%5.1f%%", 100.0*progress)
		reset_color()
	}
	if t.progress == t.size {
		elapsed := float64(get_time()-t.start) / 1000000.0
		set_timing_color(elapsed)
		fmt.Printf(" %s", format_elapsed(elapsed))
		fmt.Print("\033[0m\n")
	}
}

func send_display_basic(t transfer) {
	if t.progress == 0 {
		total_progress := float64(t.number) / float64(t.q.total)
		width := int(math.Floor(math.Log10(float64(t.q.total))) + 1)

		set_progress_color(total_progress)
		fmt.Printf("[%*d/%*d] ", width, t.number, width, t.q.total)
		reset_color()
		fmt.Printf("%s", t.name)
		fmt.Println()
	}
}

func send(paths []string, local, remote string) error {
	var err error

	raddr, _ := net.ResolveTCPAddr("tcp6", fmt.Sprintf("[%s]:%d", remote, DATA_PORT))
	laddr, _ := net.ResolveTCPAddr("tcp6", fmt.Sprintf("[%s]:%d", local, DATA_PORT))
	laddr.Port = 0

	conn, err := net.DialTCP("tcp6", laddr, raddr)
	if err != nil {
		show_error(err, "dial failed")
		terminate()
	}
	defer conn.Close()

	writer := bufio.NewWriter(conn)

	q := new_queue()

	for _, path := range paths {
		q.enqueue_path(path)
	}

	for _, p := range q.pending {
		header := p.build_header()
		if err = write_from_buffer(writer, header); err != nil {
			break
		}

		if err = to_wire(writer, *p, send_display); err != nil {
			break
		}
		writer.Flush()
	}

	if err != nil {
		fmt.Print("\033[6D\033[J")
		show_error(err, "FAIL")
	}

	return nil
}
