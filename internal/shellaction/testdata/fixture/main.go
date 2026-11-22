// This isolated executable is built into test temporary directories only.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "echo":
		if _, err := io.WriteString(os.Stdout, strings.Join(args, "|")); err != nil {
			os.Exit(3)
		}
	case "report":
		cwd, err := os.Getwd()
		if err != nil {
			os.Exit(3)
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"args": args, "cwd": cwd, "env": os.Environ()}); err != nil {
			os.Exit(3)
		}
	case "output":
		if len(args) != 3 {
			os.Exit(2)
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 0 || n > 65536 {
			os.Exit(2)
		}
		m, err := strconv.Atoi(args[1])
		if err != nil || m < 0 || m > 65536 {
			os.Exit(2)
		}
		ch := "x"
		if args[2] == "null" {
			ch = "\x00"
		}
		if _, err := io.WriteString(os.Stdout, strings.Repeat(ch, n)); err != nil {
			os.Exit(3)
		}
		if _, err := io.WriteString(os.Stderr, strings.Repeat(ch, m)); err != nil {
			os.Exit(3)
		}
	case "failure":
		if _, err := fmt.Fprint(os.Stdout, "secret-marker"); err != nil {
			os.Exit(3)
		}
		if _, err := fmt.Fprint(os.Stderr, "secret-marker"); err != nil {
			os.Exit(3)
		}
		os.Exit(7)
	case "binary":
		if _, err := os.Stdout.Write([]byte{0xff}); err != nil {
			os.Exit(3)
		}
	case "hold":
		if len(args) != 1 {
			os.Exit(2)
		}
		conn, err := net.DialTimeout("tcp", args[0], 5*time.Second)
		if err != nil {
			os.Exit(3)
		}
		if err := conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
			os.Exit(3)
		}
		if _, err := fmt.Fprintln(conn, os.Getpid()); err != nil {
			os.Exit(3)
		}
		_, readErr := io.Copy(io.Discard, conn)
		closeErr := conn.Close()
		if readErr != nil || closeErr != nil {
			os.Exit(3)
		}
	case "tree", "orphan":
		if len(args) != 1 {
			os.Exit(2)
		}
		self, err := os.Executable()
		if err != nil {
			os.Exit(3)
		}
		child := exec.Command(self, "hold", args[0])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		child.Env = os.Environ()
		if os.Args[1] == "orphan" {
			if err := child.Start(); err != nil {
				os.Exit(3)
			}
			os.Exit(0)
		}
		if err := child.Run(); err != nil {
			os.Exit(3)
		}
	default:
		os.Exit(2)
	}
}
