package sw

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"log"
	"math"
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
	defaultRto = time.Millisecond * 1000
	errAgain   = fmt.Errorf("EAGAIN")
	errBuf     = fmt.Errorf("E_EMPTY_BUF")
)

type segment struct {
	cmd  uint8
	seq  uint32
	data []byte
}

type ackReq struct {
	seg segment
	fin chan struct{}
}

type Sw struct {
	// 发送函数
	output func([]byte)

	rcvbuf    *list.List
	rcvpush   chan struct{}
	rcvlocker sync.Mutex

	// 收到ack 序列号
	recvAck chan *ackReq

	// 当前发送序列号
	// 首次收到确认之后,sndSeq+1
	sndSeq uint32

	// 上次收到的序列号
	// 如果收到的序列号 < latestSeq，程序bug
	// 如果收到的序列号 = latestSeq，视为重传包，需要进行确认
	// 如果收到的序列号 = latestSeq + 1，视为有效包，需要进行确认并往上层投递
	// 如果收到的序列号 > latestSeq + 1，程序bug
	latestSeq uint32

	// mss，超过mss进行分片
	mss int

	// 重传计时时间
	// 默认100ms
	// 每次收到ack之后，调整为 rtt * 0.6 + minrtt * 0.2 + maxrtt * 0.2
	rto time.Duration

	// rtt采样，用于动态调整rto
	// 微妙级别
	rtt    int64
	minrtt int64
	maxrtt int64

	// 重传统计
	resend int32

	// 重复ack统计
	dupAck int32

	// 超时计时器
	timer *time.Timer
}

func NewSw(output func([]byte)) *Sw {
	sw := &Sw{
		output:    output,
		rcvbuf:    list.New(),
		rcvpush:   make(chan struct{}),
		recvAck:   make(chan *ackReq),
		timer:     time.NewTimer(defaultRto),
		mss:       1400,
		rto:       defaultRto,
		latestSeq: 0,
		sndSeq:    1,
		rtt:       0,
		minrtt:    math.MaxInt64,
		maxrtt:    math.MinInt64,
	}
	return sw
}

// 上层包输出
// 上层把要发送的数据准备好之后，调用此函数
// output对数据包进行sw编码，编码成功之后
// 调用sw.output回调函数发送数据
func (sw *Sw) Output(data []byte) {
	// 分片
	if len(data) > sw.mss {
		log.Println("[D] fragment")
		for c := 0; c < len(data); {
			pos := c + sw.mss
			if pos > len(data) {
				pos = len(data)
			}

			seg := segment{
				cmd:  cmdPush,
				seq:  sw.sndSeq,
				data: data[c:pos],
			}

			sw.send(seg)
		}
	} else {
		seg := segment{
			cmd:  cmdPush,
			seq:  sw.sndSeq,
			data: data,
		}

		sw.send(seg)
	}
}

func (sw *Sw) send(seg segment) {
	data := sw.encode(seg)
	sw.output(data)

	beg := time.Now()
	sw.timer.Reset(sw.rto)
	for {
		select {
		// 死等ack
		case ack := <-sw.recvAck:
			log.Printf("[D] receive ack for segment: %d\n", sw.sndSeq)
			atomic.AddUint32(&sw.sndSeq, 1)

			rtt := time.Now().Sub(beg).Microseconds()
			sw.rtt = rtt
			if rtt < sw.minrtt {
				sw.minrtt = rtt
			}

			if rtt > sw.maxrtt {
				sw.maxrtt = rtt
			}

			// sw.rto = time.Duration(
			// 	int64(float64(sw.rtt)*0.4+
			// 		float64(sw.minrtt)*0.2+
			// 		float64(sw.maxrtt)*0.4)) * time.Microsecond

			log.Printf("[D] rtt %d minrtt: %d maxrtt: %d, rto: %d\n",
				sw.rtt, sw.minrtt, sw.maxrtt, sw.rto.Microseconds())

			sw.timer.Stop()
			close(ack.fin)
			return

			// 超时重传
		case <-sw.timer.C:
			atomic.AddInt32(&sw.resend, 1)
			log.Printf("[D] segment %d timeout, resending\n", sw.sndSeq)
			sw.output(data)
			sw.timer.Reset(sw.rto)
		}
	}
}

func (sw *Sw) sendAck(seg segment) {
	data := sw.encode(seg)
	sw.output(data)
}

