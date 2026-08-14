package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// defaultAddr is used when neither --addr nor TERMUP_ADDR is set.
// Local default is the unix socket (file permissions act as auth);
// remote uses an http:// TCP address.
const defaultAddr = "unix:///tmp/termupd.sock"

// requestTimeout bounds a single call to termupd.
const requestTimeout = 5 * time.Second

// resolveAddr picks the server address: --addr flag > TERMUP_ADDR env > default.
func resolveAddr(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("TERMUP_ADDR"); v != "" {
		return v
	}
	return defaultAddr
}

// newClient returns an HTTP client and base URL. For unix:// it dials the
// socket; otherwise it talks plain HTTP to the given address.
func newClient(addr string) (*http.Client, string) {
	if path, ok := strings.CutPrefix(addr, "unix://"); ok {
		return &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", path)
				},
			},
		}, "http://localhost"
	}
	return &http.Client{Timeout: requestTimeout}, strings.TrimRight(addr, "/")
}

func getJSON(c *http.Client, url string, out any) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
