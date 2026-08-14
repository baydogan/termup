package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	addrFlag := flag.String("addr", "", "termupd server address (unix:///path or http://host:port)")
	flag.Parse()

	client, base := newClient(resolveAddr(*addrFlag))

	args := flag.Args()
	if len(args) == 0 || args[0] != "watch" {
		fmt.Fprintln(os.Stderr, "usage: termup [--addr ...] watch")
		os.Exit(2)
	}

	p := tea.NewProgram(newModel(client, base), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "termup:", err)
		os.Exit(1)
	}
}
