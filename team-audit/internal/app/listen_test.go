package app

import (
	"net"
	"strconv"
	"testing"
)

func listen(t *testing.T, port int) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	return l
}
