package ut

import (
	"net"
	"time"
)

const (
	CTR_BUFFER_SIZE = 0x080
	DELAY_TIME      = 100 * time.Millisecond
)

type Message struct {
	IsController bool
	Add          net.Addr
	Content      []byte
}
type Connection interface {
	SendMsg(src []byte) error
	SendControll(signal uint8, src []byte) error
	Rec(msg *Message) error
	Close() error
}
