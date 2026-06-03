package main

import "sync/atomic"

type meshtasticMessageStats struct {
	forwarded atomic.Int64
	dropped   atomic.Int64
}

func (s *meshtasticMessageStats) IncForwarded() {
	if s != nil {
		s.forwarded.Add(1)
	}
}

func (s *meshtasticMessageStats) IncDropped() {
	if s != nil {
		s.dropped.Add(1)
	}
}

func (s *meshtasticMessageStats) Forwarded() int64 {
	if s == nil {
		return 0
	}
	return s.forwarded.Load()
}

func (s *meshtasticMessageStats) Dropped() int64 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}
