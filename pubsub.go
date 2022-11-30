package main

import (
	"io"
	"sync"
	"time"
)

type EventData = any

type Event struct {
	Data   EventData
	Sender int64
}

type Subscriber interface {
	Send(data Event) error
}

type SubscriberCh struct {
	Ch chan<- Event
}

func (s *SubscriberCh) Send(e Event) error {
	select {
	case s.Ch <- e:
		return nil
	default:
		return io.ErrClosedPipe
	}
}

type Publisher struct {
	topic   *Topic
	created int64
}

func (p *Publisher) Send(data EventData) {
	p.topic.Send(p, Event{Sender: p.created, Data: data})
}

func (p *Publisher) Close() {
	topic := p.topic
	if topic == nil {
		return
	}
	p.topic = nil
	topic.Locker.Lock()
	defer topic.Locker.Unlock()
	if topic.ActivePublisher == p {
		topic.ActivePublisher = nil
	}
}

type Topic struct {
	Subscribers     map[Subscriber]struct{}
	Locker          sync.RWMutex
	ActivePublisher *Publisher
	MultiPublisher  bool
}

func NewTopic() *Topic {
	return &Topic{
		Subscribers:    map[Subscriber]struct{}{},
		MultiPublisher: true,
	}
}

func (t *Topic) Subscribe(s Subscriber) {
	t.Locker.Lock()
	defer t.Locker.Unlock()
	t.Subscribers[s] = struct{}{}
}

func (t *Topic) Unsubscribe(s Subscriber) {
	t.Locker.Lock()
	defer t.Locker.Unlock()
	delete(t.Subscribers, s)
}

func (t *Topic) Send(p *Publisher, event Event) {
	t.Locker.RLock()
	defer t.Locker.RUnlock()

	if !t.MultiPublisher && t.ActivePublisher != p {
		if t.ActivePublisher != nil && t.ActivePublisher.created > p.created {
			// Ignore old publisher
			return
		}
		// TODO: Swtich publisher event
		t.ActivePublisher = p
	}

	var errs []Subscriber

	for s := range t.Subscribers {
		if s.Send(event) != nil {
			errs = append(errs, s)
		}
	}

	for _, s := range errs {
		delete(t.Subscribers, s)
	}
}

func (t *Topic) NewPublisher() *Publisher {
	return &Publisher{
		topic:   t,
		created: time.Now().UnixNano(),
	}
}

type PubSubServer struct {
	Topics     map[string]*Topic
	Locker     sync.RWMutex
	AutoCreate bool
}

func NewPubSubServer() *PubSubServer {
	return &PubSubServer{
		Topics: map[string]*Topic{},
	}
}

func (s *PubSubServer) GetTopic(topic string, autoCreate bool) *Topic {
	s.Locker.Lock()
	defer s.Locker.Unlock()
	if t := s.Topics[topic]; t != nil {
		return t
	}

	if !autoCreate {
		return nil
	}

	t := NewTopic()
	s.Topics[topic] = t
	return t
}
