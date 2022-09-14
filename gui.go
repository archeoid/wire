package main

import (
	"fmt"
	"io"
	"os"
)

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

func help() {
	fmt.Println("wire r\n\treceive mode\nwire s PATH...\n\tsend PATH/s\nwire i\n\tinstall\nwire u\n\tuninstall")
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
