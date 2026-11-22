package localipc

import (
	"errors"
	"net"
)

func isClosed(err error) bool { return errors.Is(err, net.ErrClosed) }

type address struct{}

func (address) Network() string { return "test-local" }
func (address) String() string  { return "test-local" }
