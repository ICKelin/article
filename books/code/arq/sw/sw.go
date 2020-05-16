package sw

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	_ = iota
	cmdPush
	cmdAck
)

var (
	cmdSize    = 1
	seqSize    = 4
	overHead   = cmdSize + seqSize
	defaultRto = time.Millisecond * 100
	errAgain   = fmt.Errorf("EAGAIN")
)

type segment struct {
	cmd  uint8
	seq  uint32
	data []byte

	raddr *net.UDPAddr
}

type Sw struct {
	conn *net.UDPConn

	// 接收缓冲区
	rcvMu  sync.Mutex
	rcvbuf *list.List

	// 收到ack事件
	recvAck chan struct{}

	// 收到push事件
	recvPush chan struct{}

	// 当前发送序列号
	// 首次收到确认之后,sndSeq+1
	sndSeq uint32

	// 上收到的序列号
	// 如果收到的序列号 < lastSeq，程序bug
	// 如果收到的序列号 = lastSeq，视为重传包，需要进行确认
	// 如果收到的序列号 = lastSeq+1，视为有效包，需要进行确认并往上层投递
	// 如果收到的序列号 > lastSeq + 1，程序bug
	lastSeq uint32

	// mss，超过mss则进行分片
	mss int

	// 重传计时时间
	// 默认100ms
	// 每次收到ack之后，调整为 rtt * 0.6 + minrtt * 0.2 + maxrtt * 0.2
	rto time.Duration

	// rtt采样，微妙级别
	rtt    int64
	minrtt int64
	maxrtt int64

	// 超时计时器
	timer *time.Timer
}

func NewSw(conn *net.UDPConn) *Sw {
	sw := &Sw{
		conn:     conn,
		rcvbuf:   list.New(),
		recvAck:  make(chan struct{}),
		recvPush: make(chan struct{}),
		timer:    time.NewTimer(defaultRto),
		mss:      1400,
		rto:      defaultRto,
		lastSeq:  0,
		sndSeq:   1,
		rtt:      0,
		minrtt:   math.MaxInt64,
		maxrtt:   math.MinInt64,
	}
	go sw.read()
	return sw
}

func (sw *Sw) Peek() ([]byte, *net.UDPAddr, error) {
	sw.rcvMu.Lock()

	if sw.rcvbuf.Len() > 0 {
		ele := sw.rcvbuf.Front()
		seg := ele.Value.(segment)
		sw.rcvbuf.Remove(ele)
		sw.rcvMu.Unlock()
		return seg.data, seg.raddr, nil
	}
	sw.rcvMu.Unlock()
	return nil, nil, errAgain
}

func (sw *Sw) Send(data []byte, raddr *net.UDPAddr) {
	// 分片
	if len(data) > sw.mss {
		log.Println("[D] fragment")
		for c := 0; c < len(data); {
			pos := c + sw.mss
			if pos > len(data) {
				pos = len(data)
			}
			sw.sendFragment(data[c:pos], raddr)
			c = pos
		}
	} else {
		sw.sendFragment(data, raddr)
	}
}

func (sw *Sw) sendFragment(data []byte, raddr *net.UDPAddr) {
	log.Printf("[D]send fragment: %d\n", sw.sndSeq)
	seg := segment{
		cmd:   cmdPush,
		seq:   sw.sndSeq,
		data:  data,
		raddr: raddr,
	}
	sw.tx(seg)

	beg := time.Now()
	sw.timer.Reset(sw.rto)
	for {
		select {
		// 死等ack
		case <-sw.recvAck:
			log.Printf("[D]receive ack for segment: %d\n", sw.sndSeq)
			rtt := time.Now().Sub(beg).Microseconds()
			sw.rtt = rtt
			if rtt < sw.minrtt {
				sw.minrtt = rtt
			}

			if rtt > sw.maxrtt {
				sw.maxrtt = rtt
			}

			sw.rto = time.Duration(int64(float64(sw.rtt)*0.6+float64(sw.minrtt)*0.2+float64(sw.maxrtt)*0.2)) * time.Microsecond

			log.Printf("[D] rtt %d minrtt: %d maxrtt: %d, rto: %d\n",
				sw.rtt, sw.minrtt, sw.maxrtt, sw.rto.Microseconds())

			sw.timer.Stop()
			atomic.AddUint32(&sw.sndSeq, 1)
			return

		// 超时重传
		case <-sw.timer.C:
			log.Printf("[D] segment %d timeout, resending\n", sw.sndSeq)
			sw.tx(seg)
			sw.timer.Reset(defaultRto)
			log.Println("resend ", seg.seq)
		}
	}
}

