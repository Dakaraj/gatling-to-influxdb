package types

import (
	"sync"
	"time"
)

// UserLineData is a container for user data line and a node for linked ListULD
type UserLineData struct {
	Timestamp time.Time
	Scenario  string
	Status    string
	next      *UserLineData
}

// NewUserLineData creates a new UserLineData node
func NewUserLineData(timestamp time.Time, scenario, status string) *UserLineData {
	return &UserLineData{
		Timestamp: timestamp,
		Scenario:  scenario,
		Status:    status,
	}
}

type ListULD struct {
	head *UserLineData
	tail *UserLineData
	len  int
	l    sync.RWMutex
}

func (l *ListULD) Len() int {
	l.l.RLock()
	defer l.l.RUnlock()

	return l.len
}

func (l *ListULD) Head() *UserLineData {
	l.l.RLock()
	defer l.l.RUnlock()

	return l.head
}

func (l *ListULD) Tail() *UserLineData {
	l.l.RLock()
	defer l.l.RUnlock()

	return l.tail
}

func (l *ListULD) Append(urd *UserLineData) {
	l.l.Lock()
	defer l.l.Unlock()

	if l.head == nil {
		l.head = urd
		l.len = 1
		return
	}

	if l.tail == nil {
		l.tail = urd
		l.head.next = urd
		l.len = 2
		return
	}

	l.tail = urd
	l.tail.next = urd
	l.len++
}

func (l *ListULD) PopHead() *UserLineData {
	if l.head == nil {
		return nil
	}

}
