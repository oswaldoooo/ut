package ut

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

var (
	TIME_OUT = 1 * time.Second
	// SolveFunc                           func(*Message)            = func(m *Message) {}
	Con_Lost                            error                     = errors.New("connection lost")
	register_con_map                    map[uint32]*NetConnection = make(map[uint32]*NetConnection) //临时储存，绑定成功后就删除
	register_con_mutex, store_con_mutex sync.Mutex
	store_con_map                       map[string]*NetConnection = make(map[string]*NetConnection) //通过udp客户端地址
	// DebugLog                            *log.Logger               = log.New(os.Stdout, "[debug]", log.Llongfile|log.Ltime)
	TimeOut error = errors.New("time out")
)

type NetListener struct {
	msglistener *net.UDPConn
	ctllistener *net.TCPListener
	Msg_Buffer  int
	// connection_map sync.Map
}
type NetConnection struct {
	con    *net.TCPConn
	udpcon *net.UDPConn
	// con_map     *sync.Map
	udp_cli_add   *net.UDPAddr
	msgchan       chan Message
	msgchan_mutex sync.Mutex
}

func Channel_Send(con *NetConnection, v *Message) {
	// 	ctx, cf := context.WithTimeout(context.Background(), TIME_OUT)
	// 	defer cf()
	// 	DebugLog.Println("msg chan prepare lock")
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			DebugLog.Println("msgchan lock time out")
	// 			return
	// 		default:
	// 			if con.msgchan_mutex.TryLock() {
	// 				defer con.msgchan_mutex.Unlock()
	// 				goto outbreak
	// 			}
	// 		}
	// 	}
	// outbreak:
	// 	DebugLog.Println("msg chan locked")
	// 	defer DebugLog.Println("msg chan unlocked")

	con.msgchan <- *v
}
func Channel_Get(con *NetConnection, dst *Message) {
	// 	ctx, cf := context.WithTimeout(context.Background(), TIME_OUT)
	// 	defer cf()
	// 	for {
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		default:
	// 			if con.msgchan_mutex.TryLock() {
	// 				defer con.msgchan_mutex.Unlock()
	// 				goto outbreak
	// 			}
	// 		}
	// 	}
	// outbreak:
	*dst = <-con.msgchan
}
func RegisterNewListener(tcp_port, udp_port int) (*NetListener, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: tcp_port})
	if err == nil {
		var udplistener *net.UDPConn
		udplistener, err = net.ListenUDP("udp", &net.UDPAddr{Port: udp_port})
		if err == nil {
			udplistener.SetReadBuffer(5 << 10)
			return &NetListener{msglistener: udplistener, ctllistener: listener, Msg_Buffer: 1 << 10}, nil
		}
	}
	return nil, err
}

// set the udp listener read buffer
func (s *NetListener) SetUdpReadBuffer(size int) {
	if size <= 0 {
		return
	}
	s.msglistener.SetReadBuffer(5 * size)
	s.Msg_Buffer = size
}

