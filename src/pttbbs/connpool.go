package pttbbs

import (
	"code.google.com/p/vitess/go/memcache"
	"time"
)

type MemcacheConnPool struct {
	idle            chan connectResult
	drop            chan error
	req             chan int
	nrOpen, maxOpen int
	nrWait          int
	server          string
}

type connectResult struct {
	conn *memcache.Connection
	err  error
}

func NewMemcacheConnPool(server string, maxOpen int) *MemcacheConnPool {
	m := &MemcacheConnPool{
		idle:    make(chan connectResult),
		drop:    make(chan error),
		req:     make(chan int),
		nrOpen:  0,
		maxOpen: maxOpen,
		nrWait:  0,
		server:  server,
	}
	go m.manager()
	return m
}

func (m *MemcacheConnPool) GetConn() (*memcache.Connection, error) {
	var r connectResult
	select {
	case r = <-m.idle:
	default:
		m.req <- 1
		r = <-m.idle
		m.req <- -1
	}
	if r.err != nil {
		m.DropConn(r.conn)
	}
	return r.conn, r.err
}

func (m *MemcacheConnPool) ReleaseConn(c *memcache.Connection) {
	go func(c *memcache.Connection) {
		select {
		case m.idle <- connectResult{conn: c, err: nil}:
			// Somebody got it
		case <-time.After(time.Second * 10):
			// Timeout, close it
			m.DropConn(c)
		}
	}(c)
}

func (m *MemcacheConnPool) DropConn(c *memcache.Connection) {
	m.drop <- nil
	if c != nil {
		c.Close()
	}
}

func (m *MemcacheConnPool) manager() {
	for {
		select {
		case <-m.drop:
			m.nrOpen--
		case i := <-m.req:
			m.nrWait += i
		}
		for i := m.nrWait; i > 0 && m.nrOpen < m.maxOpen; i-- {
			m.nrOpen++
			go m.connect()
		}
	}
}

func (m *MemcacheConnPool) connect() {
	if c, err := memcache.Connect(m.server); err != nil {
		m.idle <- connectResult{conn: c, err: err}
	} else {
		m.ReleaseConn(c)
	}
}
