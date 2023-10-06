package ut

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

var (
	DebugLog *log.Logger = log.New(os.Stdout, "[debug]", log.Ltime|log.Llongfile)
)

type NetClient struct {
	ctlcon                       *net.TCPConn
	msgcon                       *net.UDPConn
	msgchan                      chan Message
	CtlBufferSize, MsgBufferSize int
}

func (s *NetClient) SendMsg(src []byte) error {
	var (
		err error
	)
	_, err = s.msgcon.Write(src)
	return err
}

func (s *NetClient) SendControll(signal uint8, src []byte) error {
	var (
		err     error
		content []byte = make([]byte, 1+len(src))
	)
	content[0] = signal
	copy(content[1:], src)
	_, err = s.ctlcon.Write(content)
	return err
}

func (s *NetClient) Rec(msg *Message) error {
	*msg = <-s.msgchan
	if msg.Content == nil {
		return io.EOF
	}
	return nil
}

func (s *NetClient) Close() error {
	var (
		errlist []error
		err     error
	)
	err = s.ctlcon.Close()
	if err != nil {
		errlist = append(errlist, err)
	}
	err = s.msgcon.Close()
	if err != nil {
		errlist = append(errlist, err)
	}
	if len(errlist) > 0 {
		err = errors.Join(errlist...)
	}
	return err
}

func DialNewConn(host net.IP, tcp_port, udp_port int) (*NetClient, error) {
	var (
		err error
		cli *NetClient = new(NetClient)
	)
	cli.ctlcon, err = net.DialTCP("tcp", nil, &net.TCPAddr{IP: host, Port: tcp_port})
	if err == nil {
		cli.msgcon, err = net.DialUDP("udp", nil, &net.UDPAddr{IP: host, Port: udp_port})
		if err == nil {
			tibuffer := make([]byte, 4)
			_, err = rand.Reader.Read(tibuffer)
			if err == nil {
				cli.MsgBufferSize = 1 << 10
				cli.CtlBufferSize = 1 << 10
				_, err = cli.ctlcon.Write(tibuffer)
				fmt.Println("[debug] send control connection request")
				if err == nil {
					fmt.Println("[debug] send msgconnection request")
					_, err = cli.msgcon.Write(tibuffer)
					if err != nil {
						DebugLog.Println(err.Error())
						return nil, io.EOF
					}
					cli.msgchan = make(chan Message, 512)
					go func() {
						buffer := make([]byte, cli.CtlBufferSize)
						var lang int
						for {
							lang, err = cli.ctlcon.Read(buffer)
							if err == nil && lang > 0 {
								fmt.Println("[debug]accept msg from tcp length", lang)
								cli.msgchan <- Message{IsController: true, Content: buffer[:lang]}
							} else {
								cli.msgchan <- Message{Content: nil}
								break
							}
						}
					}()
					go func() {
						buffer := make([]byte, cli.MsgBufferSize)
						var lang int
						for {
							lang, err = cli.msgcon.Read(buffer)
							if err == nil && lang > 0 {
								fmt.Println("[debug]accept msg from udp length", lang)
								cli.msgchan <- Message{IsController: false, Content: buffer[:lang]}
							} else {
								time.Sleep(time.Second) //睡眠一会儿
								cli.msgchan <- Message{Content: nil}
								break
							}
						}
					}()
				}
			}
		}

	}
	return cli, err
}