// set udp write buffer
func (s *NetListener) SetUdpWriteBuffer(size int) {
	if size <= 0 {
		return
	}
	s.msglistener.SetWriteBuffer(5 * size)
}
func (s *NetListener) Accept() (*NetConnection, error) {
	//需要等待tcp，udp都就绪后才能返回，否则可能会引起udp地址为空导致难以预测问题
	var (
		netcon *NetConnection = &NetConnection{}
		err    error
	)
	netcon.con, err = s.ctllistener.AcceptTCP()
	if err == nil {
		netcon.udpcon = s.msglistener
		// netcon.con_map = &s.connection_map
		tibuffer := make([]byte, 4)
		_, err = netcon.con.Read(tibuffer)
		if err == nil { //新增待注册连接
			register_con_mutex.Lock()
			// defer register_con_mutex.Unlock()
			register_code := binary.BigEndian.Uint32(tibuffer)
			register_con_map[register_code] = netcon
			register_con_mutex.Unlock()
			netcon.msgchan = make(chan Message)
			// s.connection_map.LoadOrStore(netcon.con.RemoteAddr().String(), netcon)
			go func() {
				defer func() {
					if e := recover(); e != nil {
						DebugLog.Println(e)
					}
				}()
				//listen tcp message
				var (
					err    error
					lang   int
					buffer []byte = make([]byte, CTR_BUFFER_SIZE)
				)
				for err == nil {
					lang, err = netcon.con.Read(buffer)
					if err == nil {
						DebugLog.Println("message from tcp")
						Channel_Send(netcon, &Message{IsController: true, Content: buffer[:lang]})
					}
				}
				DebugLog.Println("tcp connection broken")
				Channel_Send(netcon, &Message{Content: nil})
			}()
			DebugLog.Println("wait connection udp connection register")
			timer := time.NewTimer(2 * time.Second)
			for {
				select {
				case <-timer.C:
					return nil, TimeOut
				default:
					if netcon.udp_cli_add == nil {
						goto outbreak
					}
				}
			}
		outbreak:
			DebugLog.Printf("connection %s register success", netcon.udp_cli_add.String())
			return netcon, nil
		}
	}
	return nil, err
}
func (s *NetListener) Listen() {
	go func() {
		buffer := make([]byte, s.Msg_Buffer)
		var (
			lang          int
			register_code uint32
			err           error
			add           *net.UDPAddr
		)
		DebugLog.Println("udp listener start...")
		for {
			lang, add, err = s.msglistener.ReadFromUDP(buffer)
			if err == nil && lang > 0 {
				DebugLog.Println("accept new msg length", lang)
				if lang == 4 { //可能是注册码，进行绑定
					register_code = binary.BigEndian.Uint32(buffer[:4])
					time.Sleep(10 * time.Millisecond) //udp数据可能会比tcp先到，等待片刻
					register_con_mutex.Lock()
					if con_info, ok := register_con_map[register_code]; ok {
						con_info.udp_cli_add = new(net.UDPAddr)
						*con_info.udp_cli_add = *add
						//移除临时注册表，添加到持续储存表
						store_con_mutex.Lock()
						store_con_map[con_info.udp_cli_add.String()] = con_info
						store_con_mutex.Unlock()
						delete(register_con_map, register_code)
						DebugLog.Printf("bind new client %s success", con_info.udp_cli_add.String())
						register_con_mutex.Unlock()
						continue
					}
					register_con_mutex.Unlock()
				}
				store_con_mutex.Lock()
				if con, ok := store_con_map[add.String()]; ok {
					DebugLog.Printf("accept msg from %s,length %d,value %v", add.String(), lang, buffer[:lang])
					Channel_Send(con, &Message{IsController: false, Content: buffer[:lang]})

				} else {
					DebugLog.Printf("accept msg from %s,length %d,not find store connection", add.String(), lang)
				}
				store_con_mutex.Unlock()
			}
			if err != nil {
				DebugLog.Println("[udp error]", err.Error())
			}
		}
	}()
}
func (s *NetConnection) SendMsg(src []byte) error {
	var (
		err error
	)
	_, err = s.udpcon.WriteToUDP(src, s.udp_cli_add)
	return err
}

func (s *NetConnection) SendControll(signal uint8, src []byte) error {
	var (
		err     error
		content []byte = make([]byte, len(src)+1)
	)
	content[0] = signal
	copy(content[1:], src)
	_, err = s.con.Write(content)
	return err
}

func (s *NetConnection) Rec(msg *Message) error {
	if msg == nil {
		return errors.New("empty message body")
	}
	if s.msgchan == nil {
		DebugLog.Println("[warn]msg chan is nil")
		return errors.New("msgchan is nil")
	}
	Channel_Get(s, msg)
	if msg.Content == nil {
		return io.EOF
	}
	return nil
}

func (s *NetConnection) Close() error {
	time.Sleep(DELAY_TIME) //延迟一段时间再关闭
	store_con_mutex.Lock()
	defer store_con_mutex.Unlock()
	if _, ok := store_con_map[s.udp_cli_add.String()]; ok {
		delete(store_con_map, s.udp_cli_add.String())
		DebugLog.Println("close client channel")
		close(s.msgchan)
	} else {
		DebugLog.Printf("delete %s failed,not existed in store_map", s.udp_cli_add.String())
	}
	DebugLog.Printf("client %s closed", s.udp_cli_add.String())
	return s.con.Close()
}
