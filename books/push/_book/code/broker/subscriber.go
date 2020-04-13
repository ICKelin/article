package broker

import "fmt"

var DefaultManager = NewSubManager()

type Subscriber struct {
	Id      int64
	Channel chan *PushMsg
}

type SubManager struct {
	subscribers map[int64]*Subscriber
}

func NewSubManager() *SubManager {
	return &SubManager{
		subscribers: make(map[int64]*Subscriber),
	}
}

func (sm *SubManager) Add(key int64, sub *Subscriber) {
	sm.subscribers[key] = sub
}

func (sm *SubManager) Del(key int64) {
	delete(sm.subscribers, key)
}

func (sm *SubManager) Get(id int64) *Subscriber {
	return sm.subscribers[id]
}

func (sm *SubManager) Broadcast(msg *PushMsg) {
	for id, s := range sm.subscribers {
		select {
		case s.Channel <- msg:
		default:
			fmt.Printf("subscriber[%d] channel full, channel size: %d\n", id, len(s.Channel))
		}
	}
}