// 下层包输入
// 收到一个下层数据包（eg: udp）时，调用此函数
// input对数据的数据包进行sw协议解码，并将数据写入recvbuf当中
// 上层调用sw.RecvFrom或者sw.RecvFromNonblock获取recvbuf数据
func (sw *Sw) Input(data []byte) error {
	seg, err := sw.decode(data)
	if err != nil {
		return err
	}

	// ack包
	if seg.cmd == cmdAck {
		log.Printf("[D] recv ack for seq %d\n", seg.seq)
		// 失序包，在sw协议中基本不会发生失序包
		if seg.seq > sw.sndSeq {
			log.Printf("[FATAL] unorder pkt, curseq %d, ack seq %d\n",
				sw.sndSeq, seg.seq)
			return nil
		}

		// 由于重传造成的ack重传
		// 由于数据包已经确认
		// 会出现ack的序列号比当前发送的序列号要小的场景
		if seg.seq < sw.sndSeq {
			atomic.AddInt32(&sw.dupAck, 1)
			log.Printf("[D] receive %d, curSeq: %d\n", seg.seq, sw.sndSeq)
			return nil
		}

		req := &ackReq{
			seg: seg,
			fin: make(chan struct{}),
		}
		sw.recvAck <- req
		<-req.fin
	}

	// 数据包
	if seg.cmd == cmdPush {
		// 失序包，在sw协议中基本不会发生失序包
		if seg.seq > sw.latestSeq+1 || seg.seq < sw.latestSeq {
			log.Printf("[FATAL] unorder pkt, expectSeq %d, pkt seq %d\n",
				sw.latestSeq, seg.seq)
			return nil
		}

		// 重复包
		// 由于不针对ack进行超时重传机制
		// 当ack丢包时，对方会继续重传，造成重复包
		// 如果不再进行确认，对方可能会一直重传
		if seg.seq == sw.latestSeq+1 {
			// sw.input(seg.data)
			sw.rcvlocker.Lock()
			sw.rcvbuf.PushBack(seg)
			sw.rcvlocker.Unlock()
			select {
			case sw.rcvpush <- struct{}{}:
			default:
			}
		} else {
			log.Printf("[D] duplicate segment %d\n", seg.seq)
		}

		// ack
		sw.sendAck(segment{
			cmd: cmdAck,
			seq: seg.seq,
		})

		log.Printf("[D] receive packet %d, size %d, %s\n",
			seg.seq, len(seg.data), string(seg.data))
		sw.latestSeq = seg.seq
	}

	return nil
}

func (sw *Sw) RecvFrom() ([]byte, error) {
	sw.rcvlocker.Lock()
	if sw.rcvbuf.Len() > 0 {
		front := sw.rcvbuf.Front()
		if front != nil {
			seg := front.Value.(segment)
			sw.rcvbuf.Remove(front)
			sw.rcvlocker.Unlock()
			return seg.data, nil
		}
	}
	sw.rcvlocker.Unlock()

	// 等待数据就绪
	fmt.Println("waiting for buf")
	<-sw.rcvpush

	sw.rcvlocker.Lock()
	if sw.rcvbuf.Len() > 0 {
		front := sw.rcvbuf.Front()
		if front != nil {
			seg := front.Value.(segment)
			sw.rcvbuf.Remove(front)
			sw.rcvlocker.Unlock()
			return seg.data, nil
		}
	}
	sw.rcvlocker.Unlock()

	return nil, errAgain
}

func (sw *Sw) RecvFromNonblock() ([]byte, error) {
	sw.rcvlocker.Lock()
	defer sw.rcvlocker.Unlock()
	if sw.rcvbuf.Len() > 0 {
		front := sw.rcvbuf.Front()
		if front != nil {
			seg := front.Value.(segment)
			return seg.data, nil
		}
	}

	return nil, errAgain
}

func (sw *Sw) decode(buf []byte) (segment, error) {
	if len(buf) < overHead {
		return segment{}, fmt.Errorf("invalid overhead")
	}

	seq := binary.BigEndian.Uint32(buf[cmdSize:])
	seg := segment{
		cmd:  buf[0],
		seq:  seq,
		data: buf[cmdSize+seqSize:],
	}

	return seg, nil
}

func (sw *Sw) encode(seg segment) []byte {
	data := make([]byte, 5)
	data[0] = seg.cmd
	binary.BigEndian.PutUint32(data[1:], seg.seq)
	data = append(data, seg.data...)
	return data
}

func (sw *Sw) Stat() {
	tick := time.NewTicker(time.Second * 5)
	defer tick.Stop()
	for range tick.C {
		resend := atomic.LoadInt32(&sw.resend)
		dupAck := atomic.LoadInt32(&sw.dupAck)
		fmt.Printf("resend %d, dupack %d\n", resend, dupAck)
	}
}