func (sw *Sw) read() error {
	for {
		seg, err := sw.rx()
		if err != nil {
			log.Println(err)
			return err
		}

		// ack包
		if seg.cmd == cmdAck {
			// 失序包，在sw协议中基本不会发生失序包
			if seg.seq > sw.lastSeq+1 {
				log.Printf("[FATAL] unorder pkt, curseq %d, ack seq %d\n", sw.sndSeq, seg.seq)
				continue
			}

			// 由于重传造成的ack重传
			// 由于数据包已经确认
			// 会出现ack的序列号比当前发送的序列号要小的场景
			if seg.seq < sw.sndSeq {
				log.Printf("[D] receive %d, curSeq: %d\n", seg.seq, sw.sndSeq)
				continue
			}

			// 收到ack
			// ack可以是首次ack
			// 也可能是由于重传造成的重复ack
			// 这两种情况都需要重置计时器
			select {
			case sw.recvAck <- struct{}{}:
			default:
			}
		}

		// 数据包
		if seg.cmd == cmdPush {
			// 失序包，在sw协议中基本不会发生失序包
			if seg.seq > sw.lastSeq+1 || seg.seq < sw.lastSeq {
				log.Printf("[FATAL] unorder pkt, expectSeq %d, pkt seq %d\n", sw.lastSeq, seg.seq)
				continue
			}

			// ack
			sw.tx(segment{
				cmd:   cmdAck,
				seq:   seg.seq,
				raddr: seg.raddr,
			})

			// 重复包
			// 由于不针对ack进行超时重传机制
			// 当ack丢包时，对方会继续重传，造成重复包
			// 如果不再进行确认，对方会一直重传
			if seg.seq == sw.lastSeq {
				log.Printf("[D] duplicate segment %d\n", seg.seq)
				continue
			}

			sw.rcvMu.Lock()
			sw.rcvbuf.PushBack(seg)
			sw.rcvMu.Unlock()

			// 投递到上层
			select {
			case sw.recvPush <- struct{}{}:
			default:
			}

			sw.lastSeq = seg.seq
		}
	}
}

func (sw *Sw) tx(buf segment) {
	data := make([]byte, 5)
	data[0] = buf.cmd
	binary.BigEndian.PutUint32(data[1:], buf.seq)
	data = append(data, buf.data...)
	if sw.conn.RemoteAddr() != nil {
		_, err := sw.conn.Write(data)
		if err != nil {
			log.Println(err)
		}
	} else {
		_, err := sw.conn.WriteTo(data, buf.raddr)
		if err != nil {
			log.Println(err)
		}
	}
}

func (sw *Sw) rx() (segment, error) {
	buf := make([]byte, sw.mss)
	nr, raddr, err := sw.conn.ReadFromUDP(buf)
	if err != nil {
		return segment{}, err
	}

	if nr < overHead {
		return segment{}, fmt.Errorf("invalid overhead")
	}

	seq := binary.BigEndian.Uint32(buf[cmdSize:])
	seg := segment{
		cmd:   buf[0],
		seq:   seq,
		data:  buf[cmdSize+seqSize : nr],
		raddr: raddr,
	}

	return seg, nil
}
