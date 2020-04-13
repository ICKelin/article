package broker

import (
	"sync"
)

type PushMsg struct {
	Topic *Topic
	Data  interface{}
}

type Broker struct {
	rw     sync.RWMutex
	Routes map[string]*SubManager
}

func NewBroker() *Broker {
	return &Broker{
		Routes: make(map[string]*SubManager),
	}
}

func (b *Broker) Publish(topic *Topic, msg interface{}) {
	b.rw.RLock()
	defer b.rw.RUnlock()
	subMgr := b.Routes[topic.String()]
	if subMgr != nil {
		subMgr.Broadcast(&PushMsg{Topic: topic, Data: msg})
	}
}

func (b *Broker) Subscribe(topic *Topic, sub *Subscriber) {
	b.rw.Lock()
	defer b.rw.Unlock()
	key := topic.String()

	subMgr := b.Routes[key]
	if subMgr == nil {
		subMgr = NewSubManager()
		b.Routes[key] = subMgr
	}

	subMgr.Add(sub.Id, sub)
}

func (b *Broker) Unsubscribe(topic *Topic, sub *Subscriber) {
	b.rw.Lock()
	defer b.rw.Unlock()
	key := topic.String()

	subMgr := b.Routes[key]
	if subMgr != nil {
		subMgr.Del(sub.Id)
	}
}
