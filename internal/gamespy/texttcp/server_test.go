package texttcp

import (
	"errors"
	"io"
	"net"
	"testing"
)

func TestIsNormalDisconnect(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{io.EOF, true},
		{net.ErrClosed, true},
		{errors.New("use of closed network connection"), false},
		{errors.New("connection reset by peer"), false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := isNormalDisconnect(tc.err); got != tc.want {
			t.Fatalf("isNormalDisconnect(%v)=%v want %v", tc.err, got, tc.want)
		}
	}

	// OpError wrapping ErrClosed (as produced by conn.Read after Close).
	op := &net.OpError{Op: "read", Net: "tcp", Err: net.ErrClosed}
	if !isNormalDisconnect(op) {
		t.Fatalf("isNormalDisconnect(OpError{ErrClosed})=false want true")
	}
}
